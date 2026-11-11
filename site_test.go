package docpages

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/require"
)

// testRoot is a test docs README front matter that sets every site option.
const testRoot = `---
name: Demo
home: ../
repository: https://github.com/o/demo
suggested: true
---

# Introduction

Demo is a web framework.
`

// testRepoFS returns a repository whose docs/ uses every layout convention.
func testRepoFS() fstest.MapFS {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	return fstest.MapFS{
		"LICENSE": file(dedent(`
			MIT License

			Copyright (c) 2026 Jane Doe

			Permission ...
		`)),
		"SPECIFICATION.md": file(dedent(`
			# Demo Specification

			## Pages

			### Actions
		`)),
		"CHANGELOG.md": file(dedent(`
			# Changelog

			## v1.0.0

			### Added

			## v0.9.0

			### Added
		`)),
		"main.go":        file("package demo\n"),
		"docs/README.md": file(testRoot),
		"docs/logo.svg": file(dedent(`
			<?xml version="1.0"?>
			<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10"/></svg>`)),
		"docs/favicon.svg": file(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"docs/style.css":   file("body{color:red}"),
		"docs/1-get-started/icon.svg": file(
			`<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="1"/></svg>`),
		"docs/1-get-started/1-installation.md": file(dedent(`
			---
			suggested: true
			---

			# Installation

			Run go install.
		`)),
		"docs/1-get-started/2-upgrading.md": file(dedent(`
			# Upgrading

			Read the [changelog](../../CHANGELOG.md).
		`)),
		"docs/2-guides/README.md": file(dedent(`
			---
			title: Overview
			---

			# Guides

			Start with [events](01-events.md).
		`)),
		"docs/2-guides/01-events.md": file(dedentMD(`
			# Events

			Events update tabs.

			## Dispatch

			See [actions](../../SPECIFICATION.md#actions), [the spec](../3-reference/1-specification.md) and [main.go](../../main.go).

			~~~go
			func f() {}
			~~~
		`)),
		"docs/2-guides/10-testing.md": file(dedent(`
			# Testing

			Test it.
		`)),
		"docs/3-reference/1-specification.md": file(dedent(`
			---
			source: ../../SPECIFICATION.md
			title: Specification
			---

			# Specification

			See [SPECIFICATION.md](../../SPECIFICATION.md).
		`)),
		"docs/3-reference/2-changelog.md": file(dedent(`
			---
			source: ../../CHANGELOG.md
			---

			# Changelog
		`)),
		"docs/3-reference/3-go-api.md": file(dedent(`
			---
			title: Go API
			url: https://pkg.go.dev/github.com/o/demo
			---

			# Go API
		`)),
		"docs/4-more/1-examples.md": file(dedent(`
			# Examples

			See the examples.
		`)),
	}
}

// dedent strips the leading line break and the common tab indent from an
// indented raw string. A tab-only last line keeps the final line break.
func dedent(s string) string {
	lines := strings.Split(strings.TrimPrefix(s, "\n"), "\n")
	indent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if n := len(l) - len(strings.TrimLeft(l, "\t")); indent < 0 || n < indent {
			indent = n
		}
	}
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			lines[i] = ""
		} else {
			lines[i] = l[indent:]
		}
	}
	return strings.Join(lines, "\n")
}

// dedentMD is [dedent] for Markdown, with each ~ turned into the backquote that
// a raw string can't hold.
func dedentMD(s string) string {
	return strings.ReplaceAll(dedent(s), "~", "`")
}

// noRemote is the Git remote of a repository without one.
func noRemote() (string, error) { return "", errors.New("no remote") }

// newTestSite returns [testRepoFS]'s release-free docs site using [testPage].
func newTestSite(t *testing.T) *Site {
	t.Helper()
	s, err := read(testRepoFS(), "docs", noRemote, nil, false)
	require.NoError(t, err)
	s.page = testPage
	return s
}

