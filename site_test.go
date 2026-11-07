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

	"github.com/stretchr/testify/require"
)

// testRoot is the front matter of the README.md of the test docs directory,
// which sets every setting of the site.
const testRoot = `---
name: Demo
home: ../
repository: https://github.com/o/demo
suggested: true
header:
  - title: Guide
    url: README.md
    icon: icons/book.svg
  - title: Examples
    url: 4-more/1-examples.md
footer:
  - title: Chat
    url: https://chat.example.com
    icon: icons/chat.svg
  - title: Changelog
    url: 3-reference/2-changelog.md
license: MIT License © 2026 Jane Doe
---

# Introduction

Demo is a web framework.
`

// testRepoFS returns a repository with a docs directory, docs/, that uses
// every convention of the layout.
func testRepoFS() fstest.MapFS {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	return fstest.MapFS{
		"LICENSE":          file("MIT License\n\nCopyright (c) 2026 Jane Doe\n\nPermission ...\n"),
		"SPECIFICATION.md": file("# Demo Specification\n\n## Pages\n\n### Actions\n"),
		"CHANGELOG.md":     file("# Changelog\n\n## v1.0.0\n\n### Added\n\n## v0.9.0\n\n### Added\n"),
		"docs/README.md":   file(testRoot),
		"docs/logo.svg": file(`<?xml version="1.0"?>` + "\n" +
			`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10"/></svg>`),
		"docs/favicon.svg":    file(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"docs/style.css":      file("body{color:red}"),
		"docs/icons/chat.svg": file(`<svg viewBox="0 0 16 16"><path d="M1 1"/></svg>`),
		"docs/icons/book.svg": file(`<svg viewBox="0 0 16 16"><path d="M2 2"/></svg>`),
		"docs/1-get-started/icon.svg": file(
			`<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="1"/></svg>`),
		"docs/1-get-started/1-installation.md": file("---\nsuggested: true\n---\n\n" +
			"# Installation\n\nRun go install.\n"),
		"docs/1-get-started/2-upgrading.md": file("# Upgrading\n\nRead the [changelog](../../CHANGELOG.md).\n"),
		"docs/2-guides/README.md": file("---\ntitle: Overview\n---\n\n# Guides\n\n" +
			"Start with [events](01-events.md).\n"),
		"docs/2-guides/01-events.md": file("# Events\n\nEvents update tabs.\n\n" +
			"## Dispatch\n\nSee [actions](../../SPECIFICATION.md#actions), " +
			"[the spec](../3-reference/1-specification.md) and [main.go](../../main.go).\n\n" +
			"```go\nfunc f() {}\n```\n"),
		"docs/2-guides/10-testing.md": file("# Testing\n\nTest it.\n"),
		"docs/3-reference/1-specification.md": file("---\nsource: ../../SPECIFICATION.md\n" +
			"title: Specification\n---\n\n# Specification\n\nSee [SPECIFICATION.md](../../SPECIFICATION.md).\n"),
		"docs/3-reference/2-changelog.md": file("---\nsource: ../../CHANGELOG.md\n---\n\n# Changelog\n"),
		"docs/3-reference/3-go-api.md": file("---\ntitle: Go API\n" +
			"url: https://pkg.go.dev/github.com/o/demo\n---\n\n# Go API\n"),
		"docs/4-more/1-examples.md": file("# Examples\n\nSee the examples.\n"),
	}
}

// noRemote is the Git remote of a repository without one.
func noRemote() (string, error) { return "", errors.New("no remote") }

// newTestSite returns the site of the docs directory of [testRepoFS], which
// holds no releases.
func newTestSite(t *testing.T) *Site {
	t.Helper()
	s, err := read(testRepoFS(), "docs", noRemote, false)
	require.NoError(t, err)
	return s
}

// withReleases returns fsys with a snapshot of its docs directory, docs/, for
// each of tags.
func withReleases(t *testing.T, fsys fstest.MapFS, tags ...string) fstest.MapFS {
	t.Helper()
	for _, tag := range tags {
		_, files, err := snapshot(fsys, "docs", tag, noRemote)
		require.NoError(t, err)
		for name, data := range files {
			fsys[name] = &fstest.MapFile{Data: data}
		}
	}
	return fsys
}

// pageHTML returns the HTML of the page of main of s at the URL path p.
func pageHTML(t *testing.T, s *Site, p string) string {
	t.Helper()
	return editionPageHTML(t, s, MainVersion, p)
}

// editionPageHTML returns the HTML of the page of s at the URL path p in the
// edition of version.
func editionPageHTML(t *testing.T, s *Site, version, p string) string {
	t.Helper()
	i := slices.IndexFunc(s.editions, func(e *edition) bool { return e.version == version })
	require.GreaterOrEqual(t, i, 0, version)
	e := s.editions[i]
	j := slices.IndexFunc(e.pages, func(pg page) bool { return pg.path == p })
	require.GreaterOrEqual(t, j, 0, p)
	return render(t, docsPage(s.view(e, j)))
}

// TestReadLayout tests that the docs directory yields its pages in the order of
// the prefixes, the README.md of the docs directory opening the first group and
// that of a group opening it, with URL paths and titles without the prefixes.
func TestReadLayout(t *testing.T) {
	l, err := readLayout(testRepoFS(), "docs")
	require.NoError(t, err)
	require.Equal(t, []page{
		{file: "docs/README.md", path: "", suggested: true},
		{file: "docs/1-get-started/1-installation.md", path: "get-started/installation", suggested: true},
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
	require.Equal(t, "# Demo Specification\n\n## Pages\n\n### Actions\n", string(l.srcs[6]))
	require.True(t, strings.HasPrefix(string(l.srcs[1]), "\n# Installation"),
		"the Markdown starts after the front matter")
}

// TestReadLayoutErr tests that a docs directory that the navigation cannot
// show completely or a front matter that contradicts itself is an error.
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
				"docs/1-a/1-b.md": "---\nname: B\n---\n# B\n",
			},
			want: "docs/1-a/1-b.md: only the README.md of the docs directory takes site settings",
		},
		"url without a title": {
			files: map[string]string{
				"docs/1-a/1-b.md": "---\nurl: https://x.dev\n---\n",
			},
			want: "docs/1-a/1-b.md: a page with url needs a title",
		},
		"url and source": {
			files: map[string]string{
				"docs/1-a/1-b.md": "---\nurl: https://x.dev\nsource: ../../A.md\ntitle: B\n---\n",
			},
			want: "docs/1-a/1-b.md: url and source exclude each other",
		},
		"source outside the repository": {
			files: map[string]string{
				"docs/1-a/1-b.md": "---\nsource: ../../../A.md\n---\n",
			},
			want: `docs/1-a/1-b.md: source "../../../A.md" is outside the repository`,
		},
		"missing source": {
			files: map[string]string{
				"docs/1-a/1-b.md": "---\nsource: ../../A.md\n---\n",
			},
			want: "docs/1-a/1-b.md: source: open A.md",
		},
		"unknown key": {
			files: map[string]string{"docs/1-a/1-b.md": "---\ntitel: B\n---\n"},
			want:  "field titel not found",
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

// TestGroupTitle tests that the title of a group has spaces for the hyphens of
// its directory name and its first letter in title case, also when that letter
// takes more than 1 byte.
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

// TestParseFrontMatter tests that front matter opens a file between 2 lines of
// ---, with any line ending, and that the Markdown follows it.
func TestParseFrontMatter(t *testing.T) {
	for name, tt := range map[string]struct {
		data   string
		want   frontMatter
		wantMD string
	}{
		"none":                 {data: "# A\n", wantMD: "# A\n"},
		"thematic break later": {data: "# A\n\n---\n\nB\n", wantMD: "# A\n\n---\n\nB\n"},
		"empty":                {data: "---\n---\n# A\n", wantMD: "# A\n"},
		"fields": {
			data:   "---\ntitle: A\nsuggested: true\n---\n\n# B\n",
			want:   frontMatter{Title: "A", Suggested: true},
			wantMD: "\n# B\n",
		},
		"CRLF": {
			data:   "---\r\ntitle: A\r\n---\r\n# B\r\n",
			want:   frontMatter{Title: "A"},
			wantMD: "# B\r\n",
		},
		"no Markdown": {
			data: "---\ntitle: A\n---",
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

	_, _, err := parseFrontMatter([]byte("---\ntitle: A\n"))
	require.ErrorContains(t, err, "front matter has no closing ---")
}

// TestSiteLicense tests that the footer ends with the license setting as
// written, that without it the footer has no such line although the repository
// has a LICENSE file, and that the README.md of a release can't set it.
func TestSiteLicense(t *testing.T) {
	for name, tt := range map[string]struct {
		setting string // The line of the front matter, empty for none.
		want    string // The end of the footer.
	}{
		"set":   {`license: " © 2026 Demo Inc. All rights reserved. "`, "<span>© 2026 Demo Inc. All rights reserved.</span></footer>"},
		"unset": {"", "llms.txt</a></nav></footer>"},
		"empty": {`license: ""`, "llms.txt</a></nav></footer>"},
	} {
		t.Run(name, func(t *testing.T) {
			fsys := testRepoFS()
			fsys["docs/README.md"] = &fstest.MapFile{Data: []byte(strings.Replace(
				testRoot, "license: MIT License © 2026 Jane Doe\n", tt.setting+"\n", 1))}
			s, err := read(fsys, "docs", noRemote, false)
			require.NoError(t, err)
			require.Contains(t, pageHTML(t, s, ""), tt.want)
		})
	}

	fsys := withReleases(t, testRepoFS(), "v1.2.0")
	fsys["docs/versions/v1.2/README.md"] = &fstest.MapFile{
		Data: []byte("---\nlicense: MIT\n---\n\n# Introduction\n\nDemo.\n"),
	}
	_, err := read(fsys, "docs", noRemote, false)
	require.ErrorContains(t, err, "a release takes the settings of the site")
}

// TestHTTPSRepoURL tests that the URL of a Git remote over SSH or HTTPS
// becomes the HTTPS URL of the repository.
func TestHTTPSRepoURL(t *testing.T) {
	for name, tt := range map[string]struct{ remote, want string }{
		"ssh":        {remote: "git@github.com:o/demo.git", want: "https://github.com/o/demo"},
		"https":      {remote: "https://github.com/o/demo.git\n", want: "https://github.com/o/demo"},
		"no suffix":  {remote: "https://github.com/o/demo", want: "https://github.com/o/demo"},
		"ssh scheme": {remote: "ssh://git@github.com/o/demo.git", want: "https://github.com/o/demo"},
		"credentials": {
			remote: "https://x:token@github.com/o/demo.git", want: "https://github.com/o/demo",
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

// TestInlineSVG tests that an SVG file loses what precedes its root element
// and gains the attributes that its root element does not set.
func TestInlineSVG(t *testing.T) {
	for name, tt := range map[string]struct {
		data  string
		attrs []string
		want  string
	}{
		"prolog": {
			data:  "<?xml version=\"1.0\"?>\n<!-- c -->\n<svg viewBox=\"0 0 1 1\"></svg>\n",
			attrs: []string{`aria-hidden="true"`},
			want:  `<svg aria-hidden="true" viewBox="0 0 1 1"></svg>`,
		},
		"attribute set": {
			data:  `<svg aria-hidden="false" class="a"></svg>`,
			attrs: []string{`aria-hidden="true"`, `role="img"`},
			want:  `<svg role="img" aria-hidden="false" class="a"></svg>`,
		},
		"multiline tag": {
			data:  "<svg\n\tclass=\"a\"\n>\n</svg>",
			attrs: []string{`class="b"`},
			want:  "<svg\n\tclass=\"a\"\n>\n</svg>",
		},
		"not SVG": {data: "<png/>", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, inlineSVG([]byte(tt.data), tt.attrs...))
		})
	}
}

// TestSiteChrome tests what every page shows of the settings of the site: the
// name in the title, the logo that leads home, the links of the header and the
// footer with their icons, the link of the header to the page marked current,
// or that to the first page on a page that no link leads to, the license, the
// favicon and the stylesheet of the docs directory.
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
				`<script type="module" src="../docpages.js"></script>`,
				`<a class="logo" href="../../../" aria-label="Demo home">` +
					`<svg class="logo-svg" role="img" aria-label="Demo" ` +
					`xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">`,
				`<a class="header-link icon-link" href="../" aria-current="page">` +
					`<svg aria-hidden="true" viewBox="0 0 16 16"><path d="M2 2"/></svg> Guide</a>`,
				`<a class="header-link" href="../more/examples">Examples</a>`,
				`<a class="icon-link" href="https://chat.example.com">` +
					`<svg aria-hidden="true" viewBox="0 0 16 16"><path d="M1 1"/></svg> Chat</a>`,
				`<a class="icon-link" href="../reference/changelog"> Changelog</a>`,
				`<a class="icon-link" href="../llms.txt">`,
				"<span>MIT License © 2026 Jane Doe</span>",
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

// TestSiteDefaults tests the chrome of a docs directory without settings,
// logo, favicon, stylesheet and license: the name of the repository, the logo
// as text that leads to the first page, a header that links the first page as
// Docs, and no favicon or license.
func TestSiteDefaults(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md":    {Data: []byte("# Intro\n\nA tool.\n")},
		"1-a/1-b.md":   {Data: []byte("# B\n")},
		"1-a/2-c.md":   {Data: []byte("# C\n")},
		"unrelated.go": {Data: []byte("package x\n")},
	}
	s, err := read(fsys, ".", func() (string, error) {
		return "git@github.com:o/tool.git", nil
	}, false)
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

	_, err = read(fsys, ".", noRemote, false)
	require.ErrorContains(t, err, "set repository in the front matter of README.md")
}

// TestSiteHeader tests that an empty header leaves the header without links
// and that a missing icon of a link of the header fails the read.
func TestSiteHeader(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/README.md"] = &fstest.MapFile{Data: []byte(strings.Replace(testRoot,
		"header:\n  - title: Guide\n    url: README.md\n    icon: icons/book.svg\n"+
			"  - title: Examples\n    url: 4-more/1-examples.md\n",
		"header: []\n", 1))}
	// The icon of the link that the header lost, which nothing uses now.
	delete(fsys, "docs/icons/book.svg")
	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	require.NotContains(t, pageHTML(t, s, "guides/events"), "header-link")

	fsys = testRepoFS()
	delete(fsys, "docs/icons/book.svg")
	_, err = read(fsys, "docs", noRemote, false)
	require.ErrorContains(t, err, `icon of the header link "Guide"`)
}

// TestSiteLinks tests that the links of a page lead to the pages of the files
// that they name, a source through its stub or directly, and to GitHub for
// other files, and that a dead link of the front matter fails the read.
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

	fsys := testRepoFS()
	fsys["docs/README.md"] = &fstest.MapFile{Data: []byte(strings.Replace(testRoot,
		"4-more/1-examples.md", "4-more/1-examples.md#missing", 1))}
	_, err := read(fsys, "docs", noRemote, false)
	require.ErrorContains(t, err, `docs/README.md: link "Examples" of the front matter: `+
		`link "4-more/1-examples.md#missing" names no heading`)
}

