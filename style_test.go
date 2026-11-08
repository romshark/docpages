package docpages

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// testStyle has a comment, token spacing, and a nested rule; minification drops
// the first two and keeps the last.
const testStyle = `/* The colors. */
:root {
  --bg: light-dark(#ffffff, #000000);

  .docs-page & {
    color: red;
  }
}
`

// TestStyleMinified checks that sites minify docs style.css comments and spaces,
// while [Handler]'s served mode keeps it unchanged.
func TestStyleMinified(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/style.css"] = &fstest.MapFile{Data: []byte(testStyle)}

	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	files, err := s.Files()
	require.NoError(t, err)
	require.Equal(t,
		":root{--bg: light-dark(#ffffff, #000000);.docs-page &{color:red}}\n",
		string(files["main/style.css"]))

	s, err = read(fsys, "docs", noRemote, true)
	require.NoError(t, err)
	files, err = s.Files()
	require.NoError(t, err)
	require.Equal(t, testStyle, string(files["main/style.css"]))
}

// TestStyleChecked checks that invalid docs style.css fails in static and served modes.
func TestStyleChecked(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/style.css"] = &fstest.MapFile{Data: []byte(dedent(`
		a {
		  colr: red;
		}
	`))}
	for _, served := range []bool{false, true} {
		_, err := read(fsys, "docs", noRemote, served)
		require.EqualError(t, err,
			`docs/style.css:2:3: "colr" is not a known CSS property `+
				`(Did you mean "color" instead?)`)
	}
}