// testPage is [testRepoFS]'s template using every [Layout] part.
func testPage(p Page) templ.Component {
	return Layout(p, Parts{
		Head:   templ.Raw(`<meta name="test" content="head">`),
		Banner: templ.Raw(`<p class="test-banner">Upgrade</p>`),
		HeaderLinks: HeaderLinks(p,
			Link{Title: "Guide", URL: "README.md", Icon: testIcon("M2 2")},
			Link{Title: "Examples", URL: "4-more/1-examples.md"},
		),
		FooterLinks: FooterLinks(p,
			Link{Title: "Chat", URL: "https://chat.example.com", Icon: testIcon("M1 1")},
			Link{Title: "Changelog", URL: "3-reference/2-changelog.md"},
		),
		FooterEnd: templ.Raw("<span>MIT License © 2026 Jane Doe</span>"),
	})
}

// testIcon returns an icon drawing path d.
func testIcon(d string) templ.Component {
	return templ.Raw(
		`<svg aria-hidden="true" viewBox="0 0 16 16"><path d="` + d + `"/></svg>`,
	)
}

// testTags is a [tagStore] that holds the repository files at each tag.
type testTags map[string]fstest.MapFS

func (t testTags) tags() ([]string, error) { return slices.Sorted(maps.Keys(t)), nil }

func (t testTags) tree(tag, _ string) (fs.FS, error) {
	fsys, ok := t[tag]
	if !ok {
		return nil, errors.New("no tag " + tag)
	}
	return fsys, nil
}

// withReleases returns [testRepoFS] with releases from the feature version
// from, none if empty, and [testTags] that hold testRepoFS at each of tags.
func withReleases(from string, tags ...string) (fstest.MapFS, testTags) {
	fsys := testRepoFS()
	if from != "" {
		fsys["docs/README.md"] = &fstest.MapFile{Data: []byte(strings.Replace(testRoot,
			"suggested: true\n", "suggested: true\nreleases:\n  from: "+from+"\n", 1))}
	}
	t := make(testTags)
	for _, tag := range tags {
		t[tag] = testRepoFS()
	}
	return fsys, t
}

// pageHTML returns s's main-page HTML at URL path p.
func pageHTML(t *testing.T, s *Site, p string) string {
	t.Helper()
	return editionPageHTML(t, s, MainVersion, p)
}

// editionPageHTML returns s's HTML at path p in version.
func editionPageHTML(t *testing.T, s *Site, version, p string) string {
	t.Helper()
	i := slices.IndexFunc(s.editions, func(e *edition) bool {
		return e.version == version
	})
	require.GreaterOrEqual(t, i, 0, version)
	e := s.editions[i]
	j := slices.IndexFunc(e.pages, func(pg page) bool { return pg.path == p })
	require.GreaterOrEqual(t, j, 0, p)
	return render(t, s.page(s.pageView(e, j)))
}

