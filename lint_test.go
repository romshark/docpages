package docpages

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/a-h/templ"
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
		"fine": {md: dedentMD(`
			# Testing

			~~~go
			func f() {}
			~~~

			~~~
			plain
			~~~

			~~~mermaid
			flowchart LR
			~~~

			~~~math
			E = mc^2
			~~~

			<!-- A comment. -->

			| A | B |
			| --- | --- |
			| 1 | 2 |

			- One
			- Two

			1. One
			2. Two

			*Emphasis* at the start
			-- a dash, and 2 in
			2.5 seconds.

			2.x is the current version, and the project started in
			2019. It grew.

			<details open>
			<summary>More <code>here</code></summary>

			Text.

			</details>

			Press <kbd>Ctrl</kbd>+<kbd>K</kbd>: H<sub>2</sub>O, x<sup>2</sup>,
			<ins>new</ins><br>and<br/>breaks.

			<br>

			<video controls muted><track kind="captions" src="https://example.com/c.vtt"></video>
		`)},
		"code language": {
			md: dedentMD(`
				---
				title: Testing
				---

				# Testing

				~~~nolang
				x
				~~~
			`),
			want: []string{`7: code block in "nolang", a language that Chroma doesn't highlight`},
		},
		"raw HTML": {
			md: dedent(`
				# Testing

				<div align="center">

				Text.

				</div>

				Press <kbd class="key">K</kbd>.

				<img src="https://example.com/a.png" class="wide">

				<img src="javascript:alert(1)">

				<details>
				<summary class="s">More</summary>
				<script>alert(1)</script>
				</details>
			`),
			want: []string{
				"3: raw HTML <div>, which a page leaves out",
				"9: attribute class of <kbd>, which a page leaves out",
				"11: attribute class of <img>, which a page leaves out",
				`13: src "javascript:alert(1)" of <img>, a URL that a page leaves out`,
				"15: attribute class of <summary>, which a page leaves out",
				"15: raw HTML <script>, which a page leaves out",
			},
		},
		"table": {
			md: dedent(`
				# Testing

				| A | B |
				| 1 | 2 |
			`),
			want: []string{`3: lines that start with "|" but make no table`},
		},
		"list": {
			md: dedent(`
				# Testing

				-item

				Steps:
				2. Run it

				1.First
			`),
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
			_, err := read(fsys, "docs", noRemote, nil, false)
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

// TestLintRepoDocs checks that the docs of docpages and the examples pass every
// check of a build, with the repository set in their front matter.
func TestLintRepoDocs(t *testing.T) {
	for _, dir := range []string{"docs", "example/cli/docs", "example/library/docs"} {
		t.Run(dir, func(t *testing.T) {
			s, err := read(os.DirFS("."), dir, noRemote, gitTags{root: "."}, false)
			require.NoError(t, err)
			_, err = s.Files()
			require.NoError(t, err)
		})
	}
}

// buildFS builds the site of the docs directory in fsys, with pages from page
// unless it's nil, and returns the error.
func buildFS(fsys fstest.MapFS, page func(Page) templ.Component) error {
	s, err := read(fsys, "docs", noRemote, nil, false)
	if err != nil {
		return err
	}
	if page != nil {
		s.page = page
	}
	_, err = s.Files()
	return err
}

// TestUnusedFiles checks that unshown docs files unnamed by links or settings fail,
// such as obsolete images or pages in unpositioned directories,
// while linked directory contents, group icons and files that only the page
// template links count as used.
func TestUnusedFiles(t *testing.T) {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

	fsys := testRepoFS()
	fsys["docs/2-guides/img/old.png"] = file("old")
	fsys["docs/guides/draft.md"] = file("# Draft\n")
	err := buildFS(fsys, nil)
	require.EqualError(t, err, dedent(`
		docs/2-guides/img/old.png: no page shows this file, and no link or setting names it
		docs/guides/draft.md: no page shows this file, and no link or setting names it`))

	fsys = testRepoFS()
	fsys[lintPage] = file(dedent(`
		# Testing

		See [the files](files/).
	`))
	fsys["docs/2-guides/files/a.txt"] = file("a")
	fsys["docs/2-guides/files/b/c.txt"] = file("c")
	fsys["docs/2-guides/.hidden"] = file("hidden")
	require.NoError(t, buildFS(fsys, nil))

	fsys = testRepoFS()
	fsys["docs/files/cheatsheet.pdf"] = file("pdf")
	require.EqualError(t, buildFS(fsys, nil), "docs/files/cheatsheet.pdf: "+
		"no page shows this file, and no link or setting names it")
	// As HeaderLinks does with a Link of URL "files/cheatsheet.pdf".
	linking := func(p Page) templ.Component {
		return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			if _, err := p.URL("files/cheatsheet.pdf"); err != nil {
				return err
			}
			return DefaultPage(p).Render(ctx, w)
		})
	}
	require.NoError(t, buildFS(fsys, linking))
}
