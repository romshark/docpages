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
		"static/style.css",
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
			css: "@import url(\"a.css\");\n/* Colors. */\n:root " +
				"{\n  --accent: #0b7a53;\n  color: var(--accent);\n" +
				"  -webkit-line-clamp: 2;\n  .a & { translate: 0 1px; }\n}\n" +
				"@media (width < 40rem) { .b { inline-size: 100%; } }\n" +
				"@font-face { font-family: X; src: url(x.woff2); font-display: swap; }\n" +
				"@keyframes spin { to { rotate: 1turn; } }\n" +
				"@supports (display: grid) { .c { display: grid; } }\n" +
				"@layer base;\n"},
		"typo": {
			css: "a {\n  colr: red;\n}\n",
			want: []string{
				`2:3: "colr" is not a known CSS property (Did you mean "color" instead?)`,
			},
		},
		"no value": {
			css:  "a { color: ; }\n",
			want: []string{"1:5: color without a value"},
		},
		"no colon": {
			css:  "a {\n  color red;\n}\n",
			want: []string{`2:8: Expected ":"`},
		},
		"stray brace": {
			css:  "a { color: red; }\n}\n",
			want: []string{`2:1: Unexpected "}"`},
		},
		"unclosed parenthesis": {
			css: "a {\n  inline-size: calc((100% - 1rem);\n}\n",
			want: []string{
				`4:1: Expected ")" to go with "(" (The unbalanced "(" is here: 2:20)`,
			},
		},
		"no selector": {
			css:  "{ color: red; }\n",
			want: []string{`1:1: Unexpected "{"`},
		},
		"unclosed block": {
			css: "a {\n  color: red;\n\n.b { color: blue; }\n",
			want: []string{
				`5:1: Expected "}" to go with "{" (The unbalanced "{" is here: 1:3)`,
			},
		},
		"unknown at-rule": {
			css:  "@meida (width < 40rem) { a { color: red; } }\n",
			want: []string{"1:1: unknown at-rule @meida"},
		},
		"string": {
			css:  "a::before {\n  content: \"x;\n}\n",
			want: []string{"2:15: Unterminated string token"},
		},
		"comment": {
			css: "a { color: red; }\n/* The end",
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
