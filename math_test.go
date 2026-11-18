package docpages

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMath checks that $...$, $`...`$, $$...$$, a block between 2 lines of $$
// and a math code block hold their TeX for docs.ts, and that a $ outside the
// rules of $...$, as a price, stays text.
func TestMath(t *testing.T) {
	for name, tt := range map[string]struct{ md, want string }{
		"inline": {
			md:   `Area $\pi r^2$.`,
			want: `<p>Area <span class="math-inline">\pi r^2</span>.</p>`,
		},
		"backticks": {
			md:   dedentMD(`Raw $~ a $1 ~$.`),
			want: `<p>Raw <span class="math-inline"> a $1 </span>.</p>`,
		},
		"display": {
			md:   `So $$x^2$$ shows.`,
			want: `<p>So <span class="math-display">x^2</span> shows.</p>`,
		},
		"escaped dollar in TeX": {
			md:   `$a \$ b$`,
			want: `<span class="math-inline">a \$ b</span>`,
		},
		"Markdown in TeX": {
			md:   `$a_1 * b_2$ and *c*`,
			want: `<p><span class="math-inline">a_1 * b_2</span> and <em>c</em></p>`,
		},
		"lines": {
			md: dedent(`
				Spans $a +
				b$ lines.`),
			want: dedent(`
				<p>Spans <span class="math-inline">a +
				b</span> lines.</p>`),
		},
		"prices":          {md: `From $5 to $10.`, want: `<p>From $5 to $10.</p>`},
		"digit after":     {md: `$x$5`, want: `<p>$x$5</p>`},
		"space inside":    {md: `$ x $`, want: `<p>$ x $</p>`},
		"escaped opener":  {md: `\$x$`, want: `<p>$x$</p>`},
		"code span":       {md: dedentMD(`~$x$~`), want: `<p><code>$x$</code></p>`},
		"empty display":   {md: `$$ $$`, want: `<p>$$ $$</p>`},
		"unclosed inline": {md: `$x`, want: `<p>$x</p>`},
		"block": {
			md: dedent(`
				Text
				$$
				a &= b \\
				$$
			`),
			want: dedent(`
				<p>Text</p>
				<div class="math-display">a &amp;= b \\
				</div>
			`),
		},
		"code block": {
			md: dedentMD(`
				~~~math
				E = mc^2
				~~~
			`),
			want: dedent(`
				<div class="math-display">E = mc^2
				</div>
			`),
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := convert(t, "# T\n\n"+tt.md)
			require.Contains(t, d.body, tt.want)
		})
	}
}

// TestMathPlainText checks that the description and the search text hold the
// TeX of math in the text.
func TestMathPlainText(t *testing.T) {
	d := convert(t, dedent(`
		# T

		Area $\pi r^2$.
	`))
	require.Equal(t, `Area \pi r^2.`, d.description)
	require.Equal(t, `T Area \pi r^2.`, d.search[0].Text)
}
