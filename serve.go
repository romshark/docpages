package docpages

import (
	"bytes"
	"cmp"
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// reloadPath is the path of the event stream of [Handler], relative to the
// output directory. It reports the state of the files that the site reads.
const reloadPath = "_reload"

// repositoryPath is the directory of [Handler], relative to the output
// directory, that holds the files of the repository that the links lead to.
const repositoryPath = "_repository"

// pollInterval is how often [Handler] looks for a change of the files that the
// site reads.
const pollInterval = 250 * time.Millisecond

// Handler serves the files of the site of o at their paths relative to the
// output directory, as [Site.Write] writes them. It reads the docs directory
// on every request for a file, which shows a change without a restart, and the
// pages reload when a file that the site reads changes. It serves the
// stylesheet unminified. Paths resolve as on GitHub Pages. A path that ends in
// "/" serves its index.html, and a path that names no file serves the file with
// ".html" appended, also a path with a dot, as guide/v1.2-notes. An error, such
// as a link to a missing heading, is a page, which reloads too.
//
// A link to a file of the repository that no page shows leads to the file in
// the working tree, not on the host, which has only what was pushed. Handler
// serves the files and directories that the links name and what the directories
// hold, except names that start with ".", as .git. It serves text as plain text
// and a directory as a list of its files.
func Handler(o Options) http.Handler {
	w := &watcher{root: o.Root, dir: o.Dir}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+reloadPath, w.serveEvents)
	mux.HandleFunc("GET /"+repositoryPath+"/", func(rw http.ResponseWriter, r *http.Request) {
		serveRepository(o, rw, r)
	})
	mux.HandleFunc("GET /", func(rw http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || strings.HasSuffix(name, "/") {
			name += "index.html"
		}
		state := w.state()
		files, err := w.files(o, state)
		if err != nil {
			reload := strings.Repeat("../", strings.Count(name, "/")) + reloadPath
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			rw.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(rw,
				"<!DOCTYPE html><pre>%s</pre>", html.EscapeString(err.Error()))
			_ = reloadScript(reload, state).Render(r.Context(), rw)
			return
		}
		content, ok := files[name]
		if !ok {
			name += ".html"
			content, ok = files[name]
		}
		if !ok {
			http.NotFound(rw, r)
			return
		}
		ctype := mime.TypeByExtension(path.Ext(name))
		switch path.Ext(name) {
		case ".md":
			ctype = "text/markdown; charset=utf-8"
		case ".txt":
			ctype = "text/plain; charset=utf-8"
		}
		if ctype != "" {
			rw.Header().Set("Content-Type", ctype)
		}
		// A range request, as a video player makes, gets its range.
		http.ServeContent(rw, r, name, time.Time{}, bytes.NewReader(content))
	})
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Without it, a reload can take the stylesheet from the cache, whose
		// freshness the browser derives from the modification time.
		rw.Header().Set("Cache-Control", "no-cache")
		mux.ServeHTTP(rw, r)
	})
}

// watcher tracks the state of the files that a site reads.
type watcher struct {
	root, dir string
	lock      sync.Mutex
	// sources are the files outside the docs directory that the pages
	// showed when the site was last read: sources of pages and media files.
	sources []string
}

// files reads the site of o and returns its files, the pages reloading when
// the state of the files differs from state.
func (w *watcher) files(o Options, state string) (map[string][]byte, error) {
	s, err := readOptions(o, true)
	if err != nil {
		return nil, err
	}
	var sources []string
	for _, e := range s.editions {
		for _, p := range e.pages {
			if p.stub != "" {
				sources = append(sources, p.file)
			}
		}
		for file := range e.links.media {
			if !inDir(file, path.Clean(filepath.ToSlash(o.Dir))) {
				sources = append(sources, file)
			}
		}
	}
	w.lock.Lock()
	w.sources = sources
	w.lock.Unlock()
	s.chrome.reload = state
	files, err := s.Files()
	if err != nil {
		return nil, err
	}
	css, err := fs.ReadFile(static, "static/style.css")
	if err != nil {
		return nil, err
	}
	for _, e := range s.editions {
		files[e.outDir()+"docpages.css"] = css
	}
	return files, nil
}

