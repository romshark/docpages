package docpages

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

// MainVersion is the version of the pages of the docs directory, which document
// the main branch, and the branch that the links to the repository lead to.
const MainVersion = "main"

// versionsDir is the directory of the docs directory that holds a docs
// directory for each release, named after its feature version, as v1.2.
const versionsDir = "versions"

// featureVersion matches the name of the directory of a release in
// versionsDir, as v1.2, which holds the docs of every patch release of v1.2.
var featureVersion = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)$`)

// releaseTag matches a release tag, as v1.2.3.
var releaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// edition is a version of the documentation: main, whose pages the docs
// directory holds, or a release, whose pages a directory of versionsDir holds.
type edition struct {
	version string // MainVersion or a feature version, as v1.2.
	dir     string // The docs directory of the edition, from the repository root.
	pages   []page
	groups  []group
	docs    []doc
	links   *site
	// note is the Markdown of the note above every page, from the front matter
	// of the README.md of dir. Empty for none.
	note string
}

// isMain reports whether e documents the main branch.
func (e *edition) isMain() bool { return e.version == MainVersion }

// outDir returns the directory of the pages of e, relative to the output directory.
func (e *edition) outDir() string { return e.version + "/" }

// readEdition reads the docs directory dir of fsys as the edition of version
// and converts its pages. A release must keep its links and its sources within dir,
// since the files outside it document main and not the release. served is the
// [site] served of its links.
func readEdition(
	fsys fs.FS, dir, version string, h *host, served map[string]bool,
) (*edition, error) {
	l, err := readLayout(fsys, dir)
	if err != nil {
		return nil, err
	}
	links := &site{ref: MainVersion, host: h, served: served, fsys: fsys}
	if version != MainVersion {
		if l.settings.hasSettings() {
			return nil, fmt.Errorf("%s: a release takes the settings of the site "+
				"from the README.md of the docs directory", path.Join(dir, readmeName))
		}
		for _, p := range l.pages {
			if p.stub != "" && !inDir(p.file, dir) {
				return nil, fmt.Errorf("%s: source %s is outside %s", p.stub, p.file, dir)
			}
		}
		links.root = dir
	}
	docs, err := convertDocs(l.pages, l.srcs, links)
	if err != nil {
		return nil, err
	}
	return &edition{
		version: version,
		dir:     dir,
		pages:   l.pages,
		groups:  l.groups,
		docs:    docs,
		links:   links,
		note:    strings.TrimSpace(l.settings.Note),
	}, nil
}

// noteHTML returns the HTML of the note of e on the page with index i, or ""
// without a note. The links of the note lead where the links of the README.md
// of e lead, except that a link to a page of another of editions leads to that
// page, which lets the note of a release name another version.
// The note is 1 paragraph, which the page shows without <p>.
func (e *edition) noteHTML(i int, editions []*edition) (string, error) {
	if e.note == "" {
		return "", nil
	}
	src := []byte(e.note)
	doc := parser.New(parser.WithExtensions(extension.NewGFMParser())).Parse(src)
	para, ok := doc.FirstChild().(*ast.Paragraph)
	if !ok || para.NextSibling() != nil {
		return "", errors.New("a note is 1 paragraph")
	}
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		l, ok := n.(*ast.Link)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		raw := string(l.Destination.Value(src))
		dest, ok, err := e.editionLink(i, raw, editions)
		if !ok && err == nil {
			dest, err = e.links.resolveOn(i, 0, raw, e.pages[i].base(),
				func(p page) string { return p.path })
		}
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

// editionLink returns the URL of the page of another of editions that the link
// dest of the README.md of e leads to, from the page with index i of e, and
// whether dest leads to such a page: a file in the docs directory of that
// edition that the page shows.
func (e *edition) editionLink(i int, dest string, editions []*edition) (string, bool, error) {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
		return "", false, nil
	}
	file := repoPath(e.pages[0].file, u.Path)
	for _, x := range editions {
		if x == e || !inDir(file, x.dir) {
			continue
		}
		to := x.links.pageOf(file)
		if to < 0 {
			continue
		}
		href := x.versionLink(e.pages[i].base(), x.pages[to].path, false).Href
		if u.Fragment == "" {
			return href, true, nil
		}
		if !x.links.ids[to][u.Fragment] {
			return "", true, fmt.Errorf("link %q names no heading", dest)
		}
		return href + "#" + u.Fragment, true, nil
	}
	return "", false, nil
}

// releases returns the feature versions of the releases in the versions
// directory of the docs directory dir, the latest first.
func releases(fsys fs.FS, dir string) ([]string, error) {
	entries, err := fs.ReadDir(fsys, path.Join(dir, versionsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	type release struct {
		name         string
		major, minor int
	}
	var rs []release
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		m := featureVersion.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("%s: the directory of a release is named "+
				"after its feature version, as v1.2",
				path.Join(dir, versionsDir, e.Name()))
		}
		major, err1 := strconv.Atoi(m[1])
		minor, err2 := strconv.Atoi(m[2])
		if err := errors.Join(err1, err2); err != nil {
			return nil, fmt.Errorf("%s: %w", path.Join(dir, versionsDir, e.Name()), err)
		}
		rs = append(rs, release{name: e.Name(), major: major, minor: minor})
	}
	slices.SortFunc(rs, func(a, b release) int {
		return cmp.Or(cmp.Compare(b.major, a.major), cmp.Compare(b.minor, a.minor))
	})
	names := make([]string, len(rs))
	for i, r := range rs {
		names[i] = r.name
	}
	return names, nil
}

// inDir reports whether file, a path from the repository root, is dir or in dir.
func inDir(file, dir string) bool {
	return dir == "." || file == dir || strings.HasPrefix(file, dir+"/")
}

// versionLink is an entry of the version menu of a docs page.
type versionLink struct {
	Version string
	Current bool // Whether the page belongs to this version.
	// Href is the same page in this version, relative to the page.
	Href string
	// Fallback is the first page of this version, which docs.js opens when
	// this version lacks the page.
	Fallback string
}

// versionLink returns the link to the page at path in e, from a page whose
// version root is base, and current.
func (e *edition) versionLink(base, path string, current bool) versionLink {
	root := strings.TrimPrefix(base, "./") + "../" + e.outDir()
	return versionLink{
		Version:  e.version,
		Current:  current,
		Href:     root + path,
		Fallback: root,
	}
}

// currentVersion returns the version among versions that the page belongs to.
func currentVersion(versions []versionLink) string {
	for _, l := range versions {
		if l.Current {
			return l.Version
		}
	}
	return ""
}
