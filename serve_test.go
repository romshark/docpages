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

// writeTestRepo writes the files of [testRepoFS] into a new directory and returns it.
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

// get returns the status code and the body of the response of h to a request
// for path.
func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

// TestHandler tests that the handler serves the pages at the paths of GitHub
// Pages with the script that reloads them, the unminified stylesheet, and an
// error as a page that reloads too.
func TestHandler(t *testing.T) {
	root := writeTestRepo(t)
	h := Handler(Options{Root: root, Dir: "docs"})

	code, body := get(t, h, "/main/guides/events")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `<script data-url="../../_reload" data-state="`)

	code, body = get(t, h, "/main/")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `<script data-url="./../_reload"`)

	css, err := static.ReadFile("static/style.css")
	require.NoError(t, err)
	code, body = get(t, h, "/main/docpages.css")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, string(css), body)

	code, _ = get(t, h, "/main/missing")
	require.Equal(t, http.StatusNotFound, code)

	events := filepath.Join(root, "docs", "2-guides", "01-events.md")
	require.NoError(t, os.WriteFile(events, []byte("# Events\n\n[x](#missing)\n"), 0o644))
	code, body = get(t, h, "/main/guides/events")
	require.Equal(t, http.StatusInternalServerError, code)
	require.Contains(t, body, `link &#34;#missing&#34; names no heading`)
	require.Contains(t, body, `<script data-url="../../_reload"`)
}

// TestHandlerRepository tests that the links to the files of the repository
// that no page shows lead to the files of the working tree that the handler
// serves: those that the links name and what a directory among them holds,
// except hidden names and a symbolic link out of the repository.
func TestHandlerRepository(t *testing.T) {
	root := writeTestRepo(t)
	write := func(name, data string) {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(data), 0o644))
	}
	write("docs/2-guides/10-testing.md",
		"# Testing\n\nSee [the examples](../../example/), [the license](../../LICENSE) "+
			"and [the design](../../Design%20Doc.txt).\n")
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

// TestHandlerEvents tests that the event stream reports the state of the
// files at first and again after a file of the docs directory changes.
func TestHandlerEvents(t *testing.T) {
	root := writeTestRepo(t)
	h := Handler(Options{Root: root, Dir: "docs"})
	srv := httptest.NewServer(h)
	defer srv.Close()

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
	_, body := get(t, h, "/main/")
	require.Contains(t, body, `data-state="`+first+`"`)

	// The modification time of the file is the state, which a coarse clock
	// of the file system would leave unchanged by an immediate write.
	future := time.Now().Add(time.Hour)
	p := filepath.Join(root, "docs", "2-guides", "10-testing.md")
	require.NoError(t, os.WriteFile(p, []byte("# Testing\n\nChanged.\n"), 0o644))
	require.NoError(t, os.Chtimes(p, future, future))
	require.NotEqual(t, first, next())
}
