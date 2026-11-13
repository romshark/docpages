package docpages

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/require"
)

// testPages covers [page.base]: a directory page, a page within it,
// a version-root page, and an external link.
var testPages = []page{
	{
		file:  "guide/README.md",
		path:  "docs/",
		title: "Introduction",
		group: 0,
	},
	{
		file:  "guide/events.md",
		path:  "docs/events",
		title: "Events",
		group: 1,
	},
	{
		file:  "SPECIFICATION.md",
		stub:  "guide/specification.md",
		path:  "specification",
		title: "Specification",
		group: 2,
	},
	{
		url:   "https://pkg.go.dev/github.com/o/demo",
		title: "Go API",
		group: 2,
	},
}

// testGroups are the groups of [testPages].
var testGroups = []group{
	{title: "Get started"},
	{title: "Guides"},
	{title: "Reference"},
}

// testRepo is the repository of the test pages.
const testRepo = "https://github.com/o/demo"

// testRepoFiles are the repository files other than pages that test pages link.
var testRepoFiles = fstest.MapFS{
	"FAQ.md":                        {},
	"Design Doc.pdf":                {},
	"datapages.go":                  {},
	"example/counter/main.go":       {},
	"internal/acceptance/README.md": {},
}

var testSite = &site{
	pages: testPages,
	ids: []map[string]bool{
		{"introduction": true},
		{"events": true, "dispatch": true},
		{"spec": true, "pages": true},
		nil,
	},
	ref:  MainVersion,
	host: githubHost(testRepo),
	fsys: testRepoFiles,
}

// blob is the URL of the files of the main branch on GitHub.
const blob = testRepo + "/blob/main/"

// convertTest converts srcs as the first len(srcs) test pages at Git ref.
func convertTest(t *testing.T, ref string, srcs ...string) []doc {
	t.Helper()
	pages := slices.Clone(testPages[:len(srcs)])
	b := make([][]byte, len(srcs))
	for i, src := range srcs {
		if src != "" {
			b[i] = []byte(src)
		}
	}
	docs, err := convertDocs(pages, b,
		&site{ref: ref, host: githubHost(testRepo), fsys: testRepoFiles})
	require.NoError(t, err)
	return docs
}