// TestPageNameWithSpace tests that the Markdown copies, llms-full.txt and
// llms.txt link a page whose name holds a space by its escaped URL, because a
// Markdown link ends at a space.
func TestPageNameWithSpace(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/README.md": {
			Data: []byte("---\nrepository: https://github.com/o/demo\n---\n\n" +
				"# Intro\n\nSee [the first steps](1-guide/1-first%20steps.md).\n"),
		},
		"docs/1-guide/1-first steps.md": {
			Data: []byte("# First steps\n\nRun it.\n"),
		},
	}
	s, err := read(fsys, "docs", noRemote, false)
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

// TestSiteFiles tests that the site holds the files of each edition under
// main/ or <feature version>/ with the assets that the pages load, and index.html
// and llms.txt beside them, and that the pages of a release show its sources
// from its own directory.
func TestSiteFiles(t *testing.T) {
	s, err := read(withReleases(t, testRepoFS(), "v1.2.3"), "docs", noRemote, false)
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
		// The style.css of the docs directory as esbuild minifies it.
		require.Equal(t, []byte("body{color:red}\n"), files[dir+"style.css"])
		// The copy of a page with front matter starts with the line after it.
		require.Equal(t, "# Demo Specification\n\n## Pages\n\n### Actions\n",
			strings.TrimLeft(string(files[dir+"reference/specification.md"]), "\n"))
	}
}

