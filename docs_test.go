package docpages

import (
	"bytes"
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/require"
)

// testPages are pages at paths that cover [page.base]: 1 for a directory,
// 1 in it, 1 in the root of the version, and a link that leaves the site.
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
}

// blob is the URL of the files of the main branch on GitHub.
const blob = testRepo + "/blob/main/"

// convertTest converts srcs as the files of the first len(srcs)
// test pages at the Git ref.
func convertTest(t *testing.T, ref string, srcs ...string) []doc {
	t.Helper()
	pages := slices.Clone(testPages[:len(srcs)])
	b := make([][]byte, len(srcs))
	for i, src := range srcs {
		if src != "" {
			b[i] = []byte(src)
		}
	}
	docs, err := convertDocs(pages, b, &site{ref: ref, host: githubHost(testRepo)})
	require.NoError(t, err)
	return docs
}

// TestResolveLink tests that a link relative to the file of a page leads to the
// page of the file it names, that a link to another file of the repository
// resolves to the file on GitHub and that every other link stays as written.
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

// TestResolveLinkErr tests that a fragment naming no heading of the linked page
// and an invalid URL are errors.
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

// TestResolveMarkdownLink tests that a link of a Markdown copy leads to the Markdown
// copy of the page it names, as seen from the copy and as seen from the site root.
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