// TestResolveLink checks that page-file links open their pages, other
// repository-file links open GitHub, and all other links stay unchanged.
func TestResolveLink(t *testing.T) {
	for name, tt := range map[string]struct {
		from       int
		dest, want string
	}{
		"heading": {
			from: 1, dest: "#dispatch",
			want: "#dispatch",
		},
		"own file": {
			from: 1, dest: "events.md",
			want: "#",
		},
		"page in the same dir": {
			from: 0, dest: "events.md#dispatch",
			want: "../docs/events#dispatch",
		},
		"page in the parent": {
			from: 1, dest: "../SPECIFICATION.md",
			want: "../specification",
		},
		"heading of other page": {
			from: 1, dest: "../SPECIFICATION.md#pages",
			want: "../specification#pages",
		},
		"from the site root": {
			from: 2, dest: "guide/events.md#dispatch",
			want: "./docs/events#dispatch",
		},
		"directory of a README": {
			from: 2, dest: "guide/",
			want: "./docs/",
		},
		"README of the dir": {
			from: 1, dest: "README.md",
			want: "../docs/",
		},
		"stub of a source": {
			from: 1, dest: "specification.md#pages",
			want: "../specification#pages",
		},
		"file": {
			from: 1, dest: "../FAQ.md",
			want: blob + "FAQ.md",
		},
		"nested file": {
			from: 1, dest: "../internal/acceptance/README.md",
			want: blob + "internal/acceptance/README.md",
		},
		"line of a file": {
			from: 2, dest: "datapages.go#L10",
			want: blob + "datapages.go#L10",
		},
		"directory": {
			from: 1, dest: "../example/counter/",
			want: testRepo + "/tree/main/example/counter/",
		},
		"absolute": {
			from: 1, dest: "https://templ.guide",
			want: "https://templ.guide",
		},
		"mailto": {
			from: 1, dest: "mailto:dev@example.com",
			want: "mailto:dev@example.com",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := testSite.resolveLink(tt.from, tt.dest)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestResolveLinkErr checks that missing headings and invalid URLs fail.
func TestResolveLinkErr(t *testing.T) {
	for name, dest := range map[string]string{
		"unknown heading":               "#missing",
		"heading of another page":       "../SPECIFICATION.md#dispatch",
		"unknown heading of other page": "../SPECIFICATION.md#missing",
		"invalid":                       "%zz",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := testSite.resolveLink(1, dest)
			require.Error(t, err)
		})
	}
}

// TestResolveLinkNoFile checks that links to files and directories missing from
// the repository fail with the repository path.
func TestResolveLinkNoFile(t *testing.T) {
	for dest, want := range map[string]string{
		"renamed.md":       `link "renamed.md": no file guide/renamed.md`,
		"../example/none/": `link "../example/none/": no file example/none`,
		"../../outside.md": `link "../../outside.md": no file ../outside.md`,
	} {
		_, err := testSite.resolveLink(1, dest)
		require.EqualError(t, err, want)
	}
}

// TestResolveMarkdownLink checks links to page Markdown copies from both
// another copy and the site root.
func TestResolveMarkdownLink(t *testing.T) {
	for name, tt := range map[string]struct {
		base, dest, want string
	}{
		"other page": {
			base: "../", dest: "../SPECIFICATION.md#pages",
			want: "../specification.md#pages",
		},
		"directory page": {
			base: "../", dest: "README.md",
			want: "../docs/index.md",
		},
		"from the root": {
			base: "", dest: "../SPECIFICATION.md#pages",
			want: "specification.md#pages",
		},
		"own heading": {
			base: "../", dest: "#dispatch",
			want: "#dispatch",
		},
		"file": {
			base: "../", dest: "../FAQ.md",
			want: blob + "FAQ.md",
		},
		// A Markdown link ends at a space.
		"file with a space": {
			base: "../", dest: "../Design%20Doc.pdf",
			want: blob + "Design%20Doc.pdf",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := testSite.resolve(1, tt.dest, tt.base, page.markdownURL)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestConvertDocs checks that each page file produces its title, description,
// headings, and body; HTML links open pages and copy links open Markdown copies.
func TestConvertDocs(t *testing.T) {
	docs := convertTest(t, MainVersion,
		dedentMD(`
			# Introduction

			Demo is a *web* framework
			for ~Go~.

			Read [the events](events.md#dispatch) and [the FAQ](../FAQ.md).

			| a | b |
			| - | - |
			| 1 | 2 |
		`),
		dedent(`
			# Events

			Events update tabs.

			## Dispatch

			See [pages][spec] and [again][spec].

			[spec]: ../SPECIFICATION.md#pages
		`),
		dedent(`
			# Spec

			## Pages

			### Actions

			#### Parameters

			See [the guide](guide/) and [pages](#pages).
		`),
		"",
	)
	require.Len(t, docs, 4)

	intro := docs[0]
	require.Equal(t, "Introduction", intro.title)
	require.Equal(t, "Demo is a web framework for Go.", intro.description)
	require.Contains(t, intro.body, `<a href="../docs/events#dispatch">`)
	require.Contains(t, intro.body, `<a href="`+blob+`FAQ.md">`)
	require.Contains(t, intro.body, "<table>")
	require.Contains(t, string(intro.markdown),
		"[the events](../docs/events.md#dispatch)")
	require.Contains(t, string(intro.markdownFromRoot),
		"[the events](docs/events.md#dispatch)")

	events := docs[1]
	require.Equal(t, []section{
		{ID: "events", Title: "Events", Level: 1},
		{ID: "dispatch", Title: "Dispatch", Level: 2},
	}, events.sections)
	require.Equal(t, 2,
		strings.Count(events.body, `<a href="../specification#pages">`))
	require.Contains(t, string(events.markdown),
		"[spec]: ../specification.md#pages\n")

	spec := docs[2]
	require.Equal(t, []section{
		{ID: "spec", Title: "Spec", Level: 1},
		{ID: "pages", Title: "Pages", Level: 2},
		{ID: "actions", Title: "Actions", Level: 3},
		{ID: "parameters", Title: "Parameters", Level: 4},
	}, spec.sections)
	require.Contains(t, spec.body, `<a href="./docs/">`)
	require.Contains(t, spec.body, `<a href="#pages">`)
	require.Contains(t, spec.body, `<h4 id="parameters">`)

	require.Zero(t, docs[3])
}

// TestConvertDocsDeadFragment checks that an unknown fragment fails conversion.
// GitHub derives use_it from the heading; goldmark derives use-it.
func TestConvertDocsDeadFragment(t *testing.T) {
	_, err := convertDocs(slices.Clone(testPages[:1]), [][]byte{
		[]byte(dedent(`
			# Title

			See [usage](#use_it).

			## Use_it
		`)),
	}, &site{ref: MainVersion, host: githubHost(testRepo)})
	require.ErrorContains(t, err,
		`guide/README.md: link "#use_it" names no heading`)
}

// TestConvertDocsTitle checks that a missing title uses the level 1 heading and
// that missing both is an error.
func TestConvertDocsTitle(t *testing.T) {
	pages := []page{{file: "a.md"}}
	_, err := convertDocs(pages, [][]byte{[]byte("# The `App`\n")}, &site{})
	require.NoError(t, err)
	require.Equal(t, "The App", pages[0].title)

	_, err = convertDocs([]page{{file: "b.md"}}, [][]byte{[]byte("Text.\n")}, &site{})
	require.ErrorContains(t, err, "b.md has no level 1 heading and no title")
}

// TestContents checks that contents list the title, then headings from level 2
// through the page depth, and are absent without those headings.
func TestContents(t *testing.T) {
	sections := []section{
		{ID: "spec", Title: "Spec", Level: 1},
		{ID: "pages", Title: "Pages", Level: 2},
		{ID: "actions", Title: "Actions", Level: 3},
		{ID: "parameters", Title: "Parameters", Level: 4},
	}
	for name, tt := range map[string]struct {
		depth    int
		sections []section
		want     []ContentsLink
	}{
		"depth 3": {
			depth:    3,
			sections: sections,
			want: []ContentsLink{
				{Href: "#spec", Title: "Spec", Level: 1},
				{Href: "#pages", Title: "Pages", Level: 2},
				{Href: "#actions", Title: "Actions", Level: 3},
			},
		},
		"depth 4": {
			depth:    4,
			sections: sections,
			want: []ContentsLink{
				{Href: "#spec", Title: "Spec", Level: 1},
				{Href: "#pages", Title: "Pages", Level: 2},
				{Href: "#actions", Title: "Actions", Level: 3},
				{Href: "#parameters", Title: "Parameters", Level: 4},
			},
		},
		"no title": {
			depth:    3,
			sections: sections[1:],
			want: []ContentsLink{
				{Href: "#pages", Title: "Pages", Level: 2},
				{Href: "#actions", Title: "Actions", Level: 3},
			},
		},
		"title only": {
			depth:    3,
			sections: sections[:1],
			want:     nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := doc{sections: tt.sections, depth: tt.depth}
			require.Equal(t, tt.want, contents(d))
		})
	}
}

// TestTOCDepth checks that contents reach level 4 unless text repeats below
// level 2, which stops them above that level.
func TestTOCDepth(t *testing.T) {
	h := func(level int, title string) section {
		return section{Title: title, Level: level}
	}
	for name, tt := range map[string]struct {
		sections []section
		want     int
	}{
		"no headings":   {want: 4},
		"level 2 alike": {sections: []section{h(2, "A"), h(2, "A"), h(3, "B")}, want: 4},
		"level 3 alike": {
			sections: []section{h(2, "Init"), h(3, "Flags"), h(2, "Gen"), h(3, "Flags")},
			want:     2,
		},
		"level 4 alike": {
			sections: []section{
				h(2, "A"), h(3, "B"), h(4, "Example"), h(3, "C"), h(4, "Example"),
			},
			want: 3,
		},
		"level 3 and 4 alike": {
			sections: []section{h(3, "Same"), h(4, "Same")},
			want:     3,
		},
		"distinct": {
			sections: []section{h(2, "A"), h(3, "B"), h(4, "C"), h(5, "C"), h(5, "C")},
			want:     4,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, tocDepth(tt.sections))
		})
	}
}

// TestNavigation checks page order and groups, current-relative links,
// and the current mark.
func TestNavigation(t *testing.T) {
	require.Equal(t, []NavGroup{
		{Title: "Get started", Links: []NavLink{
			{Title: "Introduction", Href: "../docs/"},
		}},
		{Title: "Guides", Links: []NavLink{
			{Title: "Events", Href: "../docs/events", Current: true},
		}},
		{Title: "Reference", Links: []NavLink{
			{Title: "Specification", Href: "../specification"},
			{
				Title:    "Go API",
				Href:     "https://pkg.go.dev/github.com/o/demo",
				External: true,
			},
		}},
	}, navigation(testPages, testGroups, 1))
}

// TestNeighbors checks that previous and next links skip external entries.
func TestNeighbors(t *testing.T) {
	for name, tt := range map[string]struct {
		page       int
		prev, next NavLink
	}{
		"first": {
			page: 0,
			next: NavLink{Title: "Events", Href: "../docs/events"},
		},
		"middle": {
			page: 1,
			prev: NavLink{Title: "Introduction", Href: "../docs/"},
			next: NavLink{Title: "Specification", Href: "../specification"},
		},
		"last before a link": {
			page: 2,
			prev: NavLink{Title: "Events", Href: "./docs/events"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			prev, next := neighbors(testPages, tt.page)
			require.Equal(t, tt.prev, prev)
			require.Equal(t, tt.next, next)
		})
	}
}

// convert converts md as the file of the first test page.
func convert(t *testing.T, md string) doc {
	t.Helper()
	return convertTest(t, MainVersion, md)[0]
}

// TestCodeBlocks checks Chroma classes for known languages, plain text without
// a language, and per-block titles, IDs, and copy buttons. Titled blocks put
// the button in the title. [TestLint] covers unknown-language failures.
func TestCodeBlocks(t *testing.T) {
	const goFunc = `<pre><code class="language-go"><span class="kd">func</span>`
	for name, tt := range map[string]struct {
		md   string
		want []string
	}{
		"go": {
			md: dedentMD(`
				~~~go
				func f() {}
				~~~
			`),
			want: []string{goFunc},
		},
		"no language": {
			md: dedentMD(`
				~~~
				func f() {}
				~~~
			`),
			want: []string{dedent(`
				<pre><code>func f() {}
				</code></pre>`)},
		},
		"title": {
			md: dedentMD(`
				~~~go title="app/app.go"
				func f() {}
				~~~
			`),
			want: []string{
				`<div class="code-title">app/app.go<neo-clipcopy for="code_1">`,
				`</neo-clipcopy></div><div class="code-body" id="code_1">` + goFunc,
				"</code></pre></div></div>\n",
			},
		},
		"copy buttons": {
			md: dedentMD(`
				~~~
				a
				~~~

				~~~
				b
				~~~
			`),
			want: []string{
				`<div class="code-body" id="code_1">`,
				`<neo-clipcopy for="code_1">`,
				`<div class="code-body" id="code_2">`,
				`<neo-clipcopy for="code_2">`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := convert(t, "# T\n\n"+tt.md).body
			for _, want := range tt.want {
				require.Contains(t, body, want)
			}
		})
	}
}

// TestCallouts checks that GitHub-alert blockquotes become marker-free callouts
// and other blockquotes stay unchanged.
func TestCallouts(t *testing.T) {
	for name, tt := range map[string]struct {
		md, want, unwanted string
	}{
		"note": {
			md: dedent(`
				> [!NOTE]
				> Read *this*.
			`),
			want: dedent(`
				<div class="callout callout-note" role="note"><p class="callout-title">Note</p>
				<p>Read <em>this</em>.</p>
				</div>`),
			unwanted: "[!NOTE]",
		},
		"marker only": {
			md: "> [!WARNING]\n",
			want: dedent(`
				<div class="callout callout-warning" role="note"><p class="callout-title">Warning</p>
				</div>`),
			unwanted: "<p></p>",
		},
		"unknown kind": {
			md: dedent(`
				> [!FOO]
				> text
			`),
			want:     "<blockquote>",
			unwanted: "callout",
		},
		"blockquote": {
			md: "> text\n",
			want: dedent(`
				<blockquote>
				<p>text</p>
				</blockquote>`),
			unwanted: "callout",
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := convert(t, "# T\n\n"+tt.md).body
			require.Contains(t, body, tt.want)
			require.NotContains(t, body, tt.unwanted)
		})
	}
}

