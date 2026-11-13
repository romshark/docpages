package docpages

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeTestRepo writes [testRepoFS] to a new directory and returns it.
func writeTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, f := range testRepoFS() {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, f.Data, 0o644))
	}
	return root
}

// get returns h's response status and body for path.
func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

// TestHandler checks GitHub Pages paths, reload scripts, unminified styles, and
// errors served as reloadable pages.
func TestHandler(t *testing.T) {
	root := writeTestRepo(t)
	notes := filepath.Join(root, "docs", "2-guides", "11-v1.2-notes.md")
	require.NoError(t, os.WriteFile(notes, []byte("# Notes of v1.2\n"), 0o644))
	h := Handler(Options{Root: root, Dir: "docs"})

	code, body := get(t, h, "/main/guides/events")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `<script data-url="../../_reload" data-state="`)

	// guides/v1.2-notes has extension .2-notes; GitHub Pages still serves it like
	// https://semver.org/spec/v2.0.0.
	code, body = get(t, h, "/main/guides/v1.2-notes")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, "<title>Notes of v1.2 - Demo</title>")

	code, body = get(t, h, "/main/")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `<script data-url="./../_reload"`)

	css, err := static.ReadFile("static/style.min.css")
	require.NoError(t, err)
	code, body = get(t, h, "/main/docpages.css")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, string(css), body)

	js, err := static.ReadFile("static/docs.min.js")
	require.NoError(t, err)
	code, body = get(t, h, "/main/docpages.js")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, string(js), body)

	code, _ = get(t, h, "/main/missing")
	require.Equal(t, http.StatusNotFound, code)

	events := filepath.Join(root, "docs", "2-guides", "01-events.md")
	require.NoError(t, os.WriteFile(events, []byte(dedent(`
		# Events

		[x](#missing)
	`)), 0o644))
	code, body = get(t, h, "/main/guides/events")
	require.Equal(t, http.StatusInternalServerError, code)
	require.Contains(t, body, `link &#34;#missing&#34; names no heading`)
	require.Contains(t, body, `<script data-url="../../_reload"`)
}

// TestHandlerRepository checks that non-page repository links open handler-served
// working-tree targets and directory contents,
// excluding hidden names and escaping symlinks.
func TestHandlerRepository(t *testing.T) {
	root := writeTestRepo(t)
	write := func(name, data string) {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(data), 0o644))
	}
	write("docs/2-guides/10-testing.md", dedent(`
		# Testing

		See [the examples](../../example/), [the license](../../LICENSE) and [the design](../../Design%20Doc.txt).
	`))
	write("Design Doc.txt", "The design.\n")
	write("example/cli/main.go", "package main\n")
	write("example/logo.png", "\x89PNG\r\n\x1a\n")
	write("example/.env", "SECRET=1\n")
	write("secret.txt", "Not linked.\n")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("Outside.\n"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "example", "outside.txt")))
	h := Handler(Options{Root: root, Dir: "docs"})

	code, body := get(t, h, "/main/guides/testing")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `<a href="../../_repository/example/">the examples</a>`)
	require.Contains(t, body, `<a href="../../_repository/LICENSE">the license</a>`)
	// A Markdown link ends at a space.
	_, body = get(t, h, "/main/guides/testing.md")
	require.Contains(t, body, "[the design](../../_repository/Design%20Doc.txt)")

	for name, tt := range map[string]struct {
		code  int
		ctype string
		body  string
	}{
		"/_repository/LICENSE": {
			200, "text/plain; charset=utf-8", "MIT License",
		},
		"/_repository/Design%20Doc.txt": {
			200, "text/plain; charset=utf-8", "The design.",
		},
		"/_repository/example/cli/main.go": {
			200, "text/plain; charset=utf-8", "package main",
		},
		"/_repository/example/logo.png": {
			200, "image/png", "PNG",
		},
		"/_repository/example/": {
			200, "text/html; charset=utf-8",
			`<ul><li><a href="cli/">cli/</a></li>` +
				`<li><a href="logo.png">logo.png</a></li></ul>`,
		},
		"/_repository/example/cli/": {
			200, "text/html; charset=utf-8",
			`<li><a href="../">../</a></li><li><a href="main.go">main.go</a></li>`,
		},
		"/_repository/example/.env":        {404, "", ""},
		"/_repository/example/outside.txt": {404, "", ""},
		"/_repository/secret.txt":          {404, "", ""},
		"/_repository/":                    {404, "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, name, nil))
			require.Equal(t, tt.code, rec.Code)
			if tt.code == http.StatusOK {
				require.Equal(t, tt.ctype, rec.Header().Get("Content-Type"))
				require.Contains(t, rec.Body.String(), tt.body)
			}
		})
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_repository/example", nil))
	require.Equal(t, http.StatusMovedPermanently, rec.Code)
	require.Equal(t, "example/", rec.Header().Get("Location"))
}

// TestHandlerEvents checks that the stream first reports page state, including
// outside sources, then a new state after a docs file changes.
func TestHandlerEvents(t *testing.T) {
	root := writeTestRepo(t)
	h := Handler(Options{Root: root, Dir: "docs"})
	srv := httptest.NewServer(h)
	defer srv.Close()
	_, body := get(t, h, "/main/")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, srv.URL+"/"+reloadPath, nil,
	)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	r := bufio.NewReader(resp.Body)
	next := func() string {
		t.Helper()
		for {
			line, err := r.ReadString('\n')
			if err == io.EOF {
				t.Fatal("stream ended")
			}
			require.NoError(t, err)
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				return strings.TrimSpace(data)
			}
		}
	}
	first := next()
	require.Contains(t, body, `data-state="`+first+`"`)

	// State uses mtime, which a coarse filesystem clock may not change immediately.
	future := time.Now().Add(time.Hour)
	p := filepath.Join(root, "docs", "2-guides", "10-testing.md")
	require.NoError(t, os.WriteFile(p, []byte(dedent(`
		# Testing

		Changed.
	`)), 0o644))
	require.NoError(t, os.Chtimes(p, future, future))
	require.NotEqual(t, first, next())
}

// TestWatcherState checks that renames, such as page reordering, change state
// while hidden-file changes, such as .git under root-level docs, do not.
func TestWatcherState(t *testing.T) {
	root := writeTestRepo(t)
	w := &watcher{root: root, dir: "docs"}
	state := w.state()

	hidden := filepath.Join(root, "docs", ".cache", "index")
	require.NoError(t, os.MkdirAll(filepath.Dir(hidden), 0o755))
	require.NoError(t, os.WriteFile(hidden, []byte("x"), 0o644))
	require.Equal(t, state, w.state())

	guides := filepath.Join(root, "docs", "2-guides")
	require.NoError(t, os.Rename(
		filepath.Join(guides, "10-testing.md"), filepath.Join(guides, "11-testing.md")))
	require.NotEqual(t, state, w.state())
}