// TestConvertDocs tests that each file of a page yields its title, its
// description, its headings and its body, and that its links lead to the other
// pages in the HTML and to their Markdown copies in the Markdown copy.
func TestConvertDocs(t *testing.T) {
	docs := convertTest(t, MainVersion,
		"# Introduction\n\n"+
			"Demo is a *web* framework\nfor `Go`.\n\n"+
			"Read [the events](events.md#dispatch) and [the FAQ](../FAQ.md).\n\n"+
			"| a | b |\n| - | - |\n| 1 | 2 |\n",
		"# Events\n\nEvents update tabs.\n\n## Dispatch\n\n"+
			"See [pages][spec] and [again][spec].\n\n"+
			"[spec]: ../SPECIFICATION.md#pages\n",
		"# Spec\n\n## Pages\n\n### Actions\n\n#### Parameters\n\n"+
			"See [the guide](guide/) and [pages](#pages).\n",
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

// TestConvertDocsDeadFragment tests that a link to a fragment naming no heading
// fails the conversion. GitHub derives the ID use_it from the heading,
// goldmark derives use-it.
func TestConvertDocsDeadFragment(t *testing.T) {
	_, err := convertDocs(slices.Clone(testPages[:1]), [][]byte{
		[]byte("# Title\n\nSee [usage](#use_it).\n\n## Use_it\n"),
	}, &site{ref: MainVersion, host: githubHost(testRepo)})
	require.ErrorContains(t, err,
		`guide/README.md: link "#use_it" names no heading`)
}

// TestConvertDocsTitle tests that a page without a title takes the text of its
// level 1 heading, and that a page with neither is an error.
func TestConvertDocsTitle(t *testing.T) {
	pages := []page{{file: "a.md"}}
	_, err := convertDocs(pages, [][]byte{[]byte("# The `App`\n")}, &site{})
	require.NoError(t, err)
	require.Equal(t, "The App", pages[0].title)

	_, err = convertDocs([]page{{file: "b.md"}}, [][]byte{[]byte("Text.\n")}, &site{})
	require.ErrorContains(t, err, "b.md has no level 1 heading and no title")
}

// TestContents tests that the contents of a page list its title,
// then its headings from level 2 down to the depth of the page,
// and that a page without such headings has no contents.
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
		want     []tocEntry
	}{
		"depth 3": {
			depth:    3,
			sections: sections,
			want: []tocEntry{
				{Href: "#spec", Title: "Spec", Level: 1},
				{Href: "#pages", Title: "Pages", Level: 2},
				{Href: "#actions", Title: "Actions", Level: 3},
			},
		},
		"depth 4": {
			depth:    4,
			sections: sections,
			want: []tocEntry{
				{Href: "#spec", Title: "Spec", Level: 1},
				{Href: "#pages", Title: "Pages", Level: 2},
				{Href: "#actions", Title: "Actions", Level: 3},
				{Href: "#parameters", Title: "Parameters", Level: 4},
			},
		},
		"no title": {
			depth:    3,
			sections: sections[1:],
			want: []tocEntry{
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

// TestTOCDepth tests that the contents list headings down to level 4 unless
// headings below level 2 repeat their text at a level, which stops the
// contents above that level.
func TestTOCDepth(t *testing.T) {
	h := func(level int, title string) section { return section{Title: title, Level: level} }
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

// TestNavigation tests that the navigation groups the pages in their order,
// links them relative to the current page and marks it.
func TestNavigation(t *testing.T) {
	require.Equal(t, []navGroup{
		{Title: "Get started", Entries: []navEntry{
			{Title: "Introduction", Href: "../docs/"},
		}},
		{Title: "Guides", Entries: []navEntry{
			{Title: "Events", Href: "../docs/events", Current: true},
		}},
		{Title: "Reference", Entries: []navEntry{
			{Title: "Specification", Href: "../specification"},
			{
				Title:    "Go API",
				Href:     "https://pkg.go.dev/github.com/o/demo",
				External: true,
			},
		}},
	}, navigation(testPages, testGroups, 1))
}

// TestNeighbors tests that the previous and next links of a page lead to the
// pages next to it and skip links that leave the site.
func TestNeighbors(t *testing.T) {
	for name, tt := range map[string]struct {
		page       int
		prev, next navEntry
	}{
		"first": {
			page: 0,
			next: navEntry{Title: "Events", Href: "../docs/events"},
		},
		"middle": {
			page: 1,
			prev: navEntry{Title: "Introduction", Href: "../docs/"},
			next: navEntry{Title: "Specification", Href: "../specification"},
		},
		"last before a link": {
			page: 2,
			prev: navEntry{Title: "Events", Href: "./docs/events"},
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

// TestCodeBlocks tests that a code block in a language that Chroma has a lexer
// for gets token classes, that a code block without a language stays plain text,
// and that every block gets its title and a copy button for its own ID,
// which a block with a title shows in the title. [TestLint] tests that a
// language that Chroma doesn't know fails the build.
func TestCodeBlocks(t *testing.T) {
	const goFunc = `<pre><code class="language-go"><span class="kd">func</span>`
	for name, tt := range map[string]struct {
		md   string
		want []string
	}{
		"go": {
			md:   "```go\nfunc f() {}\n```\n",
			want: []string{goFunc},
		},
		"no language": {
			md:   "```\nfunc f() {}\n```\n",
			want: []string{"<pre><code>func f() {}\n</code></pre>"},
		},
		"title": {
			md: "```go title=\"app/app.go\"\nfunc f() {}\n```\n",
			want: []string{
				`<div class="code-title">app/app.go<neo-clipcopy for="code_1">`,
				`</neo-clipcopy></div><div class="code-body" id="code_1">` + goFunc,
				"</code></pre></div></div>\n",
			},
		},
		"copy buttons": {
			md: "```\na\n```\n\n```\nb\n```\n",
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

// TestCallouts tests that a blockquote that starts with a GitHub alert marker
// becomes a callout without the marker and that other blockquotes stay.
func TestCallouts(t *testing.T) {
	for name, tt := range map[string]struct {
		md, want, unwanted string
	}{
		"note": {
			md: "> [!NOTE]\n> Read *this*.\n",
			want: `<div class="callout callout-note" role="note">` +
				`<p class="callout-title">Note</p>` +
				"\n<p>Read <em>this</em>.</p>\n</div>",
			unwanted: "[!NOTE]",
		},
		"marker only": {
			md: "> [!WARNING]\n",
			want: `<div class="callout callout-warning" role="note">` +
				`<p class="callout-title">Warning</p>` + "\n</div>",
			unwanted: "<p></p>",
		},
		"unknown kind": {
			md:       "> [!FOO]\n> text\n",
			want:     "<blockquote>",
			unwanted: "callout",
		},
		"blockquote": {
			md:       "> text\n",
			want:     "<blockquote>\n<p>text</p>\n</blockquote>",
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

// TestDetails tests that a <details> holds its summary, with the icon and the
// formatting of its text alone, and its text in a <neo-elastic>, whether the
// text is Markdown or raw HTML, that it keeps open, that it nests, that the
// Markdown copy keeps it as written. [TestLint] tests that what a <details>
// leaves out, as a script, fails the build.
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
			md: "<details>\n<summary>Why a <code>power</code> of 2?</summary>\n\n" +
				"A mask *wraps* the index.\n\n</details>\n",
			want: []string{summary("", "Why a <code>power</code> of 2?") +
				"\n<p>A mask <em>wraps</em> the index.</p>\n</neo-elastic></details>"},
			unwanted: []string{"raw HTML omitted"},
		},
		"raw text, open": {
			md: "<details open><summary><b>Title</b> <i>here</i></summary>\n" +
				"Raw text.\n</details>\n",
			want: []string{summary(" open", "<b>Title</b> <i>here</i>") +
				"\nRaw text.\n</neo-elastic></details>"},
		},
		"nested": {
			md: "<details>\n<summary>Outer</summary>\n\n<details>\n<summary>Inner</summary>\n\n" +
				"Text.\n\n</details>\n\n</details>\n",
			want: []string{
				summary("", "Outer") + "\n" + summary("", "Inner") + "\n<p>Text.</p>\n" +
					"</neo-elastic></details>\n</neo-elastic></details>",
			},
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

// TestDetailsErr tests that a <details> that lacks its <summary> or its end,
// and a <summary> or </details> outside a <details>, fail the conversion.
func TestDetailsErr(t *testing.T) {
	for md, want := range map[string]string{
		"<details>\n\nText.\n\n</details>\n":                  "<details> has no <summary>",
		"<details>\n<summary>A</summary>\n\nText.\n":          "<details> without </details>",
		"<details>\n<summary>A\n\n</details>\n":               "<summary> without </summary>",
		"Text.\n\n</details>\n":                               "</details> closes no <details>",
		"<summary>A</summary>\n":                              "<summary> is not the first in a <details>",
		"<details>\n<summary>A</summary><summary>B</summary>": "<summary> is not the first in a <details>",
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

// TestHeadingLinks tests that every heading below level 1 ends with a link to itself.
func TestHeadingLinks(t *testing.T) {
	body := convert(t, "# Title\n\n## The `App` type\n").body
	require.Contains(t, body, `<h1 id="title">Title</h1>`)
	require.Contains(t, body, `<h2 id="the-app-type">The <code>App</code> type`+
		`<a class="heading-link" href="#the-app-type"`+
		` aria-label="Link to this section"></a></h2>`)
}

// TestPlainTextAutoLinks tests that the title, the description and the search
// text of a page keep the text of its autolinks: a bare URL, which GFM links,
// a URL and an email address in <>.
func TestPlainTextAutoLinks(t *testing.T) {
	d := convert(t, "# Install from <https://go.dev>\n\n"+
		"See https://example.com and <https://go.dev>, or write <dev@example.com>.\n")
	const text = "See https://example.com and https://go.dev, or write dev@example.com."
	require.Equal(t, "Install from https://go.dev", d.title)
	require.Equal(t, text, d.description)
	require.Equal(t, "Install from https://go.dev "+text, d.search[0].Text)
}

// TestSearchEntries tests that a page yields 1 search entry for the text above
// its first section and 1 for each heading down to its depth, without code.
func TestSearchEntries(t *testing.T) {
	docs, err := convertDocs(slices.Clone(testPages[1:2]), [][]byte{[]byte("# Events\n\n" +
		"Events update\nother tabs.\n\n```go\nfunc hidden() {}\n```\n\n" +
		"## Dispatch\n\nUse a `Dispatcher`.\n\n### Details\n\n- a list\n\n" +
		"### Details\n\n- again\n")},
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

// TestTruncate tests that a long text is cut at a space, and without one before
// the character that holds byte n, and that a short one stays.
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

// TestLLMSTxt tests that llms.txt names the project with the description of
// the first page and lists every entry under its group, the pages by their
// Markdown copy.
func TestLLMSTxt(t *testing.T) {
	docs := []doc{
		{description: "Demo is a web framework for Go."},
		{description: "Events update tabs."},
		{description: "The rules."},
		{},
	}
	require.Equal(t, "# Demo\n\n"+
		"> Demo is a web framework for Go.\n\n"+
		"Each link leads to the Markdown copy of a page of the documentation. "+
		"llms-full.txt holds all of them in 1 file.\n"+
		"\n## Get started\n\n"+
		"- [Introduction](docs/index.md): Demo is a web framework for Go.\n"+
		"\n## Guides\n\n"+
		"- [Events](docs/events.md): Events update tabs.\n"+
		"\n## Reference\n\n"+
		"- [Specification](specification.md): The rules.\n"+
		"- [Go API](https://pkg.go.dev/github.com/o/demo)\n",
		string(llmsTxt("Demo", testPages, testGroups, docs)))
}

// TestSuggestions tests that the search of a page offers the suggested pages,
// linked relative to the page, a link that leaves the site by its URL,
// and hides its heading without them.
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
	require.Equal(t, []suggestion{
		{Group: "Get started", Title: "Introduction", Href: "../"},
		{Group: "More", Title: "FAQ", Href: "../faq"},
		{Group: "More", Title: "API", Href: "https://pkg.go.dev/x"},
	}, suggested)

	html := render(t, searchDialog(suggested))
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

	html = render(t, searchDialog(nil))
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

// TestHeadingKeepsTemplateIDs tests that a heading that would take the ID of
// an element of the template gets another one.
func TestHeadingKeepsTemplateIDs(t *testing.T) {
	body := convert(t, "# T\n\n## Main content\n").body
	require.Contains(t, body, `<h2 id="main-content-1">`)
}

// TestSkipLink tests that the first element of every page is the link to its
// <main> element, which every page has exactly 1 of.
func TestSkipLink(t *testing.T) {
	html := pageHTML(t, newTestSite(t), "guides/events")
	body := html[strings.Index(html, "<body"):]
	body = body[strings.Index(body, ">")+1:]
	require.True(t, strings.HasPrefix(body, `<a class="skip-link"`+
		` href="#main-content">Skip to content</a>`), body[:200])
	require.Equal(t, 1, strings.Count(html, "<main"))
	require.Contains(t, html, `id="main-content" tabindex="-1">`)
}

// TestTemplateIDs tests that templateIDs lists every element ID that the template of
// a docs page writes, which the code blocks and headings of the page do not take.
func TestTemplateIDs(t *testing.T) {
	s := newTestSite(t)
	html := pageHTML(t, s, "guides/events")
	written := make(map[string]bool)
	ids := regexp.MustCompile(` id="([^"]+)"`)
	for _, m := range ids.FindAllStringSubmatch(html, -1) {
		written[m[1]] = true
	}
	for _, sec := range s.main().docs[s.main().links.pageOf("docs/2-guides/01-events.md")].sections {
		delete(written, sec.ID)
	}
	// goldmark writes no "_" into the ID of a heading, which keeps the
	// numbered IDs of the code blocks and the suggestions apart from them.
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

// TestPageLinks tests that a docs page links its Markdown copy on the site and
// its file on GitHub, the source of a page with one, and that these links and
// the GitHub link of the header show their icon as icon links.
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

// TestMermaid tests that a mermaid code block becomes the element that docs.js
// draws the diagram of, with the source escaped and without a copy button.
func TestMermaid(t *testing.T) {
	body := convert(t, "# T\n\n```mermaid\nflowchart LR\n    a --> b\n```\n").body
	require.Contains(t, body,
		"<pre class=\"mermaid\">flowchart LR\n    a --&gt; b\n</pre>")
	require.NotContains(t, body, "code-block")
}