// TestDetails checks that <details> holds a summary with an icon and only its
// text formatting, wraps Markdown or raw-HTML content in <neo-elastic>,
// keeps open, nests, and remains unchanged in the Markdown copy.
// [TestLint] covers omitted content such as scripts.
func TestDetails(t *testing.T) {
	summary := func(open, title string) string {
		elastic := ` open="false"`
		if open != "" {
			elastic = ""
		}
		return `<details class="details"` + open + `><summary>` + detailsIcon() +
			`<span class="details-title">` + title +
			`</span></summary><neo-elastic class="details-body"` + elastic + `>`
	}
	for name, tt := range map[string]struct {
		md             string
		want, unwanted []string
	}{
		"markdown": {
			md: dedent(`
				<details>
				<summary>Why a <code>power</code> of 2?</summary>

				A mask *wraps* the index.

				</details>
			`),
			want: []string{fmt.Sprintf(dedent(`
				%s
				<p>A mask <em>wraps</em> the index.</p>
				</neo-elastic></details>`), summary("", "Why a <code>power</code> of 2?"))},
			unwanted: []string{"raw HTML omitted"},
		},
		"raw text, open": {
			md: dedent(`
				<details open><summary><b>Title</b> <i>here</i></summary>
				Raw text.
				</details>
			`),
			want: []string{fmt.Sprintf(dedent(`
				%s
				Raw text.
				</neo-elastic></details>`), summary(" open", "<b>Title</b> <i>here</i>"))},
		},
		"nested": {
			md: dedent(`
				<details>
				<summary>Outer</summary>

				<details>
				<summary>Inner</summary>

				Text.

				</details>

				</details>
			`),
			want: []string{fmt.Sprintf(dedent(`
				%s
				%s
				<p>Text.</p>
				</neo-elastic></details>
				</neo-elastic></details>`), summary("", "Outer"), summary("", "Inner"))},
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := convert(t, "# T\n\n"+tt.md)
			for _, want := range tt.want {
				require.Contains(t, d.body, want)
			}
			for _, unwanted := range tt.unwanted {
				require.NotContains(t, d.body, unwanted)
			}
			require.Contains(t, string(d.markdown), tt.md)
		})
	}
}

