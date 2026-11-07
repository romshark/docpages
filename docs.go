package docpages

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

// page is a navigation entry: a repository Markdown file or, when url is set,
// an external link. A version's pages, paths, and links are rooted at
// <version>/ in the output directory.
type page struct {
	// file is the repository-relative Markdown path that the page shows and its
	// links use as their base.
	file string
	// stub is the repository-relative docs file whose front matter names file as
	// the source. Empty when the page shows its own file.
	stub  string
	path  string // URL path from the root of the version. Empty for its first page.
	url   string // Destination of a link that leaves the site.
	title string // Title in the navigation and in <title>.
	group int    // Index of the group of the navigation that lists the entry.
	// suggested makes search offer the page while its field is empty.
	suggested bool
}

// external reports whether p is a link that leaves the site.
func (p page) external() bool { return p.url != "" }

// outFile returns p's path relative to the version root. GitHub Pages serves a
// path without a trailing "/" from the corresponding ".html" file.
func (p page) outFile() string {
	if p.path == "" || strings.HasSuffix(p.path, "/") {
		return p.path + "index.html"
	}
	return p.path + ".html"
}

// markdownFile returns p's Markdown-copy path relative to the version root.
func (p page) markdownFile() string {
	return strings.TrimSuffix(p.outFile(), ".html") + ".md"
}

// markdownURL returns p's Markdown-copy URL relative to the version root,
// for links from other copies or llms.txt.
// It escapes [page.markdownFile] because a space ends a Markdown link.
func (p page) markdownURL() string {
	return (&url.URL{Path: p.markdownFile()}).EscapedPath()
}

// urlPath returns p's URL relative to the version root. Page links use it;
// Markdown-copy links use [page.markdownURL].
func (p page) urlPath() string { return p.path }

// base returns the URL of the root of the version relative to p.
func (p page) base() string {
	if n := strings.Count(p.path, "/"); n > 0 {
		return strings.Repeat("../", n)
	}
	return "./"
}

// shows reports whether p shows file, directly or as its source.
// A directory resolves to its README.md, as on GitHub.
func (p page) shows(file string) bool {
	if p.external() {
		return false
	}
	readme := path.Join(file, readmeName)
	return p.file == file || p.file == readme ||
		p.stub != "" && (p.stub == file || p.stub == readme)
}

// maxTOCDepth is the deepest heading level that the contents of a page list.
const maxTOCDepth = 4

// tocDepth returns the deepest contents level, up to [maxTOCDepth], before two
// headings below level 2 repeat. This avoids repeated entries such as Flags or
// Examples under each command in a reference.
func tocDepth(sections []section) int {
	depth := 2
	for level := 3; level <= maxTOCDepth; level++ {
		seen := make(map[string]bool)
		for _, s := range sections {
			if s.Level < 3 || s.Level > level {
				continue
			}
			if seen[s.Title] {
				return depth
			}
			seen[s.Title] = true
		}
		depth = level
	}
	return depth
}

// doc is the Markdown file of a [page] converted for [docsPage].
type doc struct {
	page        page
	title       string    // Text of the level 1 heading.
	description string    // Text of the first paragraph.
	sections    []section // Every heading, in the order of the page.
	depth       int       // The deepest heading level that the contents list.
	body        string
	// markdown is the page copy with links to other Markdown copies.
	// markdownFromRoot resolves those links from the version root for llms-full.txt.
	markdown         []byte
	markdownFromRoot []byte
	search           []searchEntry
}

// section is a heading of a [doc].
type section struct {
	ID    string
	Title string
	Level int
}

// tocEntry is a link of the contents of a page.
type tocEntry struct {
	Href  string
	Title string
	Level int
}

// contents returns d's level 1 heading, representing text above the first
// section, plus headings from level 2 through d.depth. It returns none if the
// page has no such sections.
func contents(d doc) []tocEntry {
	var toc []tocEntry
	for _, s := range d.sections {
		if s.Level >= 2 && s.Level <= d.depth {
			toc = append(toc, tocEntry{Href: "#" + s.ID, Title: s.Title, Level: s.Level})
		}
	}
	if len(toc) == 0 {
		return nil
	}
	if s := d.sections[0]; s.Level == 1 {
		toc = append([]tocEntry{{Href: "#" + s.ID, Title: s.Title, Level: 1}}, toc...)
	}
	return toc
}

// suggestion is a page that the search offers while its field is empty.
type suggestion struct {
	Group, Title string
	// Href is relative to the searching page, or an external URL.
	Href string
}

