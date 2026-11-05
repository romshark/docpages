package docpages

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// mediaName returns the path of the copy of the media file name with the
// content data relative to the directory of an edition.
func mediaName(data, name string) string {
	sum := sha256.Sum256([]byte(data))
	return path.Join(mediaDir, hex.EncodeToString(sum[:8]), name)
}

// TestMedia tests that the site holds a copy of each media file that a page
// shows, of the docs directory or elsewhere in the repository, and that the
// images of the Markdown and the media elements of its raw HTML lead to the
// copies, on the page, in its Markdown copy and in llms-full.txt. [TestLint]
// tests that the rest of the raw HTML and other attributes fail the build.
func TestMedia(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte("# Testing\n\n" +
		"![Shot](img/shot.png) ![Logo](../../assets/logo.png) " +
		"![Badge](https://example.com/badge.svg)\n\n" +
		`<video src="img/demo.mp4" poster="img/shot.png" controls></video>` + "\n\n" +
		"<picture>\n  <source srcset=\"img/shot.png 2x, img/shot.png\">\n" +
		"  <img src=\"img/shot.png\" alt=\"Shot\">\n</picture>\n")}
	fsys["docs/2-guides/img/shot.png"] = &fstest.MapFile{Data: []byte("shot")}
	fsys["docs/2-guides/img/demo.mp4"] = &fstest.MapFile{Data: []byte("demo")}
	fsys["assets/logo.png"] = &fstest.MapFile{Data: []byte("logo")}
	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	shot := mediaName("shot", "shot.png")
	logo := mediaName("logo", "logo.png")
	demo := mediaName("demo", "demo.mp4")

	html := pageHTML(t, s, "guides/testing")
	for _, want := range []string{
		`<img src="../` + shot + `" alt="Shot">`,
		`<img src="../` + logo + `" alt="Logo">`,
		`<img src="https://example.com/badge.svg" alt="Badge">`,
		`<video src="../` + demo + `" poster="../` + shot + `" controls></video>`,
		`<picture><source srcset="../` + shot + ` 2x, ../` + shot + `">` +
			`<img src="../` + shot + `" alt="Shot"></picture>`,
	} {
		require.Contains(t, html, want)
	}

	files, err := s.Files()
	require.NoError(t, err)
	for name, data := range map[string]string{shot: "shot", logo: "logo", demo: "demo"} {
		require.Equal(t, data, string(files["main/"+name]), name)
	}
	md := string(files["main/guides/testing.md"])
	require.Contains(t, md, "![Shot](../"+shot+") ![Logo](../"+logo+")")
	require.Contains(t, md,
		`<video src="../`+demo+`" poster="../`+shot+`" controls></video>`)
	require.Contains(t, md, `<source srcset="../`+shot+` 2x, ../`+shot+`">`)
	require.Contains(t, string(files["main/llms-full.txt"]), "![Shot]("+shot+")")
}

// TestMediaSharedReference tests that a link and an image that share a
// reference definition each lead where they would alone on the page, and that
// the definition in the Markdown copy, which holds 1 destination for both,
// leads to the copy of the media file, which the link can open too.
func TestMediaSharedReference(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte("# Testing\n\n" +
		"See [the shot][shot] and ![The shot][shot].\n\n" +
		"[shot]: img/shot.png\n")}
	fsys["docs/2-guides/img/shot.png"] = &fstest.MapFile{Data: []byte("shot")}
	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	shot := mediaName("shot", "shot.png")

	html := pageHTML(t, s, "guides/testing")
	require.Contains(t, html, `<a href="`+blob+`docs/2-guides/img/shot.png">the shot</a>`)
	require.Contains(t, html, `<img src="../`+shot+`" alt="The shot">`)

	files, err := s.Files()
	require.NoError(t, err)
	require.Equal(t, "# Testing\n\n"+
		"See [the shot][shot] and ![The shot][shot].\n\n"+
		"[shot]: ../"+shot+"\n",
		string(files["main/guides/testing.md"]))
	require.Contains(t, string(files["main/llms-full.txt"]), "[shot]: "+shot+"\n")
}

