// Command docpages renders the Markdown files of a docs directory of a GitHub
// repository as a static documentation site. See the package
// [github.com/romshark/docpages] for the layout of the docs directory.
//
// Usage:
//
//	docpages [flags] [dir]
//
// dir is the docs directory, the current directory by default. The pages of
// main go into <out>/main/ and those of each release in the versions directory
// of dir, as versions/v1.2/, into <out>/v1.2/. <out>/index.html and
// <out>/llms.txt lead to the latest release, or to main without releases.
//
// -snapshot v1.2.3 copies dir into versions/v1.2/ of dir as the docs of the
// release v1.2.3 instead of writing the site.
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
		// The handler shows an error in the page, which reloads after the fix.
		// Docs that don't build at the start, though, are most likely not
		// those that were meant.
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

// fail prints err, the error of the docs directory dir, and exits. A dir that
// holds a docs directory of its own, as each example does, names that one.
func fail(dir string, err error) {
	fmt.Fprintln(os.Stderr, err)
	docs := filepath.Join(dir, "docs")
	if _, statErr := os.Stat(filepath.Join(docs, "README.md")); statErr == nil &&
		filepath.Base(filepath.Clean(dir)) != "docs" {
		fmt.Fprintf(os.Stderr, "%s holds a docs directory: did you mean %s?\n", dir, docs)
	}
	os.Exit(1)
}

// options returns the options of the docs directory dir.
// The repository root is the root of the Git repository of dir.
func options(dir string) (docpages.Options, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return docpages.Options{}, err
	}
	root, err := docpages.GitRoot(abs)
	if err != nil {
		return docpages.Options{}, err
	}
	// The root that Git reports has its symbolic links resolved.
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return docpages.Options{}, err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return docpages.Options{}, err
	}
	return docpages.Options{Root: root, Dir: rel}, nil
}
