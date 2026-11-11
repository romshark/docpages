package docpages

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/a-h/templ"
)

// Page is one site page as shown by a template.
// Its page-relative URLs work in any folder.
type Page struct {
	// Title appears in navigation and the browser tab.
	// It is the front-matter title or level 1 heading.
	Title string
	// Heading is the text of the level 1 heading.
	Heading string
	// Description is the text of the first paragraph.
	Description string
	// Body is the HTML of the Markdown of the page.
	Body string
	// Note is the version-note HTML, empty if unset.
	Note string
	// Site is the name of the project.
	Site string
	// Version is "main" or a release such as "v1.2".
	Version string
	// Root is the version-folder URL, such as "../".
	Root string
	// Home is the URL that the logo leads to.
	Home string
	// Logo is logo.svg markup, empty if absent.
	Logo string
	// Markdown is the URL of the Markdown copy of the page.
	Markdown string
	// Source is the page file's URL on the repository host: at the tag of a
	// release, or on main for main and for a file of a release folder.
	Source string
	// Repository is the repository URL; Host is its host name, such as "GitHub".
	Repository, Host string
	Navigation       []NavGroup
	// Contents lists page sections; it is empty if there are none.
	Contents []ContentsLink
	// Prev and Next are adjacent navigation pages; a zero NavLink means none.
	Prev, Next NavLink
	// Versions lists releases latest-first, then main.
	Versions []VersionLink
	// Suggested lists pages offered before a search query.
	Suggested []Suggestion
	// Indexed reports whether search engines may index the page.
	Indexed bool

	site    *Site
	edition *edition
	index   int // Of the page in edition.
}

// NavGroup is a group of the navigation.
type NavGroup struct {
	Title string
	Icon  string // SVG markup, empty without an icon.
	Links []NavLink
}

// NavLink is a navigation, previous, or next link.
type NavLink struct {
	Title    string
	Href     string
	Current  bool // Whether the link leads to the page that shows it.
	External bool // Whether the link leaves the site.
}

// ContentsLink is a link of the contents of a page.
type ContentsLink struct {
	Href  string
	Title string
	Level int // Of the heading, 1 for the level 1 heading.
}

// VersionLink is a version in the version menu.
type VersionLink struct {
	Version string
	Current bool // Whether the page belongs to this version.
	// Href is the same page in this version.
	Href string
	// Fallback is this version's first page, opened by docs.js if the page is absent.
	Fallback string
}

// Suggestion is a page offered before a search query.
type Suggestion struct {
	Group, Title string
	// Href leads to the page or outside the site.
	Href string
}

// URL resolves dest from this page. dest uses docs/README.md link syntax: a docs
// Markdown file such as "2-guide/1-events.md#dispatch", another repository file
// (resolved on its host), or a URL. A main page missing from a release falls
// back to main. A missing destination file or heading fails the build.
func (p Page) URL(dest string) (string, error) {
	href, _, err := p.resolve(dest)
	return href, err
}

// resolve returns dest's URL from p and its page index in p's edition, or -1.
func (p Page) resolve(dest string) (string, int, error) {
	if p.site == nil {
		return "", -1, fmt.Errorf("link %q: the page belongs to no site", dest)
	}
	main := p.site.main()
	if _, err := main.links.resolve(0, dest, "", page.urlPath); err != nil {
		return "", -1, err
	}
	href, to := p.site.chromeLink(p.edition, p.index, dest)
	return href, to, nil
}

// Link is a link of the header or the footer. See [HeaderLinks] and [FooterLinks].
type Link struct {
	Title string
	// URL is accepted by [Page.URL], such as "README.md" or an absolute URL.
	URL string
	// Icon appears before the title, such as a template <svg>. Nil means none.
	Icon templ.Component
}

// linkView is a page-resolved [Link].
type linkView struct {
	Title, Href string
	Icon        templ.Component
	// Current means the link reaches this page, or reaches the first page when no
	// other list link reaches this page.
	Current bool
}

// HeaderLinks shows [Layout] header links resolved by [Page.URL].
// On each page, it marks the link leading there, or a first-page link if none does.
func HeaderLinks(p Page, links ...Link) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		views, err := p.linkViews("header", links)
		if err != nil {
			return err
		}
		return headerLinks(views).Render(ctx, w)
	})
}

// FooterLinks shows [Page.URL]-resolved [Layout] footer links between the
// repository and llms.txt.
func FooterLinks(p Page, links ...Link) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		views, err := p.linkViews("footer", links)
		if err != nil {
			return err
		}
		return footerLinks(views).Render(ctx, w)
	})
}

// linkViews returns links for page part as p displays them.
func (p Page) linkViews(part string, links []Link) ([]linkView, error) {
	var views []linkView
	var first []int
	for _, l := range links {
		href, to, err := p.resolve(l.URL)
		if err != nil {
			return nil, fmt.Errorf("%s link %q: %w", part, l.Title, err)
		}
		if to == 0 {
			first = append(first, len(views))
		}
		views = append(views,
			linkView{Title: l.Title, Href: href, Icon: l.Icon, Current: to == p.index})
	}
	if !slices.ContainsFunc(views, func(v linkView) bool { return v.Current }) {
		for _, k := range first {
			views[k].Current = true
		}
	}
	return views, nil
}

// Parts are template additions to [Layout]. Nil parts add nothing.
type Parts struct {
	// Head is appended to <head>, for example with template fonts or scripts.
	Head templ.Component
	// Banner opens <body> above the header, for example with a notice of a release.
	Banner templ.Component
	// HeaderLinks precedes the repository link, as written by [HeaderLinks].
	HeaderLinks templ.Component
	// FooterLinks sits between the repository and llms.txt, as written by [FooterLinks].
	FooterLinks templ.Component
	// FooterEnd appears at the end of the footer, for example with a license.
	FooterEnd templ.Component
}
