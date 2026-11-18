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
// and reloads pages.
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
	// sources are the files and directories outside docs that the last read opened,
	// found or not, as page sources, media and linked files.
	// A failed read counts too, so that creating a missing file reads again.
	sources []string

	// read serializes reads. readState is the last state read, producing readFiles
	// or readErr. Reading template.templ runs a program, too slow per request.
	read      sync.Mutex
	readState string
	readFiles map[string][]byte
	readErr   error

	// poll shares the state of [watcher.recent] among the event streams of
	// the open tabs, computed at polledAt.
	poll        sync.Mutex
	polledAt    time.Time
	polledState string
}

// walked reports whether [watcher.state] walks file, a path from the
// repository root, of docs directory dir. Docs at the repository root take
// only the files at the root, the groups and versions/, so the walk leaves
// out the rest of the repository, as node_modules. [watcher.sources] holds
// the files that the site reads there.
func walked(file, dir string) bool {
	if dir != "." {
		return inDir(file, dir)
	}
	top, _, nested := strings.Cut(file, "/")
	return !nested || top == versionsDir || orderPrefix.MatchString(top)
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
// Releases read files at their tags, which don't change.
func (w *watcher) readSite(o Options, state string) (map[string][]byte, error) {
	fsys := &recordFS{FS: os.DirFS(o.Root), names: make(map[string]bool)}
	defer func() {
		dir := path.Clean(filepath.ToSlash(o.Dir))
		var sources []string
		for name := range fsys.names {
			if fs.ValidPath(name) && !walked(name, dir) {
				sources = append(sources, name)
			}
		}
		// state hashes them in order.
		slices.Sort(sources)
		w.lock.Lock()
		w.sources = sources
		w.lock.Unlock()
	}()
	s, err := readOptions(fsys, o, true)
	if err != nil {
		return nil, err
	}
	s.chrome.reload = state
	return s.Files()
}

// recordFS records the names that a read opens, found or not. It implements
// only Open, so that Stat, ReadFile and ReadDir open their names through it.
type recordFS struct {
	fs.FS
	names map[string]bool
}

func (r *recordFS) Open(name string) (fs.File, error) {
	r.names[name] = true
	return r.FS.Open(name)
}

// state hashes paths, sizes, and mtimes of docs files that [walked] covers and
// of the files of [watcher.sources]. Paths detect renames that preserve size and mtime.
// As the site does, it excludes dot names, keeping .git out of root-level docs.
func (w *watcher) state() string {
	h := fnv.New64a()
	consider := func(file string, info fs.FileInfo) {
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\x00",
			file, info.Size(), info.ModTime().UnixNano())
	}
	dir := path.Clean(filepath.ToSlash(w.dir))
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
			rel, err := filepath.Rel(w.root, p)
			if p != start && err == nil && !walked(filepath.ToSlash(rel)+"/", dir) {
				return filepath.SkipDir
			}
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
		switch info, err := os.Stat(p); {
		case err != nil:
		case info.IsDir():
			// Only its existence matters.
			// Its mtime changes with any entry, as an editor's swap file.
			_, _ = fmt.Fprintf(h, "%s\x00dir\x00", p)
		default:
			consider(p, info)
		}
	}
	return fmt.Sprintf("%x", h.Sum64())
}

// recent returns the state, which it computes at most twice per [pollInterval]
// for all event streams, as each open tab polls it. Twice keeps 1 tab, whose ticks
// come a little under pollInterval apart, from getting the state of its own last tick.
func (w *watcher) recent() string {
	w.poll.Lock()
	defer w.poll.Unlock()
	if now := time.Now(); now.Sub(w.polledAt) >= pollInterval/2 {
		w.polledState, w.polledAt = w.state(), now
	}
	return w.polledState
}

// serveEvents sends file state initially and after each change until the request ends.
func (w *watcher) serveEvents(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/event-stream")
	rc := http.NewResponseController(rw)
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	last := ""
	for {
		if s := w.recent(); s != last {
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
	s, err := readOptions(os.DirFS(o.Root), o, true)
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