// TestDetailsErr checks that missing summaries or closing tags, and orphaned
// <summary> or </details> tags, fail conversion.
func TestDetailsErr(t *testing.T) {
	for md, want := range map[string]string{
		dedent(`
			<details>

			Text.

			</details>
		`): "<details> has no <summary>",
		dedent(`
			<details>
			<summary>A</summary>

			Text.
		`): "<details> without </details>",
		dedent(`
			<details>
			<summary>A

			</details>
		`): "<summary> without </summary>",
		dedent(`
			Text.

			</details>
		`): "</details> closes no <details>",
		"<summary>A</summary>\n": "<summary> is not the first in a <details>",
		dedent(`
			<details>
			<summary>A</summary><summary>B</summary>`): "<summary> is not the first in a <details>",
	} {
		_, err := convertDocs(
			slices.Clone(testPages[:1]),
			[][]byte{[]byte("# T\n\n" + md)},
			&site{ref: MainVersion, host: githubHost(testRepo)},
		)
		require.ErrorContains(t, err, want, md)
		require.ErrorContains(t, err, "guide/README.md:", md)
	}
}

// TestHeadingLinks checks that every heading below level 1 links to itself.
func TestHeadingLinks(t *testing.T) {
	body := convert(t, dedentMD(`
		# Title

		## The ~App~ type
	`)).body
	require.Contains(t, body, `<h1 id="title">Title</h1>`)
	require.Contains(t, body, `<h2 id="the-app-type">The <code>App</code> type`+
		`<a class="heading-link" href="#the-app-type"`+
		` aria-label="Link to this section"></a></h2>`)
}

