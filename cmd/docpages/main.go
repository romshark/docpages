// Command docpages renders a GitHub repository's docs Markdown as a static site.
// See [github.com/romshark/docpages] for the docs-directory layout.
//
// Usage:
//
//	docpages [flags] [dir]
//
// dir is the docs directory, defaulting to the current one. Main pages go to
// <out>/main/; releases, read from their Git tags, go to <out>/v1.2/.
// <out>/index.html and <out>/llms.txt lead to the latest release, or main if none.
package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
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
// addr and version is set by a flag.
type config struct {
	dir     string // Docs directory.
	out     string // Output directory of the site.
	addr    string // Address to serve the docs on, or empty.
	version bool   // Whether to print the version of docpages.
}

// version, commit and date describe a GoReleaser build, which sets them with
// -ldflags. Other builds leave them empty.
var version, commit, date string

// versionText returns the version of this docpages, its commit and its build date,
// from the ldflags or else from the build info that Go stamps.
func versionText() string {
	v, c, d := version, commit, date
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			v = info.Main.Version
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					c = s.Value
				case "vcs.time":
					d = s.Value
				}
			}
		}
	}
	// GoReleaser's version lacks the "v" of Go's.
	if v != "" && v != "(devel)" && !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	r := "docpages " + cmp.Or(v, "(devel)")
	if c != "" {
		r += " " + c[:min(len(c), 12)]
	}
	if d != "" {
		r += " " + d
	}
	return r
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
	flags.BoolVar(&c.version, "version", false, "Print the version of docpages")
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
		case "o", "serve", "version":
			modes = append(modes, "-"+f.Name)
		}
	})
	if n := len(modes); n > 1 {
		_, _ = fmt.Fprintf(stderr, "%s and %s can't be used together\n",
			strings.Join(modes[:n-1], ", "), modes[n-1])
		flags.Usage()
		return config{}, errUsage
	}
	if c.version && flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "-version takes no dir\n")
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
func run(args []string, stdout, stderr io.Writer) (err error) {
	defer func() { err = versionHint(err, args) }()
	c, err := parseArgs(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if c.version {
		_, _ = fmt.Fprintln(stdout, versionText())
		return nil
	}
	o, err := options(c.dir)
	if err != nil {
		return err
	}
	if c.addr != "" {
		// The handler shows later errors in a page that reloads after fixes.
		// An initial build failure more likely means these are the wrong docs.
		s, err := docpages.Read(o)
		if err == nil {
			_, err = s.Files()
		}
		if err != nil {
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

// versionHint adds to a [docpages.VersionError] in err the command that runs
// the docpages that the docs module requires, with args.
func versionHint(err error, args []string) error {
	var ve *docpages.VersionError
	if !errors.As(err, &ve) {
		return err
	}
	cmd := append([]string{"go", "run", commandPath + "@" + ve.Required}, args...)
	for i, a := range cmd {
		cmd[i] = shellQuote(a)
	}
	return fmt.Errorf("%w\nrun: %s", err, strings.Join(cmd, " "))
}

// commandPath is the package path of this command.
const commandPath = "github.com/romshark/docpages/cmd/docpages"

// shellQuote returns s quoted for a POSIX shell, unchanged if it needs no quotes.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyz"+
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./_-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
