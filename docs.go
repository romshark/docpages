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

// page is an entry of the navigation of the site: a page that shows a
// Markdown file of the repository or, with url set, a link that leaves the site.
// The pages of a version are under <version>/ of the output directory, which is
// the root that their paths and links are relative to.
type page struct {
	// file is the path from the repository root of the Markdown file that the
	// page shows, which its links are relative to.
	file string
	// stub is the path from the repository root of the file of the docs
	// directory whose front matter makes file the source of the page.
	// Empty for a page that shows its own file.
	stub  string
	path  string // URL path from the root of the version. Empty for its first page.
	url   string // Destination of a link that leaves the site.
	title string // Title in the navigation and in <title>.
	group int    // Index of the group of the navigation that lists the entry.
	// suggested marks a page that the search offers while its field is empty.
	suggested bool
}

// external reports whether p is a link that leaves the site.
func (p page) external() bool { return p.url != "" }

// outFile returns the path of the file that holds p, relative to the root of
// the version. GitHub Pages serves a path without "/" at the end from the file
// with ".html" appended.
func (p page) outFile() string {
	if p.path == "" || strings.HasSuffix(p.path, "/") {
		return p.path + "index.html"
	}
	return p.path + ".html"
}

// markdownFile returns the path of the Markdown copy of p,
// relative to the root of the version.
func (p page) markdownFile() string {
	return strings.TrimSuffix(p.outFile(), ".html") + ".md"
}

// urlPath returns the URL path of p, relative to the root of the version.
// It is what a link of a page leads to, as [page.markdownFile] is what a link
// of a Markdown copy leads to.
func (p page) urlPath() string { return p.path }

// base returns the URL of the root of the version relative to p.
func (p page) base() string {
	if n := strings.Count(p.path, "/"); n > 0 {
		return strings.Repeat("../", n)
	}
	return "./"
}

// shows reports whether p shows file, its own or that of its source.
// A directory leads to the page of its README.md, which GitHub shows for it.
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

// tocDepth returns the deepest heading level that the contents of a page with
// sections list: the deepest level up to [maxTOCDepth] at which no 2 headings
// below level 2 share their text. Repeated headings, such as Flags and
// Examples under each command of a reference, would repeat in the contents.
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
	// markdown is the Markdown copy of the page, whose links lead to the
	// Markdown copies of the other pages. markdownFromRoot links them as seen
	// from the root of the version, which is where llms-full.txt lives.
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

// contents returns the contents of d: the level 1 heading that opens it,
// which stands for the text above the first section, and its headings from
// level 2 down to its depth. A page without such sections has none.
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
	Href         string // Relative to the page that shows the search.
}

// suggestions returns the suggested pages of pages, linked from pages[current].
func suggestions(pages []page, groups []group, current int) []suggestion {
	var s []suggestion
	for _, p := range pages {
		if p.suggested {
			s = append(s, suggestion{
				Group: groups[p.group].title,
				Title: p.title,
				Href:  pages[current].base() + p.path,
			})
		}
	}
	return s
}

// navGroup is a group of the navigation.
type navGroup struct {
	Title   string
	Icon    string // SVG markup, empty without an icon.
	Entries []navEntry
}

// navEntry is a link of the navigation or a previous or next link.
// The zero navEntry is no link.
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
	// Current is whether the link leads to the page that shows it, or to the
	// first page while no other link of the header does.
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
	// note is the HTML of the note of the edition, with its links as seen from
	// the page. Empty without a note, which leaves the page without one.
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

// sourceURL returns the URL of the file of the page of v in the repository,
// on main, which holds the files of every edition.
func (v docView) sourceURL() string {
	return v.chrome.host.url(MainVersion, v.doc.page.file, false)
}

// isAbsURL reports whether u has a scheme or a host.
func isAbsURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme != "" || p.Host != "")
}