// suggestions returns the suggested pages of pages, linked from pages[current].
func suggestions(pages []page, groups []group, current int) []suggestion {
	var s []suggestion
	for _, p := range pages {
		if !p.suggested {
			continue
		}
		href := pages[current].base() + p.path
		if p.external() {
			href = p.url
		}
		s = append(s,
			suggestion{Group: groups[p.group].title, Title: p.title, Href: href})
	}
	return s
}

// navGroup is a group of the navigation.
type navGroup struct {
	Title   string
	Icon    string // SVG markup, empty without an icon.
	Entries []navEntry
}

// navEntry is a navigation, previous, or next link. Its zero value is no link.
type navEntry struct {
	Title    string
	Href     string
	Current  bool
	External bool
}

// navigation returns the navigation of pages as seen from pages[current].
func navigation(pages []page, groups []group, current int) []navGroup {
	var nav []navGroup
	last := -1
	for i, p := range pages {
		if p.group != last {
			last = p.group
			g := groups[p.group]
			nav = append(nav, navGroup{Title: g.title, Icon: g.icon})
		}
		e := navEntry{Title: p.title, Current: i == current}
		if p.external() {
			e.Href, e.External = p.url, true
		} else {
			e.Href = pages[current].base() + p.path
		}
		g := &nav[len(nav)-1]
		g.Entries = append(g.Entries, e)
	}
	return nav
}

// neighbors returns the pages before and after pages[current] in the navigation.
// Links that leave the site are skipped.
func neighbors(pages []page, current int) (prev, next navEntry) {
	link := func(p page) navEntry {
		return navEntry{Title: p.title, Href: pages[current].base() + p.path}
	}
	for i := current - 1; i >= 0; i-- {
		if !pages[i].external() {
			prev = link(pages[i])
			break
		}
	}
	for i := current + 1; i < len(pages); i++ {
		if !pages[i].external() {
			next = link(pages[i])
			break
		}
	}
	return prev, next
}

// headerEntry is a link of the header of a docs page.
type headerEntry struct {
	Title string
	Href  string
	Icon  string // SVG markup, empty without an icon.
	// Current means the link reaches this page, or reaches the first page when
	// no other header link reaches this page.
	Current bool
}

// footerEntry is a link of the footer of a docs page.
type footerEntry struct {
	Title string
	Href  string
	Icon  string // SVG markup, empty without an icon.
}

// docView is what [docsPage] shows of 1 page.
type docView struct {
	doc        doc
	chrome     *chrome
	nav        []navGroup
	toc        []tocEntry
	prev, next navEntry
	edition    *edition
	indexed    bool          // Whether search engines may index the page.
	versions   []versionLink // The versions that the header offers.
	// note is the edition-note HTML with page-relative links. Empty means no note.
	note      string
	suggested []suggestion
	header    []headerEntry // The links of the header before the repository.
	footer    []footerEntry // The links between the repository and llms.txt.
}

// base returns the URL of the root of the version relative to the page of v.
func (v docView) base() string { return v.doc.page.base() }

// home returns the URL that the logo leads to, relative to the page of v.
func (v docView) home() string {
	switch h := v.chrome.home; {
	case h == "":
		return v.base()
	case isAbsURL(h):
		return h
	default:
		return v.base() + "../" + h
	}
}

// sourceURL returns the page file's repository URL on main,
// which holds every edition's files.
func (v docView) sourceURL() string {
	return v.chrome.host.url(MainVersion, v.doc.page.file, false)
}

// isAbsURL reports whether u has a scheme or a host.
func isAbsURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme != "" || p.Host != "")
}