// TestPlainTextAutoLinks checks that page titles, descriptions, and search text
// preserve bare GFM-linked URLs and angle-bracketed URLs and emails.
func TestPlainTextAutoLinks(t *testing.T) {
	d := convert(t, dedent(`
		# Install from <https://go.dev>

		See https://example.com and <https://go.dev>, or write <dev@example.com>.
	`))
	const text = "See https://example.com and https://go.dev, or write dev@example.com."
	require.Equal(t, "Install from https://go.dev", d.title)
	require.Equal(t, text, d.description)
	require.Equal(t, "Install from https://go.dev "+text, d.search[0].Text)
}

// TestPlainTextBlocks checks that search text separates two blocks in one node,
// such as two list items or table cells.
func TestPlainTextBlocks(t *testing.T) {
	d := convert(t, dedent(`
		# Writing

		- A list
		  - with a nested item
		- [x] A done task

		| Flag | Default |
		| --- | --- |
		| -o | site |
	`))
	require.Equal(t,
		"Writing A list with a nested item A done task Flag Default -o site",
		d.search[0].Text)
}

// TestSearchEntries checks that a page yields one code-free entry above its
// first section and one per heading through its depth.
func TestSearchEntries(t *testing.T) {
	docs, err := convertDocs(
		slices.Clone(testPages[1:2]),
		[][]byte{[]byte(dedentMD(`
			# Events

			Events update
			other tabs.

			~~~go
			func hidden() {}
			~~~

			## Dispatch

			Use a ~Dispatcher~.

			### Details

			- a list

			### Details

			- again
		`))},
		&site{ref: MainVersion, host: githubHost(testRepo)})
	require.NoError(t, err)
	require.Equal(t, []searchEntry{
		{
			Page: "Events",
			URL:  "docs/events",
			Text: "Events Events update other tabs.",
		},
		{
			Page:    "Events",
			Section: "Dispatch",
			URL:     "docs/events#dispatch",
			Text:    "Use a Dispatcher. Details a list Details again",
		},
	}, docs[0].search)
}

