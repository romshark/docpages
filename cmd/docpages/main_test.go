package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/docpages"
)

// TestParseArgs tests the config of valid arguments.
func TestParseArgs(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want config
	}{
		{args: nil, want: config{dir: ".", out: "site", offline: true}},
		{args: []string{"docs"}, want: config{dir: "docs", out: "site", offline: true}},
		{args: []string{"-o", "out", "docs"}, want: config{dir: "docs", out: "out", offline: true}},
		{
			args: []string{"-serve", ":7340", "docs"},
			want: config{dir: "docs", out: "site", addr: ":7340", offline: true},
		},
		{
			args: []string{"-offline=false", "docs"},
			want: config{dir: "docs", out: "site"},
		},
		{
			args: []string{"-version"},
			want: config{dir: ".", out: "site", version: true, offline: true},
		},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var stderr strings.Builder
			c, err := parseArgs(tt.args, &stderr)
			require.NoError(t, err)
			require.Equal(t, tt.want, c)
			require.Empty(t, stderr.String())
		})
	}
}

// TestParseArgsErr tests that wrong arguments return errUsage after their error
// and the usage, such as -o, -serve and -version set together.
func TestParseArgsErr(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string // The error before the usage, empty for usage alone.
	}{
		{
			args: []string{"-o", "out", "-serve", ":0"},
			want: "-o and -serve can't be used together\n",
		},
		{
			args: []string{"-o", "out", "-serve", ":0", "-version"},
			want: "-o, -serve and -version can't be used together\n",
		},
		{
			args: []string{"-version", "-serve", ":0"},
			want: "-serve and -version can't be used together\n",
		},
		{
			args: []string{"-version", "docs"},
			want: "-version takes no dir\n",
		},
		{
			args: []string{"-version", "-offline=false"},
			want: "-offline and -version can't be used together\n",
		},
		{
			args: []string{"-bogus"},
			want: "flag provided but not defined: -bogus\n",
		},
		{args: []string{"a", "b"}},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var stderr strings.Builder
			_, err := parseArgs(tt.args, &stderr)
			require.ErrorIs(t, err, errUsage)
			require.True(t,
				strings.HasPrefix(
					stderr.String(),
					tt.want+"usage: docpages [flags] [dir]\n",
				),
				stderr.String())
		})
	}
}

// TestParseArgsHelp tests that -h returns flag.ErrHelp after the usage.
func TestParseArgsHelp(t *testing.T) {
	var stderr strings.Builder
	_, err := parseArgs([]string{"-h"}, &stderr)
	require.ErrorIs(t, err, flag.ErrHelp)
	require.True(t,
		strings.HasPrefix(
			stderr.String(),
			"usage: docpages [flags] [dir]\n",
		),
		stderr.String())
}

// TestVersionHint tests that a version error suggests running the required
// docpages with the same arguments, quoted for a shell, and that other errors
// stay as they are.
func TestVersionHint(t *testing.T) {
	ve := &docpages.VersionError{
		GoMod: "docs/go.mod", Required: "v0.4.0", Running: "v0.3.2",
	}
	err := versionHint(fmt.Errorf("reading: %w", ve),
		[]string{"-serve", "127.0.0.1:7340", "my docs"})
	require.ErrorIs(t, err, ve)
	require.EqualError(t, err, dedent(`
		reading: docs/go.mod requires github.com/romshark/docpages v0.4.0, incompatible with the running v0.3.2
		run: go run github.com/romshark/docpages/cmd/docpages@v0.4.0 -serve 127.0.0.1:7340 'my docs'`))

	other := errors.New("other")
	require.Equal(t, other, versionHint(other, nil))
}

// TestShellQuote tests the quoting of arguments for a POSIX shell.
func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"docs":           "docs",
		"127.0.0.1:7340": "127.0.0.1:7340",
		"my docs":        "'my docs'",
		"it's":           `'it'\''s'`,
		"":               "''",
	} {
		require.Equal(t, want, shellQuote(in), in)
	}
}

// dedent strips the leading line break and the common tab indent from an
// indented raw string. A tab-only last line keeps the final line break.
func dedent(s string) string {
	lines := strings.Split(strings.TrimPrefix(s, "\n"), "\n")
	indent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if n := len(l) - len(strings.TrimLeft(l, "\t")); indent < 0 || n < indent {
			indent = n
		}
	}
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			lines[i] = ""
		} else {
			lines[i] = l[indent:]
		}
	}
	return strings.Join(lines, "\n")
}

// TestVersionText tests the version that -version prints for a GoReleaser build,
// whose version lacks the "v" of Go's, and that a build without ldflags
// prints the version of its build info.
func TestVersionText(t *testing.T) {
	t.Cleanup(func() { version, commit, date = "", "", "" })
	version, commit, date = "0.1.0", "abc1234", "2026-11-09T12:00:00Z"
	require.Equal(t, "docpages v0.1.0 abc1234 2026-11-09T12:00:00Z", versionText())

	version, commit, date = "", "", ""
	require.True(t, strings.HasPrefix(versionText(), "docpages "), versionText())

	var stdout strings.Builder
	require.NoError(t, run([]string{"-version"}, &stdout, io.Discard))
	require.Equal(t, versionText()+"\n", stdout.String())
}