// convertDocs converts page Markdown in srcs to GitHub Flavored HTML with a heading ID
// on every heading. srcs[i] belongs to pages[i] and is nil for an external link.
// s supplies the repository-link Git ref and host.
// A page with no title uses its level 1 heading.
//
// Links resolve from their files, as on GitHub. A page-file link opens that page;
// any other repository-file link is rewritten to its Git-ref URL because
// the site does not contain the file.
//
// A fragment must name a page heading. goldmark derives IDs containing "_" or
// non-ASCII letters differently from GitHub, so an unchecked link could work
// on GitHub but fail here.
func convertDocs(pages []page, srcs [][]byte, s *site) ([]doc, error) {
	p := newParser(parser.WithAutoHeadingID())
	s.pages = pages
	s.ids = make([]map[string]bool, len(pages))
	docs := make([]doc, len(pages))
	nodes := make([]ast.Node, len(pages))
	// Resolve links after parsing all files; a link may name a later page's heading.
	pageRefs := make([]refs, len(pages))
	for i, src := range srcs {
		if pages[i].external() {
			continue
		}
		d := &docs[i]
		s.ids[i] = make(map[string]bool)
		ctx := parser.NewContext(parser.WithIDGenerator(goldmarkIDs{}))
		for _, id := range templateIDs {
			ctx.IDs().Put([]byte(id))
		}
		nodes[i] = p.Parse(src, parser.WithContext(ctx))
		if errs := lintMarkdown(nodes[i], src); len(errs) > 0 {
			return nil, lintErrors(s.fsys, pages[i].file, src, errs)
		}
		markCallouts(nodes[i], src)
		for n := nodes[i].FirstChild(); n != nil; n = n.NextSibling() {
			if para, ok := n.(*ast.Paragraph); ok && d.description == "" {
				d.description = plainText(para, src)
			}
		}
		_ = ast.Walk(nodes[i], func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			if h, ok := n.(*ast.Heading); ok {
				attr, _ := h.Attribute("id")
				id := attr.Value(src)
				s.ids[i][id] = true
				title := plainText(h, src)
				d.sections = append(d.sections,
					section{ID: id, Title: title, Level: h.Level})
				if h.Level == 1 && d.title == "" {
					d.title = title
				}
			}
			pageRefs[i].add(n)
			return ast.WalkContinue, nil
		})
		d.depth = tocDepth(d.sections)
		if pages[i].title == "" {
			if d.title == "" {
				return nil, fmt.Errorf(
					"%s has no level 1 heading and no title",
					pages[i].file,
				)
			}
			pages[i].title = d.title
		}
	}

	for i, src := range srcs {
		if pages[i].external() {
			continue
		}
		d := &docs[i]
		d.page = pages[i]
		// copyAt returns the page's Markdown copy as read from base, the version-root
		// URL. It rewrites the original destinations, before the HTML edits below.
		copyAt := func(base string) ([]byte, error) {
			return pageRefs[i].rewrite(src,
				func(dest string) (string, error) {
					return s.resolve(i, dest, base, page.markdownURL)
				},
				func(dest string) (string, error) {
					return s.mediaURL(i, dest, base)
				})
		}
		var err error
		if d.markdown, err = copyAt(pages[i].base()); err != nil {
			return nil, fmt.Errorf("%s: %w", pages[i].file, err)
		}
		if d.markdownFromRoot, err = copyAt(""); err != nil {
			return nil, fmt.Errorf("%s: %w", pages[i].file, err)
		}
		for _, l := range pageRefs[i].links {
			dest, err := s.resolveLink(i, l.Destination.Value(src))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", pages[i].file, err)
			}
			l.Destination = text.NewSingleLineValueFromString(dest, text.IdentityDecoder)
		}
		media := func(dest string) (string, error) {
			return s.mediaURL(i, dest, pages[i].base())
		}
		for _, img := range pageRefs[i].images {
			dest, err := media(img.Destination.Value(src))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", pages[i].file, err)
			}
			img.Destination = text.NewSingleLineValueFromString(
				dest, text.IdentityDecoder,
			)
		}
		d.search = searchEntries(*d, nodes[i], src)
		// The <details> of the page span the nodes of raw HTML, inline and blocks.
		details := &collapsibles{}
		r := html.New(
			html.WithExtensions(extension.NewGFMHTMLRenderer()),
			html.WithNodeRendererDecorator(ast.KindCodeBlock, codeBlocks()),
			html.WithNodeRendererDecorator(ast.KindHeading, headingLinks),
			html.WithNodeRendererDecorator(ast.KindBlockquote, callouts),
			html.WithNodeRendererDecorator(ast.KindRawHTML, rawHTML(details, media)),
			html.WithNodeRendererDecorator(ast.KindHTMLBlock, rawHTML(details, media)),
		)
		var body bytes.Buffer
		if err := r.Render(&body, src, nodes[i]); err != nil {
			return nil, fmt.Errorf("%s: %w", pages[i].file, err)
		}
		if err := details.done(); err != nil {
			return nil, fmt.Errorf("%s: %w", pages[i].file, err)
		}
		d.body = body.String()
	}
	return docs, nil
}

// templateIDs are IDs that [docsPage] writes around converted Markdown.
// goldmark suffixes a colliding heading ID, such as main-content-1 for "Main content",
// so skip links and scripts still target the template elements.
var templateIDs = []string{
	"main-content",
	"docs-nav",
	"docs-nav-toggle",
	"docs-toc-title",
	"search-results",
}

