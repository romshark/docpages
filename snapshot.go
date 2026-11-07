package docpages

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yuin/goldmark/v2/parser"
	"go.yaml.in/yaml/v3"
)

// Snapshot copies o's docs into its versions directory for release tag, such
// as versions/v1.2/ for v1.2.3, and returns the o.Root-relative directory.
// The editable copy is its own docs directory, shared by later patch releases:
//
//   - It contains README.md, groups, and linked docs files. logo.svg, favicon,
//     and style.css stay behind because every edition uses the docs versions.
//   - README.md front matter loses site settings and main's note.
//   - A sourced page contains its source text instead of its own.
//   - A link outside docs becomes the file's repository URL at tag, or opens
//     the page showing that file.
//   - Root-based links and media paths such as /docs/a.md become relative paths
//     to their copies.
//   - Page media outside docs is copied under media/ and shown from there.
//
// An existing release directory is an error.
func Snapshot(o Options, tag string) (string, error) {
	target, files, err := snapshot(os.DirFS(o.Root), path.Clean(filepath.ToSlash(o.Dir)),
		tag, func() (string, error) { return gitRemote(o.Root) })
	if err != nil {
		return "", err
	}
	if err := writeFiles(o.Root, files); err != nil {
		return "", err
	}
	return target, nil
}

// snapshot returns tag's release directory under docs directory dir in fsys,
// plus [Snapshot]'s files keyed by repository-relative path.
func snapshot(
	fsys fs.FS, dir, tag string, remote func() (string, error),
) (string, map[string][]byte, error) {
	if !releaseTag.MatchString(tag) {
		return "", nil, fmt.Errorf("%q is not a release tag, as v1.2.3", tag)
	}
	target := path.Join(dir, versionsDir, tag[:strings.LastIndexByte(tag, '.')])
	if _, err := fs.Stat(fsys, target); err == nil {
		return "", nil, fmt.Errorf("%s exists; remove it to snapshot %s again",
			target, tag)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", nil, err
	}
	// Reading checks main's links, which the copy preserves.
	s, err := read(fsys, dir, remote, false)
	if err != nil {
		return "", nil, err
	}
	c := &snapshotter{
		fsys:   fsys,
		dir:    dir,
		target: target,
		main:   s.main().links,
		atTag:  &site{ref: tag, host: s.chrome.host},
		parser: newParser(),
		files:  make(map[string][]byte),
		queued: make(map[string]bool),
	}
	c.need(path.Join(dir, readmeName))
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return "", nil, err
	}
	groups, err := ordered(entries, dir, true)
	if err != nil {
		return "", nil, err
	}
	for _, g := range groups {
		gdir := path.Join(dir, g.name)
		err := fs.WalkDir(fsys, gdir, func(p string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case p != gdir && strings.HasPrefix(d.Name(), "."):
				if d.IsDir() {
					return fs.SkipDir
				}
			case !d.IsDir():
				c.need(p)
			}
			return nil
		})
		if err != nil {
			return "", nil, err
		}
	}
	// Copying Markdown may queue linked files.
	for i := 0; i < len(c.queue); i++ {
		if err := c.copy(c.queue[i]); err != nil {
			return "", nil, err
		}
	}
	return target, c.files, nil
}

// snapshotter copies the files of a docs directory into the directory of a release.
type snapshotter struct {
	fsys   fs.FS
	dir    string // The docs directory.
	target string // The directory of the release.
	main   *site  // The links of main, whose pages tell which page shows a file.
	atTag  *site  // The links to the repository at the release tag.
	parser parser.Parser
	files  map[string][]byte // The copies, by their path from the repository root.
	queue  []string          // The files of the docs directory to copy.
	queued map[string]bool
}

// need queues file, a path from the repository root, for copying.
func (c *snapshotter) need(file string) {
	if !c.queued[file] {
		c.queued[file] = true
		c.queue = append(c.queue, file)
	}
}

