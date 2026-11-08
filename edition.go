package docpages

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

// MainVersion names docs-directory pages and the repository-link branch they describe.
const MainVersion = "main"

// versionsDir holds each release's docs, named by feature version such as v1.2.
const versionsDir = "versions"

// featureVersion matches a release directory such as v1.2, shared by all v1.2 patches.
var featureVersion = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)$`)

// releaseTag matches a release tag, as v1.2.3.
var releaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// edition is main, stored in docs, or a release stored under versionsDir.
type edition struct {
	version string // MainVersion or a feature version, as v1.2.
	dir     string // The docs directory of the edition, from the repository root.
	pages   []page
	groups  []group
	docs    []doc
	links   *site
	// note is dir/README.md's front-matter Markdown shown above every page;
	// empty for none.
	note string
}

// isMain reports whether e documents the main branch.
func (e *edition) isMain() bool { return e.version == MainVersion }

// outDir returns the directory of the pages of e, relative to the output directory.
func (e *edition) outDir() string { return e.version + "/" }

// readEdition reads and converts dir in fsys as version. Release links and
// sources must stay in dir because outside files describe main.
// served is the link [site]'s served map.
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

// noteHTML returns e's note HTML on page i, or "". Note links and images resolve
// like e's README, except links may open pages in other editions so release
// notes can name other versions. The page omits <p> around the single paragraph.
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
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		// Image paths are relative to README.md (page 0).
		if img, ok := n.(*ast.Image); ok {
			dest, err := e.links.mediaURL(
				0, img.Destination.Value(src), e.pages[i].base(),
			)
			if err != nil {
				return ast.WalkStop, err
			}
			img.Destination = text.NewSingleLineValueFromString(
				dest, text.IdentityDecoder,
			)
			return ast.WalkContinue, nil
		}
		l, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		raw := string(l.Destination.Value(src))
		dest, ok, err := e.editionLink(i, raw, editions)
		if !ok && err == nil {
			dest, err = e.links.resolveOn(i, 0, raw, e.pages[i].base(), page.urlPath)
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

// editionLink returns dest's URL from page i of e when e's README link reaches
// a page in another edition: a displayed file in that edition's docs directory.
func (e *edition) editionLink(
	i int, dest string, editions []*edition,
) (string, bool, error) {
	u, ok := repoLink(dest)
	if !ok {
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

// releases returns feature versions under dir's versions directory, latest first.
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