// goldmarkIDs uses goldmark's default ID generation. Parse keeps IDs added to a
// context only when that context names its own generator; otherwise it clears them.
type goldmarkIDs struct{}

func (goldmarkIDs) Generate(value []byte, kind ast.NodeKind) []byte {
	return parser.NewIDs().Generate(value, kind)
}

// site holds the heading IDs that links are resolved against.
type site struct {
	pages []page
	ids   []map[string]bool // The heading IDs of every page.
	ref   string            // The Git ref of the links to the repository.
	host  *host             // Where the repository is hosted.
	// root is a release's docs directory, which its links cannot leave; empty for main.
	root string
	// served holds linked repository files other than pages while [Handler]
	// serves them, and is nil otherwise. The handler replaces the host, which
	// would show Git versions rather than working-tree files.
	served map[string]bool
	fsys   fs.FS // The repository, which holds the media files.
	// media maps repository-relative media paths to the copies pages show.
	media map[string]mediaCopy
	// linked holds repository files and directories linked outside the pages.
	linked map[string]bool
}

// resolveLink returns the URL that the link dest has on the page with index from.
func (s *site) resolveLink(from int, dest string) (string, error) {
	return s.resolve(from, dest, s.pages[from].base(), page.urlPath)
}

// resolve returns dest's site URL from pages[from]. base is the version-root
// URL at the read location; target returns a page URL relative to that root.
func (s *site) resolve(
	from int, dest, base string, target func(page) string,
) (string, error) {
	return s.resolveOn(from, from, dest, base, target)
}

// resolveOn resolves dest from pages[from] as shown on page on.
// A self-link is only a fragment.
func (s *site) resolveOn(
	on, from int, dest, base string, target func(page) string,
) (string, error) {
	u, err := url.Parse(dest)
	if err != nil {
		return "", fmt.Errorf("link %q: %w", dest, err)
	}
	if u.Scheme != "" || u.Host != "" {
		return dest, nil
	}
	to := from
	if u.Path != "" {
		file := repoPath(s.pages[from].file, u.Path)
		if s.root != "" && !inDir(file, s.root) {
			return "", fmt.Errorf("link %q leads out of %s, "+
				"which a release links as a URL", dest, s.root)
		}
		if to = s.pageOf(file); to < 0 {
			return s.fileURL(file, strings.HasSuffix(u.Path, "/"), base, u), nil
		}
	}
	page := ""
	if to != on {
		page = base + target(s.pages[to])
	}
	switch {
	case u.Fragment == "":
		if page == "" {
			return "#", nil
		}
		return page, nil
	case !s.ids[to][u.Fragment]:
		return "", fmt.Errorf("link %q names no heading", dest)
	}
	return page + "#" + u.Fragment, nil
}

// target returns the page index reached by dest from pages[from], or -1.
func (s *site) target(from int, dest string) int {
	u, ok := repoLink(dest)
	if !ok {
		return -1
	}
	return s.pageOf(repoPath(s.pages[from].file, u.Path))
}

// repoLink parses dest and reports whether it names a repository file or
// directory: it must have a path but no scheme or host.
func repoLink(dest string) (*url.URL, bool) {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
		return nil, false
	}
	return u, true
}

// pageOf returns the index of the page that shows file, or -1.
func (s *site) pageOf(file string) int {
	return slices.IndexFunc(s.pages, func(p page) bool { return p.shows(file) })
}

// repoPath returns the repository-relative path named by p in file from.
func repoPath(from, p string) string {
	if strings.HasPrefix(p, "/") {
		return path.Clean(p[1:])
	}
	return path.Join(path.Dir(from), p)
}

// fileURL returns repository-relative file's URL, treating it as a directory
// when dir and preserving u's query and fragment. It uses the repository's Git-ref URL,
// or [Handler]'s URL while the handler serves linked files; then it also adds
// file to served. base is the version-root URL at the read location.
// It always adds file to linked.
func (s *site) fileURL(file string, dir bool, base string, u *url.URL) string {
	if s.linked == nil {
		s.linked = make(map[string]bool)
	}
	s.linked[file] = true
	var r string
	if s.served != nil {
		s.served[file] = true
		r = (&url.URL{Path: path.Join(base, "..", repositoryPath, file)}).EscapedPath()
		if dir {
			r += "/"
		}
	} else {
		r = s.host.url(s.ref, file, dir)
	}
	return withQueryFragment(r, u)
}

// withQueryFragment returns the URL r with the query and the fragment of u.
func withQueryFragment(r string, u *url.URL) string {
	if u.RawQuery != "" {
		r += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		r += "#" + u.Fragment
	}
	return r
}

