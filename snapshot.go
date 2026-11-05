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

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"go.yaml.in/yaml/v3"
)

// Snapshot copies the docs directory of o into its versions directory as the
// docs of the release tag, as versions/v1.2/ for v1.2.3, and returns that
// directory relative to o.Root. The copy is a docs directory of its own,
// which later patch releases share and whose pages stay editable:
//
//   - It holds the README.md, the groups and the other files of the docs
//     directory that their links name. logo.svg, the favicon and style.css
//     stay behind, since every edition shows those of the docs directory.
//   - The README.md loses the settings of the site and the note of main from
//     its front matter.
//   - A page with a source holds the text of the source in place of its own.
//   - A link to a file outside the docs directory becomes the URL of the file
//     at tag in the repository, or leads to the page that shows the file.
//   - An image or another media file of a page that lies outside the docs
//     directory is copied into media/ of the copy, which the page then shows.
//
// A directory of the release that exists already is an error.
func Snapshot(o Options, tag string) (string, error) {
	target, files, err := snapshot(os.DirFS(o.Root), path.Clean(filepath.ToSlash(o.Dir)),
		tag, func() (string, error) { return gitRemote(o.Root) })
	if err != nil {
		return "", err
	}
	for name, content := range files {
		p := filepath.Join(o.Root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(p, content, 0o644); err != nil {
			return "", err
		}
	}
	return target, nil
}

// snapshot returns the directory of the release tag in the versions directory
// of the docs directory dir of fsys, and the files that [Snapshot] writes
// there by their path from the repository root.
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
	// Reading the site checks the links of main, which the copy keeps.
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
		parser: parser.New(parser.WithExtensions(extension.NewGFMParser())),
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
	// Copying a Markdown file can queue the files that its links name.
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

// copy copies file of the docs directory into the directory of the release,
// a Markdown file with its links rewritten, and a media file from elsewhere
// in the repository into [mediaDir] of it.
// A missing file, as a dead link names, and a directory are skipped.
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

// markdown returns the copy of the Markdown file of the docs directory with
// the content data.
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
	var links []*ast.Link
	var images []*ast.Image
	var raws []ast.Node
	_ = ast.Walk(c.parser.Parse(body),
		func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch n := n.(type) {
			case *ast.Link:
				links = append(links, n)
			case *ast.Image:
				images = append(images, n)
			case *ast.RawHTML, *ast.HTMLBlock:
				raws = append(raws, n)
			}
			return ast.WalkContinue, nil
		})
	linkEdits, err := destEdits(body, linkDests(links),
		func(dest string) (string, error) {
			return c.link(file, origin, dest), nil
		})
	if err != nil {
		return nil, err
	}
	media := func(dest string) (string, error) {
		return c.media(file, origin, dest), nil
	}
	imageEdits, err := destEdits(body, imageDests(images), media)
	if err != nil {
		return nil, err
	}
	rawEdits, err := rawHTMLEdits(body, raws, media)
	if err != nil {
		return nil, err
	}
	body = applyEdits(body, slices.Concat(linkEdits, imageEdits, rawEdits))
	return slices.Concat(head, body), nil
}

// inside reports whether file, a path from the repository root, is a file of
// the docs directory and not of a release.
func (c *snapshotter) inside(file string) bool {
	return inDir(file, c.dir) && !inDir(file, path.Join(c.dir, versionsDir))
}

// media returns the destination of the media file dest in the copy of file,
// the dest having been written in origin: file or its source. It queues the
// media file, which a file of the docs directory keeps its place in the copy
// and another file of the repository takes its path in [mediaDir] of it.
func (c *snapshotter) media(file, origin, dest string) string {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
		return dest
	}
	t := repoPath(origin, u.Path)
	c.need(t)
	switch {
	case !c.inside(t):
		return relativeLink(file, path.Join(c.dir, mediaDir, t), u)
	case origin != file:
		return relativeLink(file, t, u)
	}
	return dest
}

// link returns the destination of the link dest in the copy of file, the dest
// having been written in origin: file or its source. A link to a file of the
// docs directory leads to its copy, which it queues, and a link to a file that
// a page shows, as a source, to the copy of that page. A link to another file
// of the repository becomes its URL at the release tag in the repository.
func (c *snapshotter) link(file, origin, dest string) string {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
		return dest
	}
	t := repoPath(origin, u.Path)
	inside := c.inside(t)
	if inside && origin == file {
		c.need(t)
		return dest
	}
	if to := c.main.pageOf(t); to >= 0 {
		p := c.main.pages[to]
		return relativeLink(file, cmp.Or(p.stub, p.file), u)
	}
	if inside {
		c.need(t)
		return relativeLink(file, t, u)
	}
	return c.atTag.fileURL(t, strings.HasSuffix(u.Path, "/"), "", u)
}

// relativeLink returns the link from the file from to the file or directory
// to, both paths from the repository root, with the query and the fragment of u.
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

// pageFrontMatter returns the front matter of the keys of fm that a page
// takes, with its delimiters, or nil if fm sets none of them.
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
