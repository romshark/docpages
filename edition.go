package docpages

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

// MainVersion names docs-directory pages and the repository-link branch they describe.
const MainVersion = "main"

// versionsDir holds a folder for each release, named by feature version
// such as v1.2, with the files that replace those of its tag.
const versionsDir = "versions"

// featureVersion matches the name of a release, its folder in versionsDir
// and its output directory, as v1.2, shared by all v1.2 patches.
var featureVersion = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)$`)

// edition is main, read from the docs directory, or a release, read from its tag.
type edition struct {
	version string // MainVersion or a feature version, as v1.2.
	tag     string // The tag of a release, as v1.2.3; empty for main.
	// dir is the docs directory, from the repository root.
	// A release reads it at its tag.
	dir    string
	pages  []page
	groups []group
	docs   []doc
	links  *site
	// note is the front-matter Markdown of noteFile shown above every page;
	// empty for none.
	note string
	// noteFile is the README.md on main that holds note: that of dir for main,
	// and that of the release folder in versions/ for a release.
	noteFile string
}

// isMain reports whether e documents the main branch.
func (e *edition) isMain() bool { return e.version == MainVersion }

// outDir returns the directory of the pages of e, relative to the output directory.
func (e *edition) outDir() string { return e.version + "/" }

// folder returns the folder of release e in versions/ of the docs directory,
// which replaces files of its tag.
func (e *edition) folder() string { return path.Join(e.dir, versionsDir, e.version) }

// readMain reads and converts docs directory dir in fsys as main.
// served is the link [site]'s served map.
func readMain(fsys fs.FS, dir string, h *host, served map[string]bool) (*edition, error) {
	l, err := readLayout(fsys, dir)
	if err != nil {
		return nil, err
	}
	links := &site{ref: MainVersion, host: h, served: served, fsys: fsys}
	docs, err := convertDocs(l.pages, l.srcs, links)
	if err != nil {
		return nil, err
	}
	readme := path.Join(dir, readmeName)
	return &edition{
		version:  MainVersion,
		dir:      dir,
		pages:    l.pages,
		groups:   l.groups,
		docs:     docs,
		links:    links,
		note:     strings.TrimSpace(l.settings.Note),
		noteFile: readme,
	}, nil
}

// noteHTML returns e's note HTML on page i, or "". Note links and images
// resolve from e's noteFile, so a release note on main names its own pages as
// that README.md names them. A link may open a page of another edition: a file
// in versions/v1.2/ is that of release v1.2, any other that of main.
// The page omits <p> around the single paragraph.
func (e *edition) noteHTML(i int, editions []*edition) (string, error) {
	if e.note == "" {
		return "", nil
	}
	src := []byte(e.note)
	doc := newParser().Parse(src)
	para, ok := doc.FirstChild().(*ast.Paragraph)
	if !ok || para.NextSibling() != nil {
		return "", errors.New("a note is 1 paragraph")
	}
	base := e.pages[i].base()
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if img, ok := n.(*ast.Image); ok {
			dest, head, err := e.noteMedia(
				string(img.Destination.Value(src)), base, editions,
			)
			if err != nil {
				return ast.WalkStop, err
			}
			sizeImage(img, head)
			img.Destination = text.NewSingleLineValueFromString(
				dest, text.IdentityDecoder,
			)
			return ast.WalkContinue, nil
		}
		l, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		dest, err := e.noteLink(i, string(l.Destination.Value(src)), editions)
		if err != nil {
			return ast.WalkStop, err
		}
		l.Destination = text.NewSingleLineValueFromString(dest, text.IdentityDecoder)
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	r := html.New(html.WithExtensions(extension.NewGFMHTMLRenderer()))
	if err := r.Render(&b, src, doc); err != nil {
		return "", err
	}
	h := strings.TrimSpace(b.String())
	return strings.TrimSuffix(strings.TrimPrefix(h, "<p>"), "</p>"), nil
}

// noteLink returns the URL of note link dest on page i of e.
func (e *edition) noteLink(i int, dest string, editions []*edition) (string, error) {
	base := e.pages[i].base()
	u, ok := repoLink(dest)
	if !ok {
		// A URL, or a fragment of the first page.
		return e.links.resolveOn(i, 0, dest, base, page.urlPath)
	}
	x, file := locate(repoPath(e.noteFile, u.Path), editions)
	if x == e {
		return e.links.resolveFile(i, 0, file, u, dest, base, page.urlPath)
	}
	to := x.links.pageOf(file)
	if to < 0 {
		// A repository file, at the ref of x.
		return x.links.resolveFile(-1, 0, file, u, dest, base, page.urlPath)
	}
	href := x.versionLink(base, x.pages[to].path, false).Href
	if u.Fragment == "" {
		return href, nil
	}
	if !x.links.ids[to][u.Fragment] {
		return "", fmt.Errorf("link %q names no heading", dest)
	}
	return href + "#" + u.Fragment, nil
}

// noteMedia returns the URL from version-root URL base and the image header of
// the media file that dest names in e's note. A release shows its own files.
func (e *edition) noteMedia(
	dest, base string, editions []*edition,
) (string, imageHeader, error) {
	u, err := url.Parse(dest)
	if err != nil {
		return "", imageHeader{}, fmt.Errorf("media %q: %w", dest, err)
	}
	if u.Scheme != "" || u.Host != "" || u.Path == "" {
		return dest, imageHeader{}, nil
	}
	x, file := locate(repoPath(e.noteFile, u.Path), editions)
	if x != e {
		return "", imageHeader{}, fmt.Errorf(
			"media %q is outside %s, which holds the files of the release",
			dest, e.folder())
	}
	c, err := e.links.mediaCopyOf(file, dest)
	if err != nil {
		return "", imageHeader{}, err
	}
	return withQueryFragment(base+(&url.URL{Path: c.name}).EscapedPath(), u),
		c.header, nil
}

// locate returns the edition of repository-relative file named by a note, and
// the path of that file in the edition. A file in the folder of a release in
// versions/ is that of the release at the same place in the docs directory.
// Any other file is main's. editions end with main.
func locate(file string, editions []*edition) (*edition, string) {
	for _, x := range editions[:len(editions)-1] {
		if f := x.folder(); inDir(file, f) {
			rel := strings.TrimPrefix(strings.TrimPrefix(file, f), "/")
			return x, path.Join(x.dir, rel)
		}
	}
	return editions[len(editions)-1], file
}

// inDir reports whether file, a path from the repository root, is dir or in dir.
func inDir(file, dir string) bool {
	return dir == "." || file == dir || strings.HasPrefix(file, dir+"/")
}

// versionLink links from version-root URL base to path in e and records current.
func (e *edition) versionLink(base, path string, current bool) VersionLink {
	root := strings.TrimPrefix(base, "./") + "../" + e.outDir()
	return VersionLink{
		Version:  e.version,
		Current:  current,
		Href:     root + path,
		Fallback: root,
	}
}

// currentVersion returns the version among versions that the page belongs to.
func currentVersion(versions []VersionLink) string {
	for _, l := range versions {
		if l.Current {
			return l.Version
		}
	}
	return ""
}