// TestTruncate checks that long text cuts at a space, or before the character
// containing byte n when none exists, while short text stays unchanged.
func TestTruncate(t *testing.T) {
	for name, tt := range map[string]struct {
		s    string
		n    int
		want string
	}{
		"short":         {s: "a b", n: 5, want: "a b"},
		"at space":      {s: "aaa bbb ccc", n: 9, want: "aaa bbb"},
		"no space":      {s: "aaaaaa", n: 3, want: "aaa"},
		"in a rune":     {s: "a日本語", n: 5, want: "a日"},
		"at a rune end": {s: "a日本語", n: 4, want: "a日"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, truncate(tt.s, tt.n))
		})
	}
}

// TestLLMSTxt checks that llms.txt names the project, includes the first-page
// description, and groups all entries with page links to Markdown copies.
func TestLLMSTxt(t *testing.T) {
	docs := []doc{
		{description: "Demo is a web framework for Go."},
		{description: "Events update tabs."},
		{description: "The rules."},
		{},
	}
	require.Equal(t, dedent(`
		# Demo

		> Demo is a web framework for Go.

		Each link leads to the Markdown copy of a page of the documentation. llms-full.txt holds all of them in 1 file.

		## Get started

		- [Introduction](docs/index.md): Demo is a web framework for Go.

		## Guides

		- [Events](docs/events.md): Events update tabs.

		## Reference

		- [Specification](specification.md): The rules.
		- [Go API](https://pkg.go.dev/github.com/o/demo)
	`),
		string(llmsTxt("Demo", testPages, testGroups, docs)))
}

// TestSuggestions checks that search offers suggested pages with page-relative
// links, uses an external link's URL, and hides the heading when none exist.
func TestSuggestions(t *testing.T) {
	pages := []page{
		{path: "", title: "Introduction", group: 0, suggested: true},
		{path: "guide/events", title: "Events", group: 1},
		{path: "faq", title: "FAQ", group: 2, suggested: true},
		{
			path:      "api",
			url:       "https://pkg.go.dev/x",
			title:     "API",
			group:     2,
			suggested: true,
		},
	}
	groups := []group{{title: "Get started"}, {title: "Guides"}, {title: "More"}}
	suggested := suggestions(pages, groups, 1)
	require.Equal(t, []Suggestion{
		{Group: "Get started", Title: "Introduction", Href: "../"},
		{Group: "More", Title: "FAQ", Href: "../faq"},
		{Group: "More", Title: "API", Href: "https://pkg.go.dev/x"},
	}, suggested)

	html := render(t, Search(Page{Suggested: suggested}))
	for _, want := range []string{
		`aria-expanded="true"`,
		`<p class="search-heading">Suggested</p>`,
		`<li id="search_suggestion_1" role="option" aria-selected="false">`,
		`<a href="../faq" tabindex="-1">`,
		`<span class="result-section">FAQ</span>`,
		`<a href="https://pkg.go.dev/x" tabindex="-1">`,
	} {
		require.Contains(t, html, want)
	}

	html = render(t, Search(Page{}))
	require.Contains(t, html, `aria-expanded="false"`)
	require.Contains(t, html, `<p class="search-heading" hidden>Suggested</p>`)
}