// TestWrite tests that a run removes what an earlier run wrote, as the
// directory of a release that the docs no longer hold, and keeps other files
// of the output directory.
func TestWrite(t *testing.T) {
	out := t.TempDir()
	for _, name := range []string{"v0.9/index.html", "main/old.html", "index.html", "landing.html"} {
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

// TestSearchSuggestions tests that the search offers the pages that their
// front matter marks as suggested, under their group.
func TestSearchSuggestions(t *testing.T) {
	html := pageHTML(t, newTestSite(t), "guides/events")
	require.Regexp(t, `<a href="../" tabindex="-1">`+
		`<span class="result-page">Get started</span>\s*<span class="result-section">Introduction</span>`,
		html)
	require.Contains(t, html, `<a href="../get-started/installation" tabindex="-1">`)
	require.Equal(t, 2, strings.Count(html, `id="search_suggestion_`))
}

// TestStaticAssets tests that every asset that the pages load is embedded,
// the script minified, and that the sources that [Handler] serves are too.
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

// TestIcons tests that every icon that the templates show is a file of icons,
// which the site publishes in morpheus/icons, and that no icon drawn for the
// site has the name of an icon of Morpheus.
func TestIcons(t *testing.T) {
	templates, err := filepath.Glob("*.templ")
	require.NoError(t, err)
	var names []string
	for _, name := range templates {
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		for _, m := range regexp.MustCompile(`@icon\("([^"]+)"\)`).FindAllSubmatch(src, -1) {
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

// TestDocsDirectories tests that the docs of docpages in docs/ and those of
// the examples in example/ build.
func TestDocsDirectories(t *testing.T) {
	for _, dir := range []string{"docs", "example/cli/docs", "example/library/docs"} {
		t.Run(dir, func(t *testing.T) {
			s, err := read(os.DirFS("."), dir, noRemote, false)
			require.NoError(t, err)
			_, err = s.Files()
			require.NoError(t, err)
		})
	}
}