// TestMediaErr tests that a media file that the repository lacks and a media
// file of a release outside its directory fail the build.
func TestMediaErr(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte("# Testing\n\n![None](img/none.png)\n"),
	}
	_, err := read(fsys, "docs", noRemote, false)
	require.ErrorContains(t, err, `docs/2-guides/10-testing.md: `+
		`media "img/none.png": no file docs/2-guides/img/none.png`)

	fsys = withReleases(t, testRepoFS(), "v1.2.0")
	fsys["assets/logo.png"] = &fstest.MapFile{Data: []byte("logo")}
	fsys["docs/versions/v1.2/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte("# Testing\n\n<img src=\"../../../../assets/logo.png\">\n"),
	}
	_, err = read(fsys, "docs", noRemote, false)
	require.ErrorContains(t, err, `media "../../../../assets/logo.png" is outside docs/versions/v1.2`)
}

// TestRewriteMedia tests that the HTML of a page holds the media elements of
// raw HTML alone, with their URLs resolved, the attributes of [mediaAttrs]
// and no URL that runs a script, and that the Markdown copy keeps the rest.
func TestRewriteMedia(t *testing.T) {
	// As [site.mediaURL], it keeps a URL with a scheme.
	resolve := func(dest string) (string, error) {
		if strings.Contains(dest, ":") {
			return dest, nil
		}
		return "/r/" + dest, nil
	}
	for raw, want := range map[string]string{
		`<p align="center"><img src="a.png" width="50" onerror="x()"></p>`: `<img src="/r/a.png" width="50">`,
		`<img src="javascript:alert(1)" alt="x">`:                          `<img alt="x">`,
		`<source srcset="a.png 1x, b.png 2x">`:                             `<source srcset="/r/a.png 1x, /r/b.png 2x">`,
		`<video controls muted src="v.mp4"></video>`:                       `<video controls muted src="/r/v.mp4"></video>`,
		`<details><summary>More</summary></details>`:                       ``,
	} {
		got, found, err := rewriteMedia(raw, true, nil, resolve)
		require.NoError(t, err)
		require.Equal(t, want, got, raw)
		require.Equal(t, want != "", found, raw)
	}

	got, found, err := rewriteMedia(`<p><img class="x" src="a.png"> <b>B</b></p>`, false, nil, resolve)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, `<p><img class="x" src="/r/a.png"> <b>B</b></p>`, got)
}

// TestSnapshotMedia tests that a snapshot copies the media files that its pages
// show, those of the docs directory in their place and the others into media/
// of the copy, whose pages then show them.
func TestSnapshotMedia(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte("# Testing\n\n" +
		"![Shot](../images/shot.png) ![Logo](../../assets/logo.png)\n\n" +
		"<img src=\"../../assets/logo.png\" width=\"10\">\n")}
	fsys["docs/images/shot.png"] = &fstest.MapFile{Data: []byte("shot")}
	fsys["assets/logo.png"] = &fstest.MapFile{Data: []byte("logo")}
	_, files, err := snapshot(fsys, "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	const v = "docs/versions/v1.2/"
	require.Equal(t, "shot", string(files[v+"images/shot.png"]))
	require.Equal(t, "logo", string(files[v+"media/assets/logo.png"]))
	require.Equal(t, "# Testing\n\n"+
		"![Shot](../images/shot.png) ![Logo](../media/assets/logo.png)\n\n"+
		"<img src=\"../media/assets/logo.png\" width=\"10\">\n",
		string(files[v+"2-guides/10-testing.md"]))

	s, err := read(withReleases(t, fsys, "v1.2.3"), "docs", noRemote, false)
	require.NoError(t, err)
	html := editionPageHTML(t, s, "v1.2", "guides/testing")
	require.Contains(t, html, `<img src="../`+mediaName("logo", "logo.png")+`" width="10">`)
}

// TestSnapshotMediaSharedReference tests that a snapshot gives a reference definition
// that a link and an image share the destination of the image, the copy of
// the media file, rather than the URL at the tag that the link alone would take.
func TestSnapshotMediaSharedReference(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte("# Testing\n\n" +
		"See [the logo][logo] and ![The logo][logo].\n\n" +
		"[logo]: ../../assets/logo.png\n")}
	fsys["assets/logo.png"] = &fstest.MapFile{Data: []byte("logo")}
	_, files, err := snapshot(fsys, "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	const v = "docs/versions/v1.2/"
	require.Equal(t, "logo", string(files[v+"media/assets/logo.png"]))
	require.Equal(t, "# Testing\n\n"+
		"See [the logo][logo] and ![The logo][logo].\n\n"+
		"[logo]: ../media/assets/logo.png\n",
		string(files[v+"2-guides/10-testing.md"]))
}
