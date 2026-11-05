package docpages

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// testStyle is a style.css with a comment, spaces between its tokens and a
// nested rule, which a minified copy leaves out and keeps.
const testStyle = "/* The colors. */\n:root {\n  --bg: light-dark(#ffffff, #000000);\n\n" +
	"  .docs-page & {\n    color: red;\n  }\n}\n"

// TestStyleMinified tests that the site holds the style.css of the docs
// directory minified, without its comment and spaces, and that [Handler],
// which reads with served, serves it as written.
func TestStyleMinified(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/style.css"] = &fstest.MapFile{Data: []byte(testStyle)}

	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	files, err := s.Files()
	require.NoError(t, err)
	require.Equal(t, ":root{--bg: light-dark(#ffffff, #000000);.docs-page &{color:red}}\n",
		string(files["main/style.css"]))

	s, err = read(fsys, "docs", noRemote, true)
	require.NoError(t, err)
	files, err = s.Files()
	require.NoError(t, err)
	require.Equal(t, testStyle, string(files["main/style.css"]))
}

// TestStyleChecked tests that a mistake in the style.css of the docs directory
// fails the read, whether [Handler] serves the site or not.
func TestStyleChecked(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/style.css"] = &fstest.MapFile{Data: []byte("a {\n  colr: red;\n}\n")}
	for _, served := range []bool{false, true} {
		_, err := read(fsys, "docs", noRemote, served)
		require.EqualError(t, err,
			`docs/style.css:2:3: "colr" is not a known CSS property (Did you mean "color" instead?)`)
	}
}