// render returns the HTML of c.
func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, c.Render(context.Background(), &b))
	return b.String()
}

// TestHeadingKeepsTemplateIDs checks that a heading cannot take a template ID.
func TestHeadingKeepsTemplateIDs(t *testing.T) {
	body := convert(t, dedent(`
		# T

		## Main content
	`)).body
	require.Contains(t, body, `<h2 id="main-content-1">`)
}

// TestSkipLink checks that every page starts with a link to its sole <main>.
func TestSkipLink(t *testing.T) {
	html := pageHTML(t, newTestSite(t), "guides/events")
	body := html[strings.Index(html, "<body"):]
	body = body[strings.Index(body, ">")+1:]
	require.True(t, strings.HasPrefix(body, `<a class="skip-link"`+
		` href="#main-content">Skip to content</a>`), body[:200])
	require.Equal(t, 1, strings.Count(html, "<main"))
	require.Contains(t, html, `id="main-content" tabindex="-1">`)
}

// TestTemplateIDs checks that templateIDs covers every docs-template ID,
// keeping them unavailable to code blocks and headings.
func TestTemplateIDs(t *testing.T) {
	s := newTestSite(t)
	html := pageHTML(t, s, "guides/events")
	written := make(map[string]bool)
	ids := regexp.MustCompile(` id="([^"]+)"`)
	for _, m := range ids.FindAllStringSubmatch(html, -1) {
		written[m[1]] = true
	}
	for _, sec := range s.main().
		docs[s.main().links.pageOf("docs/2-guides/01-events.md")].
		sections {
		delete(written, sec.ID)
	}
	// goldmark omits "_" from heading IDs, keeping numbered block and suggestion
	// IDs distinct.
	for id := range written {
		if strings.Contains(id, "_") {
			delete(written, id)
		}
	}
	want := make(map[string]bool)
	for _, id := range templateIDs {
		want[id] = true
	}
	require.Equal(t, want, written)
}

// TestPageLinks checks links to a page's site Markdown and GitHub file, or its
// source, and icon-link styling for them and the header's GitHub link.
func TestPageLinks(t *testing.T) {
	const blob = "https://github.com/o/demo/blob/main/"
	s := newTestSite(t)
	for name, tt := range map[string]struct {
		path string
		want []string
	}{
		"page in a group": {
			path: "guides/events",
			want: []string{
				`<a class="icon-link" href="../guides/events.md">` +
					`<neo-icon name="markdown"`,
				`<a class="icon-link" href="` + blob + `docs/2-guides/01-events.md">` +
					`<neo-icon name="github"`,
				`<a class="header-repository icon-link" href="https://github.com/o/demo">` +
					`<neo-icon name="github"`,
			},
		},
		"first page": {
			path: "",
			want: []string{
				`<a class="icon-link" href="./index.md">`,
				`<a class="icon-link" href="` + blob + `docs/README.md">`,
			},
		},
		"page with a source": {
			path: "reference/specification",
			want: []string{
				`<a class="icon-link" href="../reference/specification.md">`,
				`<a class="icon-link" href="` + blob + `SPECIFICATION.md">`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			html := pageHTML(t, s, tt.path)
			for _, want := range tt.want {
				require.Contains(t, html, want)
			}
		})
	}
}

// TestMermaid checks that a Mermaid block becomes the escaped,
// button-free element that docs.ts draws.
func TestMermaid(t *testing.T) {
	body := convert(t, dedentMD(`
		# T

		~~~mermaid
		flowchart LR
		    a --> b
		~~~
	`)).body
	require.Contains(t, body, dedent(`
		<pre class="mermaid">flowchart LR
		    a --&gt; b
		</pre>`))
	require.NotContains(t, body, "code-block")
}