// convertDocs converts srcs, the Markdown files of pages, to HTML with GitHub
// Flavored Markdown and an ID on every heading. srcs[i] is the file of
// pages[i] and nil for a link that leaves the site. s gives the Git ref and the
// host of the links to the repository. A page without a title takes the text
// of its level 1 heading.
//
// A link is relative to the file that contains it, as on GitHub. A link to the
// file of a page leads to that page. A link to another file of the repository
// is rewritten to the file at the Git ref in the repository. The site contains
// none of these files.
//
// A link to a fragment must name a heading of its page. goldmark derives the ID
// of a heading with "_" or a non-ASCII letter differently from GitHub, which
// would leave a link that works on GitHub dead on the site.
func convertDocs(pages []page, srcs [][]byte, s *site) ([]doc, error) {
	p := newParser(parser.WithAutoHeadingID())
	s.pages = pages
	s.ids = make([]map[string]bool, len(pages))
	docs := make([]doc, len(pages))
	nodes := make([]ast.Node, len(pages))
	// Links are resolved after all files are parsed because a link can name a
	// heading of a page that is parsed later.
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
		// copyAt returns the Markdown copy of the page read from base, the URL
		// of the root of the version. The copies are rewritten from the
		// destinations as written, which the HTML below replaces.
		copyAt := func(base string) ([]byte, error) {
			return pageRefs[i].rewrite(src,
				func(dest string) (string, error) {
					return s.resolve(i, dest, base, page.markdownFile)
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

// templateIDs are the element IDs that [docsPage] writes around the converted Markdown.
// goldmark gives a heading that would take one of them a suffix,
// as main-content-1 for "Main content", which keeps the skip link
// and the scripts pointing at the template's elements.
var templateIDs = []string{
	"main-content",
	"docs-nav",
	"docs-nav-toggle",
	"docs-toc-title",
	"search-results",
}

// goldmarkIDs generates the IDs of goldmark's default generator.
// Parse keeps the IDs put into a context only when the context names
// its own generator, and replaces them with an empty set otherwise.
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
	// root is the docs directory of a release, which its links do not leave,
	// and empty for main.
	root string
	// served are the files of the repository that the links lead to, other
	// than those of the pages, while [Handler] serves them, and nil otherwise.
	// [Handler] serves them in place of the host, which shows them as Git has
	// them and not as they are in the working tree.
	served map[string]bool
	fsys   fs.FS // The repository, which holds the media files.
	// media are the copies of the media files that the pages show, by the
	// path of the file from the repository root.
	media map[string]mediaCopy
	// linked are the files and directories of the repository that the links
	// lead to, other than those of the pages.
	linked map[string]bool
}

// resolveLink returns the URL that the link dest has on the page with index from.
func (s *site) resolveLink(from int, dest string) (string, error) {
	return s.resolve(from, dest, s.pages[from].base(), page.urlPath)
}

// resolve returns the URL that the link dest of the file of pages[from] has on
// the site. base is the URL of the root of the version from where the URL is read,
// and target returns the URL of a page relative to that root.
func (s *site) resolve(
	from int, dest, base string, target func(page) string,
) (string, error) {
	return s.resolveOn(from, from, dest, base, target)
}

// resolveOn is resolve for the link dest of the file of pages[from] that the
// page with index on shows. A link to the page that shows it is a fragment.
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

// target returns the index of the page that the link dest of the file of
// pages[from] leads to, or -1 for a link that leads to no page.
func (s *site) target(from int, dest string) int {
	u, ok := repoLink(dest)
	if !ok {
		return -1
	}
	return s.pageOf(repoPath(s.pages[from].file, u.Path))
}

// repoLink returns the link dest parsed, and whether it names a file or a
// directory of the repository: whether it parses and has a path but neither
// a scheme nor a host.
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

// repoPath returns the path from the repository root of what the link path p
// names in the file from.
func repoPath(from, p string) string {
	if strings.HasPrefix(p, "/") {
		return path.Clean(p[1:])
	}
	return path.Join(path.Dir(from), p)
}

// fileURL returns the URL of file, a path from the repository root,
// of a directory if dir, with the query and the fragment of the link u: the URL of
// file at the Git ref in the repository, or, while [Handler] serves the files
// that the links lead to, the URL of the file it serves, which it adds to served.
// base is the URL of the root of the version from where the URL is read.
// It adds file to linked.
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

// newParser returns a parser of GitHub Flavored Markdown with opts. The pages,
// their notes and the copies of [Snapshot] are all parsed alike, which keeps
// their links and their raw HTML at the same places.
func newParser(opts ...parser.Option) parser.Parser {
	return parser.New(append(opts, parser.WithExtensions(extension.NewGFMParser()))...)
}

// refs are the nodes of a Markdown file that name other files: its links,
// its images and its raw HTML, inline and blocks, whose media elements do.
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

// rewrite returns src, the Markdown that r was parsed from, with the
// destination of each link replaced by what link returns for it, and that of
// each image and each URL of the media elements of the raw HTML by what media
// returns for it.
//
// A reference definition that a link and an image share holds 1 destination
// for both, which takes what media returns: the image needs the media file,
// and the link can lead to it as well.
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

// destEdits returns the edits of src that replace each of dests, the
// destinations of links or images parsed from src, by what resolve returns for it.
func destEdits(
	src []byte, dests []text.SingleLineValue, resolve func(dest string) (string, error),
) ([]edit, error) {
	var edits []edit
	// The links of 1 reference definition share its destination.
	seen := make(map[int]bool)
	for _, d := range dests {
		// An owned destination has no position in src, as for a destination
		// with escapes, and keeps its text.
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

// writePlainText writes the text of n without its inline markup to w.
// A line break becomes a space.
func writePlainText(w io.StringWriter, n ast.Node, src []byte) {
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
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
		}
		return ast.WalkContinue, nil
	})
}
