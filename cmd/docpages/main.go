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
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/romshark/docpages"
)

// errUsage reports wrong arguments, whose error and usage [parseArgs] has printed.
var errUsage = errors.New("usage")

func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	if errors.Is(err, errUsage) {
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// config is what the command-line arguments select. At most 1 of out,
// addr and tag is set by a flag.
type config struct {
	dir  string // Docs directory.
	out  string // Output directory of the site.
	addr string // Address to serve the docs on, or empty.
	tag  string // Release tag to snapshot the docs as, or empty.
}

// parseArgs returns the config of args, without the program name.
// It writes flag errors and usage to stderr and returns [errUsage] for wrong arguments,
// and [flag.ErrHelp] for -h.
func parseArgs(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("docpages", flag.ContinueOnError)
	flags.SetOutput(stderr)
	c := config{dir: "."}
	flags.StringVar(&c.out, "o", "site", "Output directory")
	flags.StringVar(&c.addr, "serve", "",
		"Serve the pages on this address instead of writing them, "+
			"reloading them on changes")
	flags.StringVar(&c.tag, "snapshot", "",
		"Copy the docs into the versions directory as the docs of this "+
			"release tag, as v1.2.3, instead of writing the site")
	flags.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "usage: docpages [flags] [dir]\n")
		flags.PrintDefaults()
	}
	switch err := flags.Parse(args); {
	case errors.Is(err, flag.ErrHelp):
		return config{}, err
	case err != nil:
		return config{}, errUsage
	}
	// Visit, unlike the values, tells -o apart from its default.
	var modes []string
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "o", "serve", "snapshot":
			modes = append(modes, "-"+f.Name)
		}
	})
	if n := len(modes); n > 1 {
		_, _ = fmt.Fprintf(stderr, "%s and %s can't be used together\n",
			strings.Join(modes[:n-1], ", "), modes[n-1])
		flags.Usage()
		return config{}, errUsage
	}
	if flags.NArg() > 1 {
		flags.Usage()
		return config{}, errUsage
	}
	if flags.NArg() == 1 {
		c.dir = flags.Arg(0)
	}
	return c, nil
}

// run runs docpages with the command-line arguments args, without the program name.
// It writes progress to stdout, and flag errors and usage to stderr.
func run(args []string, stdout, stderr io.Writer) error {
	c, err := parseArgs(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	o, err := options(c.dir)
	if err != nil {
		return err
	}
	switch {
	case c.tag != "":
		target, err := docpages.Snapshot(o, c.tag)
		if err != nil {
			return docsError(c.dir, err)
		}
		_, _ = fmt.Fprintf(stdout, "wrote the docs of %s into %s\n", c.tag,
			filepath.Join(o.Root, filepath.FromSlash(target)))
		return nil
	case c.addr != "":
		// The handler shows later errors in a page that reloads after fixes.
		// An initial build failure more likely means these are the wrong docs.
		if _, err := docpages.Read(o); err != nil {
			return docsError(c.dir, err)
		}
		_, _ = fmt.Fprintf(stdout, "serving %s on http://%s/\n", c.dir, c.addr)
		return http.ListenAndServe(c.addr, docpages.Handler(o))
	}
	s, err := docpages.Read(o)
	if err == nil {
		err = s.Write(c.out)
	}
	if err != nil {
		return docsError(c.dir, err)
	}
	return nil
}

// docsError returns err of dir's docs. If dir contains its own docs directory,
// as examples do, the error names that directory.
func docsError(dir string, err error) error {
	docs := filepath.Join(dir, "docs")
	if _, statErr := os.Stat(filepath.Join(docs, "README.md")); statErr == nil &&
		filepath.Base(filepath.Clean(dir)) != "docs" {
		return fmt.Errorf(
			"%w\n%s holds a docs directory: did you mean %s?",
			err, dir, docs,
		)
	}
	return err
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
