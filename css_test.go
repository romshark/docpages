package docpages

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckCSSValid checks that docpages, docs, and example styles using nesting,
// light-dark(), and scroll-driven animations pass [minifyCSS].
func TestCheckCSSValid(t *testing.T) {
	for _, name := range []string{
		"web/style.css",
		"docs/style.css",
		"example/cli/docs/style.css",
		"example/library/docs/style.css",
	} {
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		_, err = minifyCSS(name, src)
		require.NoError(t, err)
	}
}

// TestCheckCSS checks line-and-column errors for browser-skipped CSS found by
// esbuild with notes or [scanCSS], and accepts browser-valid CSS.
func TestCheckCSS(t *testing.T) {
	for name, tt := range map[string]struct {
		css  string
		want []string // The errors, each without "style.css:".
	}{
		"fine": {
			css: dedent(`
				@import url("a.css");
				/* Colors. */
				:root {
				  --accent: #0b7a53;
				  color: var(--accent);
				  -webkit-line-clamp: 2;
				  .a & { translate: 0 1px; }
				}
				@media (width < 40rem) { .b { inline-size: 100%; } }
				@font-face {
				  font-family: X;
				  src: url(x.woff2);
				  font-display: swap;
				}
				@keyframes spin { to { rotate: 1turn; } }
				@supports (display: grid) { .c { display: grid; } }
				@layer base;
			`),
		},
		"typo": {
			css: dedent(`
				a {
				  colr: red;
				}
			`),
			want: []string{
				`2:3: "colr" is not a known CSS property (Did you mean "color" instead?)`,
			},
		},
		"no value": {
			css:  "a { color: ; }\n",
			want: []string{"1:5: color without a value"},
		},
		"no colon": {
			css: dedent(`
				a {
				  color red;
				}
			`),
			want: []string{`2:8: Expected ":"`},
		},
		"stray brace": {
			css: dedent(`
				a { color: red; }
				}
			`),
			want: []string{`2:1: Unexpected "}"`},
		},
		"unclosed parenthesis": {
			css: dedent(`
				a {
				  inline-size: calc((100% - 1rem);
				}
			`),
			want: []string{
				`4:1: Expected ")" to go with "(" (The unbalanced "(" is here: 2:20)`,
			},
		},
		"no selector": {
			css:  "{ color: red; }\n",
			want: []string{`1:1: Unexpected "{"`},
		},
		"unclosed block": {
			css: dedent(`
				a {
				  color: red;

				.b { color: blue; }
			`),
			want: []string{
				`5:1: Expected "}" to go with "{" (The unbalanced "{" is here: 1:3)`,
			},
		},
		"unknown at-rule": {
			css:  "@meida (width < 40rem) { a { color: red; } }\n",
			want: []string{"1:1: unknown at-rule @meida"},
		},
		"string": {
			css: dedent(`
				a::before {
				  content: "x;
				}
			`),
			want: []string{"2:15: Unterminated string token"},
		},
		"comment": {
			css: dedent(`
				a { color: red; }
				/* The end`),
			want: []string{
				`2:11: Expected "*/" to terminate multi-line comment ` +
					`(The multi-line comment starts here: 2:1)`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := minifyCSS("style.css", []byte(tt.css))
			if len(tt.want) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			lines := strings.Split(err.Error(), "\n")
			for _, want := range tt.want {
				found := false
				for _, l := range lines {
					found = found || strings.HasPrefix(l, "style.css:"+want)
				}
				require.True(t, found,
					"no error starts with %q in\n%s", "style.css:"+want, err)
			}
		})
	}
}