// state returns the state of the files of the docs directory and of the
// sources of its pages: the number of files and their latest modification time.
func (w *watcher) state() string {
	var n int
	var latest time.Time
	consider := func(info fs.FileInfo) {
		n++
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	_ = filepath.WalkDir(filepath.Join(w.root, w.dir),
		func(_ string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if info, err := d.Info(); err == nil {
				consider(info)
			}
			return nil
		})
	w.lock.Lock()
	sources := w.sources
	w.lock.Unlock()
	for _, f := range sources {
		if info, err := os.Stat(
			filepath.Join(w.root, filepath.FromSlash(f)),
		); err == nil {
			consider(info)
		}
	}
	return fmt.Sprintf("%d-%d", n, latest.UnixNano())
}

// serveEvents writes the state of the files as an event of a server-sent
// event stream, at first and after each change, until the request ends.
func (w *watcher) serveEvents(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/event-stream")
	rc := http.NewResponseController(rw)
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	last := ""
	for {
		if s := w.state(); s != last {
			last = s
			if _, err := fmt.Fprintf(rw, "data: %s\n\n", s); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-t.C:
		}
	}
}

// serveRepository serves the file of the repository of o at the path of r in
// [repositoryPath], if the site serves it, a directory as a list of its files.
func serveRepository(o Options, rw http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/"+repositoryPath+"/")
	file := cmp.Or(strings.TrimSuffix(name, "/"), ".")
	s, err := readOptions(o, true)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	if !fs.ValidPath(file) || !s.serves(file) {
		http.NotFound(rw, r)
		return
	}
	// A root keeps a symbolic link from leading out of the repository.
	root, err := os.OpenRoot(o.Root)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = root.Close() }()
	fsys := root.FS()
	info, err := fs.Stat(fsys, file)
	if err != nil {
		http.NotFound(rw, r)
		return
	}
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	if !info.IsDir() {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		rw.Header().Set("Content-Type", repositoryType(file, data))
		http.ServeContent(rw, r, file, info.ModTime(), bytes.NewReader(data))
		return
	}
	if name != "" && !strings.HasSuffix(name, "/") {
		// Relative, which holds under a prefix that the server strips.
		rw.Header().Set("Location", path.Base(file)+"/")
		rw.WriteHeader(http.StatusMovedPermanently)
		return
	}
	entries, err := fs.ReadDir(fsys, file)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	title := "/"
	if file != "." {
		title += file + "/"
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(rw, "<!DOCTYPE html><meta charset=\"utf-8\">"+
		"<title>%s</title><h1>%s</h1><ul>",
		html.EscapeString(title), html.EscapeString(title))
	if file != "." && s.serves(path.Dir(file)) {
		_, _ = fmt.Fprint(rw, `<li><a href="../">../</a></li>`)
	}
	for _, e := range entries {
		// Stat follows a symbolic link in the repository and fails for one
		// out of it, which the list leaves out as the handler does.
		p := path.Join(file, e.Name())
		info, err := fs.Stat(fsys, p)
		if err != nil || !s.serves(p) {
			continue
		}
		n := e.Name()
		if info.IsDir() {
			n += "/"
		}
		_, _ = fmt.Fprintf(rw, `<li><a href="%s">%s</a></li>`,
			html.EscapeString((&url.URL{Path: n}).EscapedPath()), html.EscapeString(n))
	}
	_, _ = fmt.Fprint(rw, "</ul>")
}

// serves reports whether [Handler] serves file, a path from the repository
// root: a file or directory of served, or what such a directory holds under
// no name that starts with ".", as .git.
func (s *Site) serves(file string) bool {
	for f := range s.served {
		var rest string
		switch {
		case file == f:
			return true
		case f == ".":
			rest = file
		case strings.HasPrefix(file, f+"/"):
			rest = file[len(f)+1:]
		default:
			continue
		}
		hidden := slices.ContainsFunc(strings.Split(rest, "/"), func(name string) bool {
			return strings.HasPrefix(name, ".")
		})
		if !hidden {
			return true
		}
	}
	return false
}

// repositoryType returns the media type of the file name of the repository
// with the content data: that of its extension for an image, audio, a video or
// a PDF, which the browser shows, and plain text for other text, as source
// code, which the browser shows as it is.
func repositoryType(name string, data []byte) string {
	t := mime.TypeByExtension(path.Ext(name))
	for _, media := range []string{"image/", "audio/", "video/", "application/pdf"} {
		if strings.HasPrefix(t, media) {
			return t
		}
	}
	if strings.HasPrefix(http.DetectContentType(data), "text/") {
		return "text/plain; charset=utf-8"
	}
	return cmp.Or(t, "application/octet-stream")
}
