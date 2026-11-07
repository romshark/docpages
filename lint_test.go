package docpages

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// lintPage is the file of the test repository whose Markdown [TestLint] sets.
const lintPage = "docs/2-guides/10-testing.md"

// TestLint checks that rendering differences from GitHub or omitted content
// fail with file and front-matter-aware line: unsupported Chroma languages,
// omitted raw HTML, table-like "|" lines, and list items CommonMark reads as text.
// It also checks accepted content.
func TestLint(t *testing.T) {
	for name, tt := range map[string]struct {
		md   string
		want []string // The errors, each without "docs/2-guides/10-testing.md:".
	}{
		"fine": {md: "# Testing\n\n" +
			"```go\nfunc f() {}\n```\n\n```\nplain\n```\n\n```mermaid\nflowchart LR\n```\n\n" +
			"<!-- A comment. -->\n\n" +
			"| A | B |\n| --- | --- |\n| 1 | 2 |\n\n" +
			"- One\n- Two\n\n1. One\n2. Two\n\n" +
			"*Emphasis* at the start\n-- a dash, and 2 in\n2.5 seconds.\n\n" +
			"2.x is the current version, and the project started in\n2019. It grew.\n\n" +
			"<details open>\n<summary>More <code>here</code></summary>\n\nText.\n\n</details>\n\n" +
			"<video controls muted><track kind=\"captions\" src=\"https://example.com/c.vtt\">" +
			"</video>\n"},
		"code language": {
			md:   "---\ntitle: Testing\n---\n\n# Testing\n\n```nolang\nx\n```\n",
			want: []string{`7: code block in "nolang", a language that Chroma doesn't highlight`},
		},
		"raw HTML": {
			md: "# Testing\n\n<div align=\"center\">\n\nText.\n\n</div>\n\n" +
				"Press <kbd>K</kbd>.\n\n" +
				"<img src=\"https://example.com/a.png\" class=\"wide\">\n\n" +
				"<img src=\"javascript:alert(1)\">\n\n" +
				"<details>\n<summary class=\"s\">More</summary>\n<script>alert(1)</script>\n</details>\n",
			want: []string{
				"3: raw HTML <div>, which a page leaves out",
				"9: raw HTML <kbd>, which a page leaves out",
				"11: attribute class of <img>, which a page leaves out",
				`13: src "javascript:alert(1)" of <img>, a URL that a page leaves out`,
				"15: attribute class of <summary>, which a page leaves out",
				"15: raw HTML <script>, which a page leaves out",
			},
		},
		"table": {
			md:   "# Testing\n\n| A | B |\n| 1 | 2 |\n",
			want: []string{`3: lines that start with "|" but make no table`},
		},
		"list": {
			md: "# Testing\n\n-item\n\nSteps:\n2. Run it\n\n1.First\n",
			want: []string{
				`3: "-item" starts as a list item without the space after "-"`,
				`6: "2. Run it" starts a list at 2 right below a paragraph`,
				`8: "1.First" starts as a list item without the space after "1."`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			fsys := testRepoFS()
			fsys[lintPage] = &fstest.MapFile{Data: []byte(tt.md)}
			_, err := read(fsys, "docs", noRemote, false)
			if len(tt.want) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			lines := strings.Split(err.Error(), "\n")
			require.Len(t, lines, len(tt.want), err.Error())
			for i, want := range tt.want {
				require.True(t, strings.HasPrefix(lines[i], lintPage+":"+want),
					"%q does not start with %q", lines[i], lintPage+":"+want)
			}
		})
	}
}

// TestUnusedFiles checks that unshown docs files unnamed by links or settings fail,
// such as obsolete images or pages in unpositioned directories,
// while linked directory contents and group icons count as used.
func TestUnusedFiles(t *testing.T) {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

	fsys := testRepoFS()
	fsys["docs/2-guides/img/old.png"] = file("old")
	fsys["docs/guides/draft.md"] = file("# Draft\n")
	_, err := read(fsys, "docs", noRemote, false)
	require.EqualError(t, err, ""+
		"docs/2-guides/img/old.png: no page shows this file, and no link or setting names it\n"+
		"docs/guides/draft.md: no page shows this file, and no link or setting names it")

	fsys = testRepoFS()
	fsys[lintPage] = file("# Testing\n\nSee [the files](files/).\n")
	fsys["docs/2-guides/files/a.txt"] = file("a")
	fsys["docs/2-guides/files/b/c.txt"] = file("c")
	fsys["docs/2-guides/.hidden"] = file("hidden")
	_, err = read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
}
