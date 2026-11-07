// Command docpages renders a GitHub repository's docs Markdown as a static site.
// See [github.com/romshark/docpages] for the docs-directory layout.
//
// Usage:
//
//	docpages [flags] [dir]
//
// dir is the docs directory, defaulting to the current one. Main pages go to
// <out>/main/; releases such as dir/versions/v1.2/ go to <out>/v1.2/.
// <out>/index.html and <out>/llms.txt lead to the latest release, or main if none.
//
// -snapshot v1.2.3 copies dir to dir/versions/v1.2/ as that release's docs
// instead of writing the site.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/romshark/docpages"
)

func main() {
	out := flag.String("o", "site", "Output directory")
	addr := flag.String("serve", "",
		"Serve the pages on this address instead of writing them, "+
			"reloading them on changes")
	tag := flag.String("snapshot", "",
		"Copy the docs into the versions directory as the docs of this "+
			"release tag, as v1.2.3, instead of writing the site")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "usage: docpages [flags] [dir]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	dir := "."
	if flag.NArg() == 1 {
		dir = flag.Arg(0)
	}
	o, err := options(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	switch {
	case *tag != "":
		target, err := docpages.Snapshot(o, *tag)
		if err != nil {
			fail(dir, err)
		}
		fmt.Printf("wrote the docs of %s into %s\n", *tag,
			filepath.Join(o.Root, filepath.FromSlash(target)))
		return
	case *addr != "":
		// The handler shows later errors in a page that reloads after fixes.
		// An initial build failure more likely means these are the wrong docs.
		if _, err := docpages.Read(o); err != nil {
			fail(dir, err)
		}
		fmt.Printf("serving %s on http://%s/\n", dir, *addr)
		if err := http.ListenAndServe(*addr, docpages.Handler(o)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	s, err := docpages.Read(o)
	if err == nil {
		err = s.Write(*out)
	}
	if err != nil {
		fail(dir, err)
	}
}

// fail prints dir's docs error and exits. If dir contains its own docs directory,
// as examples do, it names that directory.
func fail(dir string, err error) {
	fmt.Fprintln(os.Stderr, err)
	docs := filepath.Join(dir, "docs")
	if _, statErr := os.Stat(filepath.Join(docs, "README.md")); statErr == nil &&
		filepath.Base(filepath.Clean(dir)) != "docs" {
		fmt.Fprintf(os.Stderr, "%s holds a docs directory: did you mean %s?\n", dir, docs)
	}
	os.Exit(1)
}

// options returns dir's options, using its Git repository root.
func options(dir string) (docpages.Options, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return docpages.Options{}, err
	}
	root, err := docpages.GitRoot(abs)
	if err != nil {
		return docpages.Options{}, err
	}
	// Git reports the root with symlinks resolved.
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return docpages.Options{}, err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return docpages.Options{}, err
	}
	return docpages.Options{Root: root, Dir: rel}, nil
}
