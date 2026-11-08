package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseArgs tests the config of valid arguments.
func TestParseArgs(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want config
	}{
		{args: nil, want: config{dir: ".", out: "site"}},
		{args: []string{"docs"}, want: config{dir: "docs", out: "site"}},
		{args: []string{"-o", "out", "docs"}, want: config{dir: "docs", out: "out"}},
		{
			args: []string{"-serve", ":7340", "docs"},
			want: config{dir: "docs", out: "site", addr: ":7340"},
		},
		{
			args: []string{"-snapshot", "v1.2.3", "docs"},
			want: config{dir: "docs", out: "site", tag: "v1.2.3"},
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
// and the usage, such as -o, -serve and -snapshot set together.
func TestParseArgsErr(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string // The error before the usage, empty for usage alone.
	}{
		{
			args: []string{"-serve", ":0", "-snapshot", "v1.2.3"},
			want: "-serve and -snapshot can't be used together\n",
		},
		{
			args: []string{"-o", "out", "-serve", ":0"},
			want: "-o and -serve can't be used together\n",
		},
		{
			args: []string{"-snapshot", "v1.2.3", "-o", "site"},
			want: "-o and -snapshot can't be used together\n",
		},
		{
			args: []string{"-o", "out", "-serve", ":0", "-snapshot", "v1.2.3"},
			want: "-o, -serve and -snapshot can't be used together\n",
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
