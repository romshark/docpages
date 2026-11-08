package docpages

import (
	"bytes"
	"cmp"
	"fmt"
	"hash/fnv"
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

// reloadPath is [Handler]'s output-relative event stream for read-file state.
const reloadPath = "_reload"

// repositoryPath is [Handler]'s output-relative directory for linked repository files.
const repositoryPath = "_repository"

// pollInterval is how often [Handler] checks read files for changes.
const pollInterval = 250 * time.Millisecond

// Handler serves o's site at output-relative paths, as [Site.Write] lays it out.
// It rereads docs after a read file changes, showing changes without a restart,
// and reloads pages. Styles and scripts stay unminified.
// Paths follow GitHub Pages: trailing "/" serves index.html; a missing path serves
// its ".html" file, even with a dot as in guide/v1.2-notes.
// Errors such as a missing link heading are reloadable pages too.
//
// Links to repository files no page shows open working-tree files, not the host's
// pushed versions. Handler serves named files, directories, and their contents,
// excluding dot names such as .git. Text is plain text; directories list files.
func Handler(o Options) http.Handler {
	w := &watcher{root: o.Root, dir: o.Dir}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+reloadPath, w.serveEvents)
	mux.HandleFunc("GET /"+repositoryPath+"/",
		func(rw http.ResponseWriter, r *http.Request) {
			serveRepository(o, rw, r)
		})
	mux.HandleFunc("GET /", func(rw http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || strings.HasSuffix(name, "/") {
			name += "index.html"
		}
		// The page records its source-file state. Reading discovers page sources
		// included in that state, and files may change mid-read, so repeat until stable.
		state := w.state()
		files, err := w.files(o, state)
		for range 2 {
			next := w.state()
			if next == state {
				break
			}
			state = next
			files, err = w.files(o, state)
		}
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
		// Honor range requests, such as from video players.
		http.ServeContent(rw, r, name, time.Time{}, bytes.NewReader(content))
	})
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Otherwise reload may reuse cached CSS whose freshness comes from mtime.
		rw.Header().Set("Cache-Control", "no-cache")
		mux.ServeHTTP(rw, r)
	})
}

// watcher tracks the state of the files that a site reads.
type watcher struct {
	root, dir string
	lock      sync.Mutex
	// sources are files outside docs shown at the last read: page sources and media.
	sources []string

	// read serializes reads. readState is the last state read, producing readFiles
	// or readErr. Reading template.templ runs a program, too slow per request.
	read      sync.Mutex
	readState string
	readFiles map[string][]byte
	readErr   error
}

// files returns o's site at file state state, rereading unless it was read last.
func (w *watcher) files(o Options, state string) (map[string][]byte, error) {
	w.read.Lock()
	defer w.read.Unlock()
	if w.readState != state {
		w.readFiles, w.readErr = w.readSite(o, state)
		w.readState = state
	}
	return w.readFiles, w.readErr
}

// readSite reads o's site; its pages reload when file state differs from state.
func (w *watcher) readSite(o Options, state string) (map[string][]byte, error) {
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
	js, err := fs.ReadFile(static, "static/docs.js")
	if err != nil {
		return nil, err
	}
	for _, e := range s.editions {
		files[e.outDir()+"docpages.css"] = css
		files[e.outDir()+"docpages.js"] = js
	}
	return files, nil
}

// state hashes paths, sizes, and mtimes of docs files and page sources.
// Paths detect renames that preserve size and mtime. As the site does,
// it excludes dot names, keeping .git out of root-level docs.
func (w *watcher) state() string {
	h := fnv.New64a()
	consider := func(file string, info fs.FileInfo) {
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\x00",
			file, info.Size(), info.ModTime().UnixNano())
	}
	start := filepath.Join(w.root, w.dir)
	_ = filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case p != start && strings.HasPrefix(d.Name(), "."):
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		case d.IsDir():
			return nil
		}
		if info, err := d.Info(); err == nil {
			consider(p, info)
		}
		return nil
	})
	w.lock.Lock()
	sources := w.sources
	w.lock.Unlock()
	for _, f := range sources {
		p := filepath.Join(w.root, filepath.FromSlash(f))
		if info, err := os.Stat(p); err == nil {
			consider(p, info)
		}
	}
	return fmt.Sprintf("%x", h.Sum64())
}

// serveEvents sends file state initially and after each change until the request ends.
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

// serveRepository serves o's repository file at r's [repositoryPath] path when
// allowed, listing a directory's files.
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
	// A rooted filesystem prevents symlinks from escaping the repository.
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
		// Keep links relative so they work below a stripped server prefix.
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
		// Stat follows repository symlinks and rejects escapes, which the listing omits.
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

// serves reports whether [Handler] serves repository-relative file:
// an entry in served, or non-dot content below a served directory.
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

// repositoryType returns name's media type using data: extension-based for
// images, audio, video, and PDFs the browser displays, and plain text for other
// text such as source code.
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