// TestReadLayout checks prefix order, the docs README opening the first group,
// each group README opening its group, and prefix-free paths and titles.
func TestReadLayout(t *testing.T) {
	l, err := readLayout(testRepoFS(), "docs")
	require.NoError(t, err)
	require.Equal(t, []page{
		{file: "docs/README.md", path: "", suggested: true},
		{
			file:      "docs/1-get-started/1-installation.md",
			path:      "get-started/installation",
			suggested: true,
		},
		{file: "docs/1-get-started/2-upgrading.md", path: "get-started/upgrading"},
		{file: "docs/2-guides/README.md", path: "guides/", title: "Overview", group: 1},
		{file: "docs/2-guides/01-events.md", path: "guides/events", group: 1},
		{file: "docs/2-guides/10-testing.md", path: "guides/testing", group: 1},
		{
			file: "SPECIFICATION.md", stub: "docs/3-reference/1-specification.md",
			path: "reference/specification", title: "Specification", group: 2,
		},
		{
			file: "CHANGELOG.md", stub: "docs/3-reference/2-changelog.md",
			path: "reference/changelog", group: 2,
		},
		{
			file: "docs/3-reference/3-go-api.md", path: "reference/go-api",
			url: "https://pkg.go.dev/github.com/o/demo", title: "Go API", group: 2,
		},
		{file: "docs/4-more/1-examples.md", path: "more/examples", group: 3},
	}, l.pages)
	require.Equal(t, []string{"Get started", "Guides", "Reference", "More"},
		slices.Collect(func(yield func(string) bool) {
			for _, g := range l.groups {
				if !yield(g.title) {
					return
				}
			}
		}))
	require.Equal(t, `<svg aria-hidden="true" viewBox="0 0 24 24">`+
		`<circle cx="12" cy="12" r="1"/></svg>`, l.groups[0].icon)
	require.Empty(t, l.groups[1].icon)
	require.Equal(t, "Demo", l.settings.Name)
	require.Nil(t, l.srcs[8], "a link that leaves the site has no Markdown")
	require.Equal(t, dedent(`
		# Demo Specification

		## Pages

		### Actions
	`), string(l.srcs[6]))
	require.True(t, strings.HasPrefix(string(l.srcs[1]), "\n# Installation"),
		"the Markdown starts after the front matter")
}