// copy copies a docs file into the release, rewriting Markdown links.
// It copies repository media from elsewhere into [mediaDir].
// Missing files, such as dead link targets, and directories are skipped.
func (c *snapshotter) copy(file string) error {
	info, err := fs.Stat(c.fsys, file)
	if errors.Is(err, fs.ErrNotExist) || err == nil && info.IsDir() {
		return nil
	} else if err != nil {
		return err
	}
	data, err := fs.ReadFile(c.fsys, file)
	if err != nil {
		return err
	}
	if path.Ext(file) == ".md" {
		if data, err = c.markdown(file, data); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	rel := path.Join(mediaDir, file)
	if c.inside(file) {
		rel = file
		if c.dir != "." {
			rel = strings.TrimPrefix(file, c.dir+"/")
		}
	}
	c.files[path.Join(c.target, rel)] = data
	return nil
}

// markdown returns the docs Markdown file's copy with content data.
func (c *snapshotter) markdown(file string, data []byte) ([]byte, error) {
	fm, body, err := parseFrontMatter(data)
	if err != nil {
		return nil, err
	}
	head := data[:len(data)-len(body)]
	origin := file
	if fm.Source != "" {
		origin = repoPath(file, fm.Source)
		src, err := fs.ReadFile(c.fsys, origin)
		if err != nil {
			return nil, fmt.Errorf("source: %w", err)
		}
		if _, body, err = parseFrontMatter(src); err != nil {
			return nil, fmt.Errorf("%s: %w", origin, err)
		}
		fm.Source = ""
	}
	// The note of main is not that of the release.
	if origin != file || fm.hasSettings() || fm.Note != "" {
		if head, err = pageFrontMatter(fm); err != nil {
			return nil, err
		}
		body = bytes.TrimLeft(body, "\n")
		if head != nil {
			head = append(head, '\n')
		}
	}
	body, err = collectRefs(c.parser.Parse(body)).rewrite(body,
		func(dest string) (string, error) {
			return c.link(file, origin, dest), nil
		},
		func(dest string) (string, error) {
			return c.media(file, origin, dest), nil
		})
	if err != nil {
		return nil, err
	}
	return slices.Concat(head, body), nil
}

// inside reports whether repository-relative file belongs to docs but not a release.
// For root-level docs, only README.md and groups count, as in [unusedFiles];
// the rest is repository content linked at the release tag.
func (c *snapshotter) inside(file string) bool {
	if inDir(file, path.Join(c.dir, versionsDir)) {
		return false
	}
	if c.dir != "." {
		return inDir(file, c.dir)
	}
	top, _, _ := strings.Cut(file, "/")
	return file == readmeName || orderPrefix.MatchString(top)
}

// media returns dest's location in file's copy; dest was written in origin,
// either file or its source. It queues the media: docs media keeps its copied
// location, while other repository media moves under [mediaDir].
func (c *snapshotter) media(file, origin, dest string) string {
	u, ok := repoLink(dest)
	if !ok {
		return dest
	}
	t := repoPath(origin, u.Path)
	c.need(t)
	switch {
	case !c.inside(t):
		return relativeLink(file, path.Join(c.dir, mediaDir, t), u)
	case origin != file || rooted(u):
		return relativeLink(file, t, u)
	}
	return dest
}

// rooted reports whether u names a repository-root path such as /docs/a.md.
// Left unchanged in a release, it would name main's file instead of its copy.
func rooted(u *url.URL) bool { return strings.HasPrefix(u.Path, "/") }

// link returns dest's destination in file's copy; dest was written in origin,
// either file or its source. A docs-file link queues and opens its copy; a link
// to a displayed file such as a source opens that page's copy.
// Other repository files use their release-tag URLs.
func (c *snapshotter) link(file, origin, dest string) string {
	u, ok := repoLink(dest)
	if !ok {
		return dest
	}
	t := repoPath(origin, u.Path)
	inside := c.inside(t)
	if inside && origin == file && !rooted(u) {
		c.need(t)
		return dest
	}
	if to := c.main.pageOf(t); to >= 0 {
		p := c.main.pages[to]
		// A directory link opens its README page, a file URL without trailing "/".
		f := *u
		f.Path = strings.TrimSuffix(f.Path, "/")
		return relativeLink(file, cmp.Or(p.stub, p.file), &f)
	}
	if inside {
		c.need(t)
		return relativeLink(file, t, u)
	}
	return c.atTag.fileURL(t, strings.HasSuffix(u.Path, "/"), "", u)
}

// relativeLink links repository-relative from to file or directory to,
// preserving u's query and fragment.
func relativeLink(from, to string, u *url.URL) string {
	var dir []string
	if d := path.Dir(from); d != "." {
		dir = strings.Split(d, "/")
	}
	parts := strings.Split(to, "/")
	n := 0
	for n < len(dir) && n < len(parts) && dir[n] == parts[n] {
		n++
	}
	p := strings.Repeat("../", len(dir)-n) + strings.Join(parts[n:], "/")
	switch {
	case p == "":
		p = "./"
	case strings.HasSuffix(u.Path, "/") && !strings.HasSuffix(p, "/"):
		p += "/"
	}
	r := *u
	r.Path, r.RawPath = p, ""
	return r.String()
}

// pageFrontMatter returns delimited front matter for page-level keys set in fm,
// or nil if none are set.
func pageFrontMatter(fm frontMatter) ([]byte, error) {
	type keys struct {
		Title     string `yaml:"title,omitempty"`
		Suggested bool   `yaml:"suggested,omitempty"`
		Source    string `yaml:"source,omitempty"`
		URL       string `yaml:"url,omitempty"`
	}
	k := keys{
		Title:     fm.Title,
		Suggested: fm.Suggested,
		Source:    fm.Source,
		URL:       fm.URL,
	}
	if k == (keys{}) {
		return nil, nil
	}
	y, err := yaml.Marshal(k)
	if err != nil {
		return nil, err
	}
	return slices.Concat(
		[]byte(frontMatterDelim+"\n"),
		y,
		[]byte(frontMatterDelim+"\n"),
	), nil
}