// edit replaces the text at Index of a Markdown file.
type edit struct {
	text.Index
	text string
}

// newParser returns a GitHub Flavored Markdown parser with opts. Parsing pages,
// notes, and [Snapshot] copies alike keeps links and raw HTML in the same places.
func newParser(opts ...parser.Option) parser.Parser {
	return parser.New(append(opts, parser.WithExtensions(extension.NewGFMParser()))...)
}

// refs are Markdown nodes that name other files: links, images, and inline or
// block raw HTML containing media elements.
type refs struct {
	links  []*ast.Link
	images []*ast.Image
	raws   []ast.Node
}

// collectRefs returns the refs of the tree n.
func collectRefs(n ast.Node) refs {
	var r refs
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			r.add(n)
		}
		return ast.WalkContinue, nil
	})
	return r
}

// add adds n to r if it is a link, an image or raw HTML.
func (r *refs) add(n ast.Node) {
	switch n := n.(type) {
	case *ast.Link:
		r.links = append(r.links, n)
	case *ast.Image:
		r.images = append(r.images, n)
	case *ast.RawHTML, *ast.HTMLBlock:
		r.raws = append(r.raws, n)
	}
}

// rewrite returns r's source Markdown with each link destination replaced by
// link's result, and each image destination and raw-HTML media URL by media's.
//
// A reference definition shared by a link and image has one destination. It
// uses media's result because the image needs the file and the link can open it.
func (r refs) rewrite(
	src []byte, link, media func(string) (string, error),
) ([]byte, error) {
	var links, images []text.SingleLineValue
	for _, l := range r.links {
		links = append(links, l.Destination)
	}
	for _, img := range r.images {
		images = append(images, img.Destination)
	}
	linkEdits, err := destEdits(src, links, link)
	if err != nil {
		return nil, err
	}
	imageEdits, err := destEdits(src, images, media)
	if err != nil {
		return nil, err
	}
	shared := make(map[int]bool, len(imageEdits))
	for _, e := range imageEdits {
		shared[e.Start] = true
	}
	linkEdits = slices.DeleteFunc(linkEdits,
		func(e edit) bool { return shared[e.Start] })
	rawEdits, err := rawHTMLEdits(src, r.raws, media)
	if err != nil {
		return nil, err
	}
	return applyEdits(src, slices.Concat(linkEdits, imageEdits, rawEdits)), nil
}

// destEdits returns edits that replace each parsed link or image destination in
// src with resolve's result.
func destEdits(
	src []byte, dests []text.SingleLineValue, resolve func(dest string) (string, error),
) ([]edit, error) {
	var edits []edit
	// Links using one reference definition share a destination.
	seen := make(map[int]bool)
	for _, d := range dests {
		// An owned destination, such as one with escapes, has no source position
		// and keeps its text.
		if d.IsOwned() {
			continue
		}
		at := d.Indices()[0]
		if seen[at.Start] {
			continue
		}
		seen[at.Start] = true
		dest, err := resolve(d.Value(src))
		if err != nil {
			return nil, err
		}
		edits = append(edits, edit{Index: at, text: dest})
	}
	return edits, nil
}

// applyEdits returns src with edits, which do not overlap, applied.
func applyEdits(src []byte, edits []edit) []byte {
	slices.SortFunc(edits, func(a, b edit) int { return a.Start - b.Start })
	var b bytes.Buffer
	last := 0
	for _, e := range edits {
		b.Write(src[last:e.Start])
		b.WriteString(e.text)
		last = e.Stop
	}
	b.Write(src[last:])
	return b.Bytes()
}

// plainText returns the text of n without its inline markup.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	writePlainText(&b, n, src)
	return strings.TrimSpace(b.String())
}

// writePlainText writes n to w without inline markup. Line breaks and block endings,
// such as for list items or table cells, become spaces.
func writePlainText(w io.StringWriter, n ast.Node, src []byte) {
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if _, ok := n.(ast.BlockNode); ok {
				_, _ = w.WriteString(" ")
			}
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Text:
			_, _ = w.WriteString(n.Value.Value(src))
			if n.SoftLineBreak() || n.HardLineBreak() {
				_, _ = w.WriteString(" ")
			}
		case *ast.CodeSpan:
			_, _ = w.WriteString(n.Value.Value(src))
			return ast.WalkSkipChildren, nil
		case *ast.AutoLink:
			// An autolink holds its text in Label, not in a child.
			_, _ = w.WriteString(n.Label.Value(src))
		}
		return ast.WalkContinue, nil
	})
}