// TestReadLayoutErr checks that incomplete navigation and contradictory front
// matter fail.
func TestReadLayoutErr(t *testing.T) {
	for name, tt := range map[string]struct {
		files map[string]string
		want  string
	}{
		"no README.md": {
			files: map[string]string{"docs/1-a/1-b.md": "# B\n"},
			want:  "open docs/README.md",
		},
		"no group": {
			files: map[string]string{"docs/README.md": "# A\n"},
			want:  "docs has no group directory",
		},
		"page without a prefix": {
			files: map[string]string{"docs/1-a/b.md": "# B\n"},
			want:  "docs/1-a/b.md: the name of a page starts with its position, as 1-b.md",
		},
		"shared position": {
			files: map[string]string{
				"docs/1-a/1-b.md":  "# B\n",
				"docs/1-a/01-c.md": "# C\n",
			},
			want: "docs/1-a/01-c.md and docs/1-a/1-b.md share the position 1",
		},
		"shared name": {
			files: map[string]string{
				"docs/1-a/1-b.md": "# B\n",
				"docs/1-a/2-b.md": "# B\n",
			},
			want: "docs/1-a/1-b.md and docs/1-a/2-b.md share the output file a/b.html",
		},
		"shared name in 2 groups": {
			files: map[string]string{
				"docs/1-a/1-b.md": "# B\n",
				"docs/2-a/1-b.md": "# B\n",
			},
			want: "docs/1-a/1-b.md and docs/2-a/1-b.md share the output file a/b.html",
		},
		"index next to README.md": {
			files: map[string]string{
				"docs/1-a/README.md":  "# A\n",
				"docs/1-a/1-index.md": "# I\n",
			},
			want: "docs/1-a/README.md and docs/1-a/1-index.md share the output file a/index.html",
		},
		"group in a group": {
			files: map[string]string{
				"docs/1-a/1-b.md":     "# B\n",
				"docs/1-a/2-c/1-d.md": "# D\n",
			},
			want: "docs/1-a/2-c: a group holds no groups",
		},
		"empty group": {
			files: map[string]string{
				"docs/1-a/1-b.md":   "# B\n",
				"docs/2-c/icon.svg": "<svg/>",
			},
			want: "group docs/2-c has no page",
		},
		"settings of a page": {
			files: map[string]string{
				"docs/1-a/1-b.md": dedent(`
					---
					name: B
					---
					# B
				`),
			},
			want: "docs/1-a/1-b.md: only the README.md of the docs directory takes site settings",
		},
		"url without a title": {
			files: map[string]string{
				"docs/1-a/1-b.md": dedent(`
					---
					url: https://x.dev
					---
				`),
			},
			want: "docs/1-a/1-b.md: a page with url needs a title",
		},
		"url and source": {
			files: map[string]string{
				"docs/1-a/1-b.md": dedent(`
					---
					url: https://x.dev
					source: ../../A.md
					title: B
					---
				`),
			},
			want: "docs/1-a/1-b.md: url and source exclude each other",
		},
		"source outside the repository": {
			files: map[string]string{
				"docs/1-a/1-b.md": dedent(`
					---
					source: ../../../A.md
					---
				`),
			},
			want: `docs/1-a/1-b.md: source "../../../A.md" is outside the repository`,
		},
		"missing source": {
			files: map[string]string{
				"docs/1-a/1-b.md": dedent(`
					---
					source: ../../A.md
					---
				`),
			},
			want: "docs/1-a/1-b.md: source: open A.md",
		},
		"unknown key": {
			files: map[string]string{
				"docs/1-a/1-b.md": dedent(`
					---
					titel: B
					---
				`),
			},
			want: "field titel not found",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fsys := fstest.MapFS{}
			if _, ok := tt.files["docs/README.md"]; !ok && name != "no README.md" {
				fsys["docs/README.md"] = &fstest.MapFile{Data: []byte("# A\n")}
			}
			for name, data := range tt.files {
				fsys[name] = &fstest.MapFile{Data: []byte(data)}
			}
			_, err := readLayout(fsys, "docs")
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// TestGroupTitle checks that group titles replace hyphens with spaces and title-case
// the first letter, including multibyte letters.
func TestGroupTitle(t *testing.T) {
	for slug, want := range map[string]string{
		"get-started": "Get started",
		"über-uns":    "Über uns",
		"élan":        "Élan",
		"日本語":         "日本語",
		"ǆungla":      "ǅungla",
		"1st-steps":   "1st steps",
	} {
		require.Equal(t, want, groupTitle(slug), slug)
	}
}

// TestParseFrontMatter checks front matter between two --- lines with any line ending,
// followed by Markdown.
func TestParseFrontMatter(t *testing.T) {
	for name, tt := range map[string]struct {
		data   string
		want   frontMatter
		wantMD string
	}{
		"none": {data: "# A\n", wantMD: "# A\n"},
		"thematic break later": {
			data: dedent(`
				# A

				---

				B
			`),
			wantMD: dedent(`
				# A

				---

				B
			`),
		},
		"empty": {
			data: dedent(`
				---
				---
				# A
			`),
			wantMD: "# A\n",
		},
		"fields": {
			data: dedent(`
				---
				title: A
				suggested: true
				---

				# B
			`),
			want: frontMatter{Title: "A", Suggested: true},
			wantMD: dedent(`

				# B
			`),
		},
		"CRLF": {
			data:   "---\r\ntitle: A\r\n---\r\n# B\r\n",
			want:   frontMatter{Title: "A"},
			wantMD: "# B\r\n",
		},
		"no Markdown": {
			data: dedent(`
				---
				title: A
				---`),
			want: frontMatter{Title: "A"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			fm, md, err := parseFrontMatter([]byte(tt.data))
			require.NoError(t, err)
			require.Equal(t, tt.want, fm)
			require.Equal(t, tt.wantMD, string(md))
		})
	}

	_, _, err := parseFrontMatter([]byte(dedent(`
		---
		title: A
	`)))
	require.ErrorContains(t, err, "front matter has no closing ---")
}

// TestHTTPSRepoURL checks conversion of SSH and HTTPS remotes to repository HTTPS.
func TestHTTPSRepoURL(t *testing.T) {
	for name, tt := range map[string]struct{ remote, want string }{
		"ssh": {
			remote: "git@github.com:o/demo.git",
			want:   "https://github.com/o/demo",
		},
		"https": {
			remote: "https://github.com/o/demo.git\n",
			want:   "https://github.com/o/demo",
		},
		"no suffix": {
			remote: "https://github.com/o/demo",
			want:   "https://github.com/o/demo",
		},
		"ssh scheme": {
			remote: "ssh://git@github.com/o/demo.git",
			want:   "https://github.com/o/demo",
		},
		"credentials": {
			remote: "https://x:token@github.com/o/demo.git",
			want:   "https://github.com/o/demo",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := httpsRepoURL(tt.remote)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
	_, err := httpsRepoURL("/srv/git/demo")
	require.ErrorContains(t, err, "has no host")
}

// TestInlineSVG checks removal before the SVG root and addition of missing root attrs.
func TestInlineSVG(t *testing.T) {
	for name, tt := range map[string]struct {
		data  string
		attrs []string
		want  string
	}{
		"prolog": {
			data: dedent(`
				<?xml version="1.0"?>
				<!-- c -->
				<svg viewBox="0 0 1 1"></svg>
			`),
			attrs: []string{`aria-hidden="true"`},
			want:  `<svg aria-hidden="true" viewBox="0 0 1 1"></svg>`,
		},
		"attribute set": {
			data:  `<svg aria-hidden="false" class="a"></svg>`,
			attrs: []string{`aria-hidden="true"`, `role="img"`},
			want:  `<svg role="img" aria-hidden="false" class="a"></svg>`,
		},
		"multiline tag": {
			data: dedent(`
				<svg
					class="a"
				>
				</svg>`),
			attrs: []string{`class="b"`},
			want: dedent(`
				<svg
					class="a"
				>
				</svg>`),
		},
		"not SVG": {data: "<png/>", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, inlineSVG([]byte(tt.data), tt.attrs...))
		})
	}
}

// TestSiteChrome checks rendered site settings and [Layout] parts: title name,
// home logo, head, banner, header and footer links and icons, current header
// link or first-page fallback, footer end, favicon, and docs stylesheet.
func TestSiteChrome(t *testing.T) {
	s := newTestSite(t)
	for name, tt := range map[string]struct {
		path          string
		want, wantNot []string
	}{
		"page in a group": {
			path: "guides/events",
			want: []string{
				"<title>Events - Demo</title>",
				`<link rel="icon" href="../favicon.svg" type="image/svg+xml">`,
				`<link rel="stylesheet" href="../docpages.css">`,
				`<link rel="stylesheet" href="../style.css">`,
				`<script type="module" src="../docpages.js"></script>` +
					`<meta name="test" content="head"></head>`,
				`Skip to content</a><p class="test-banner">Upgrade</p> ` +
					`<header class="site-header">`,
				`<a class="logo" href="../../../" aria-label="Demo home">` +
					`<svg class="logo-svg" role="img" aria-label="Demo" ` +
					`xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">`,
				`<a class="header-link icon-link" href="../" aria-current="page">` +
					`<svg aria-hidden="true" viewBox="0 0 16 16"><path d="M2 2"/></svg> Guide</a>`,
				`<a class="header-link" href="../more/examples">Examples</a>`,
				`<a class="icon-link" href="https://chat.example.com">` +
					`<svg aria-hidden="true" viewBox="0 0 16 16"><path d="M1 1"/></svg> Chat</a>`,
				`<a class="icon-link" href="../reference/changelog">Changelog</a>`,
				`<a class="icon-link" href="../llms.txt">`,
				"<span>MIT License © 2026 Jane Doe</span></footer>",
			},
			wantNot: []string{"_reload"},
		},
		"page of a header link": {
			path: "more/examples",
			want: []string{
				`<a class="header-link icon-link" href="../">` +
					`<svg aria-hidden="true" viewBox="0 0 16 16"><path d="M2 2"/></svg> Guide</a>`,
				`<a class="header-link" href="../more/examples" aria-current="page">Examples</a>`,
			},
		},
		"first page": {
			path: "",
			want: []string{
				`<a class="logo" href="./../../"`,
				`<link rel="icon" href="./favicon.svg"`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			html := pageHTML(t, s, tt.path)
			for _, want := range tt.want {
				require.Contains(t, html, want)
			}
			for _, notWant := range tt.wantNot {
				require.NotContains(t, html, notWant)
			}
		})
	}
}

// TestSiteDefaults checks unset chrome: the repository name, a text logo linking
// the first page, a "Docs" header link there, and no favicon, stylesheet, or license.
func TestSiteDefaults(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md": {Data: []byte(dedent(`
			# Intro

			A tool.
		`))},
		"1-a/1-b.md":   {Data: []byte("# B\n")},
		"1-a/2-c.md":   {Data: []byte("# C\n")},
		"unrelated.go": {Data: []byte("package x\n")},
	}
	s, err := read(fsys, ".", func() (string, error) {
		return "git@github.com:o/tool.git", nil
	}, nil, false)
	require.NoError(t, err)
	html := pageHTML(t, s, "a/b")
	for _, want := range []string{
		"<title>B - tool</title>",
		`<a class="logo" href="../" aria-label="tool home"><span class="logo-text">tool</span></a>`,
		`<a class="header-link" href="../" aria-current="page">Docs</a>`,
		`<a class="header-repository icon-link" href="https://github.com/o/tool">`,
		`<a class="icon-link" href="https://github.com/o/tool/blob/main/1-a/1-b.md">`,
	} {
		require.Contains(t, html, want)
	}
	for _, notWant := range []string{
		`rel="icon"`, `href="../style.css"`, "License", "header-link\" href=\"../a",
	} {
		require.NotContains(t, html, notWant)
	}

	_, err = read(fsys, ".", noRemote, nil, false)
	require.ErrorContains(t, err, "set repository in the front matter of README.md")
}

// TestLayoutParts checks that [Layout] without parts omits header links, footer
// links between the repository and llms.txt, and the footer end.
func TestLayoutParts(t *testing.T) {
	s := newTestSite(t)
	s.page = func(p Page) templ.Component { return Layout(p, Parts{}) }
	html := pageHTML(t, s, "guides/events")
	require.NotContains(t, html, "header-link")
	require.Regexp(t, `(?s)<nav class="footer-links" aria-label="Project">`+
		`<a class="icon-link" href="https://github.com/o/demo">.*?</a>`+
		`<a class="icon-link" href="../llms.txt">.*?</a></nav></footer>`, html)
}

// TestSiteLinks checks links to named-file pages, direct or stubbed sources, and
// other GitHub files, plus failure for template links to a missing heading or file.
func TestSiteLinks(t *testing.T) {
	s := newTestSite(t)
	html := pageHTML(t, s, "guides/events")
	for _, want := range []string{
		`<a href="../reference/specification#actions">actions</a>`,
		`<a href="../reference/specification">the spec</a>`,
		`<a href="https://github.com/o/demo/blob/main/main.go">main.go</a>`,
	} {
		require.Contains(t, html, want)
	}
	require.Contains(t, pageHTML(t, s, "get-started/upgrading"),
		`<a href="../reference/changelog">changelog</a>`)

	s.page = func(p Page) templ.Component {
		return Layout(p, Parts{HeaderLinks: HeaderLinks(p,
			Link{Title: "Examples", URL: "4-more/1-examples.md#missing"})})
	}
	_, err := s.Files()
	require.ErrorContains(t, err, `header link "Examples": `+
		`link "4-more/1-examples.md#missing" names no heading`)
	_, err = Page{site: s, edition: s.main()}.URL("4-more/1-examples.md#missing")
	require.EqualError(t, err, `link "4-more/1-examples.md#missing" names no heading`)

	s.page = func(p Page) templ.Component {
		return Layout(p, Parts{FooterLinks: FooterLinks(p,
			Link{Title: "Examples", URL: "4-more/9-renamed.md"})})
	}
	_, err = s.Files()
	require.ErrorContains(t, err, `footer link "Examples": `+
		`link "4-more/9-renamed.md": no file docs/4-more/9-renamed.md`)
	_, err = Page{site: s, edition: s.main()}.URL("../LICENSE")
	require.NoError(t, err)
}

// TestLinkOutOfRepository checks that a link above the repository root fails on disk,
// where os.DirFS reports the path as invalid rather than missing.
func TestLinkOutOfRepository(t *testing.T) {
	root := writeTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "4-more", "1-examples.md"),
		[]byte(dedent(`
			# Examples

			See [outside](../../../outside.md).
		`)), 0o644))
	_, err := Read(Options{Root: root, Dir: "docs"})
	require.EqualError(t, err, `docs/4-more/1-examples.md: `+
		`link "../../../outside.md": no file ../outside.md`)
}

// TestPageNameWithSpace checks that Markdown copies, llms-full.txt, and llms.txt
// escape a spaced page URL because a space ends a Markdown link.
func TestPageNameWithSpace(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/README.md": {
			Data: []byte(dedent(`
				---
				repository: https://github.com/o/demo
				---

				# Intro

				See [the first steps](1-guide/1-first%20steps.md).
			`)),
		},
		"docs/1-guide/1-first steps.md": {
			Data: []byte(dedent(`
				# First steps

				Run it.
			`)),
		},
	}
	s, err := read(fsys, "docs", noRemote, nil, false)
	require.NoError(t, err)
	files, err := s.Files()
	require.NoError(t, err)
	require.Contains(t, files, "main/guide/first steps.md")
	require.Contains(t, string(files["main/index.md"]),
		"[the first steps](./guide/first%20steps.md)")
	require.Contains(t, string(files["main/llms-full.txt"]),
		"[the first steps](guide/first%20steps.md)")
	require.Contains(t, string(files["main/llms.txt"]),
		"- [First steps](guide/first%20steps.md): Run it.")
}

// TestSiteFiles checks edition files and assets under main/ or <feature-version>/,
// root index.html and llms.txt, and release pages using sources at their tag.
func TestSiteFiles(t *testing.T) {
	fsys, tags := withReleases("v1.2", "v1.2.3")
	s, err := read(fsys, "docs", noRemote, tags, false)
	require.NoError(t, err)
	files, err := s.Files()
	require.NoError(t, err)
	var outside []string
	for name := range files {
		if !strings.HasPrefix(name, "main/") && !strings.HasPrefix(name, "v1.2/") {
			outside = append(outside, name)
		}
	}
	slices.Sort(outside)
	require.Equal(t, []string{"index.html", "llms.txt"}, outside)
	for _, dir := range []string{"main/", "v1.2/"} {
		for _, name := range []string{
			"index.html", "index.md", "guides/index.html", "guides/events.html",
			"guides/events.md", "reference/specification.html", "search.json",
			"llms.txt", "llms-full.txt", "docpages.css", "docpages.js",
			"morpheus/morpheus.min.js", "morpheus/morpheus.min.css",
			"morpheus/icons/chevrons-up-down.svg", "favicon.svg", "style.css",
		} {
			require.Contains(t, files, dir+name)
		}
		require.NotContains(t, files, dir+"reference/go-api.html")
		// esbuild's minified docs style.css.
		require.Equal(t, []byte("body{color:red}\n"), files[dir+"style.css"])
		// A page copy starts after its front matter.
		require.Equal(t, dedent(`
			# Demo Specification

			## Pages

			### Actions
		`), strings.TrimLeft(string(files[dir+"reference/specification.md"]), "\n"))
	}
}

// TestWrite checks that a run removes stale generated files,
// including removed releases, but keeps other output files.
func TestWrite(t *testing.T) {
	out := t.TempDir()
	for _, name := range []string{
		"v0.9/index.html",
		"main/old.html",
		"index.html",
		"landing.html",
	} {
		p := filepath.Join(out, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("old"), 0o644))
	}
	require.NoError(t, newTestSite(t).Write(out))
	for _, name := range []string{"v0.9", "main/old.html"} {
		_, err := os.Stat(filepath.Join(out, name))
		require.ErrorIs(t, err, fs.ErrNotExist, name)
	}
	for _, name := range []string{"landing.html", "main/guides/events.html"} {
		_, err := os.Stat(filepath.Join(out, filepath.FromSlash(name)))
		require.NoError(t, err, name)
	}
	index, err := os.ReadFile(filepath.Join(out, "index.html"))
	require.NoError(t, err)
	require.Contains(t, string(index), `url=main/`)
}

// TestSearchSuggestions checks that search groups front-matter suggestions.
func TestSearchSuggestions(t *testing.T) {
	html := pageHTML(t, newTestSite(t), "guides/events")
	require.Regexp(t, `<a href="../" tabindex="-1">`+
		`<span class="result-page">Get started</span>\s*<span class="result-section">Introduction</span>`,
		html)
	require.Contains(t, html, `<a href="../get-started/installation" tabindex="-1">`)
	require.Equal(t, 2, strings.Count(html, `id="search_suggestion_`))
}

// TestStaticAssets checks that all loaded assets and [Handler]-served sources
// are embedded, with a minified script.
func TestStaticAssets(t *testing.T) {
	assets, err := siteAssets()
	require.NoError(t, err)
	names := slices.Sorted(maps.Keys(assets))
	require.Equal(t, []string{
		"docpages.css", "docpages.js",
		"morpheus/icons/arrow-up-right.svg",
		"morpheus/icons/chevron-down.svg",
		"morpheus/icons/chevrons-up-down.svg",
		"morpheus/icons/file.svg",
		"morpheus/icons/github.svg",
		"morpheus/icons/markdown.svg",
		"morpheus/icons/menu.svg",
		"morpheus/icons/monitor.svg",
		"morpheus/icons/moon.svg",
		"morpheus/icons/search.svg",
		"morpheus/icons/sun.svg",
		"morpheus/morpheus.min.css", "morpheus/morpheus.min.js",
	}, names)
	min, err := static.ReadFile("static/docs.min.js")
	require.NoError(t, err)
	require.Equal(t, string(min), string(assets["docpages.js"]))
	for _, source := range []string{"static/style.css", "static/docs.js"} {
		_, err = fs.Stat(static, source)
		require.NoError(t, err)
	}
}

// TestIcons checks that every template icon exists and is published under
// morpheus/icons, and that site and Morpheus icon names do not overlap.
func TestIcons(t *testing.T) {
	templates, err := filepath.Glob("*.templ")
	require.NoError(t, err)
	var names []string
	for _, name := range templates {
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		for _, m := range regexp.MustCompile(`@icon\("([^"]+)"\)`).
			FindAllSubmatch(src, -1) {
			names = append(names, string(m[1]))
		}
	}
	require.NotEmpty(t, names)
	assets, err := siteAssets()
	require.NoError(t, err)
	for _, name := range names {
		svg, err := fs.ReadFile(icons, name+".svg")
		require.NoError(t, err)
		require.Equal(t, svg, assets["morpheus/icons/"+name+".svg"], name)
	}
	own, err := fs.ReadDir(static, iconDirs[0])
	require.NoError(t, err)
	for _, e := range own {
		_, err := fs.Stat(static, path.Join(iconDirs[1], e.Name()))
		require.ErrorIs(t, err, fs.ErrNotExist, e.Name())
	}
}

// TestDocsDirectories checks that docpages and example docs
// build with their .templ files.
func TestDocsDirectories(t *testing.T) {
	for _, dir := range []string{"docs", "example/cli/docs", "example/library/docs"} {
		t.Run(dir, func(t *testing.T) {
			s, err := readOptions(Options{Root: ".", Dir: dir}, false)
			require.NoError(t, err)
			require.Nil(t, s.page, "the .templ files render the pages")
			files, err := s.Files()
			require.NoError(t, err)
			require.Contains(t, string(files["main/index.html"]), `class="header-link"`)
		})
	}
}
