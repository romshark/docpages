package docpages

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
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
	require.Contains(t,
		html, `<a href="`+blob+`docs/2-guides/img/shot.png">the shot</a>`)
	require.Contains(t, html, `<img src="../`+shot+`" alt="The shot">`)

	files, err := s.Files()
	require.NoError(t, err)
	require.Equal(t, "# Testing\n\n"+
		"See [the shot][shot] and ![The shot][shot].\n\n"+
		"[shot]: ../"+shot+"\n",
		string(files["main/guides/testing.md"]))
	require.Contains(t, string(files["main/llms-full.txt"]), "[shot]: "+shot+"\n")
}

// mediaAcrossLines is a page with tags of raw HTML whose lines lie apart in the
// file: below a line that the tag starts, which ends no tag and so starts no
// HTML block, in a list item and in a blockquote. dest names the media file.
func mediaAcrossLines(dest string) string {
	return "# Testing\n\n" +
		"<img src=\"" + dest + "\"\n     alt=\"Shot\" width=\"10\">\n\n" +
		"1. Open it:\n\n   <img src=\"" + dest + "\"\n        alt=\"Shot\">\n\n" +
		"> [!TIP]\n> <img src=\"" + dest + "\"\n>   alt=\"Shot\">\n"
}

// TestMediaAcrossLines tests that the Markdown copy of a page keeps a tag of
// raw HTML whose lines lie apart in the file whole and as written, with its URL
// replaced, as the page shows it.
func TestMediaAcrossLines(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte(mediaAcrossLines("img/shot.png")),
	}
	fsys["docs/2-guides/img/shot.png"] = &fstest.MapFile{Data: []byte("shot")}
	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	shot := mediaName("shot", "shot.png")

	html := pageHTML(t, s, "guides/testing")
	require.Contains(t, html, `<img src="../`+shot+`" alt="Shot" width="10">`)
	require.Equal(t, 2, strings.Count(html, `<img src="../`+shot+`" alt="Shot">`))

	files, err := s.Files()
	require.NoError(t, err)
	require.Equal(t,
		mediaAcrossLines("../"+shot),
		string(files["main/guides/testing.md"]))
	require.Contains(t, string(files["main/llms-full.txt"]), mediaAcrossLines(shot))
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

// testResolve resolves a URL of raw HTML for the tests: as [site.mediaURL], it
// keeps a URL with a scheme.
func testResolve(dest string) (string, error) {
	if strings.Contains(dest, ":") {
		return dest, nil
	}
	return "/r/" + dest, nil
}

// TestRewriteMedia tests that the HTML of a page holds the media elements of
// raw HTML alone, with their URLs resolved, the attributes of [mediaAttrs]
// and no URL that runs a script.
func TestRewriteMedia(t *testing.T) {
	for raw, want := range map[string]string{
		`<p align="center"><img src="a.png" width="50" onerror="x()"></p>`: `<img src="/r/a.png" width="50">`,
		`<img src="javascript:alert(1)" alt="x">`:                          `<img alt="x">`,
		`<source srcset="a.png 1x, b.png 2x">`:                             `<source srcset="/r/a.png 1x, /r/b.png 2x">`,
		`<video controls muted src="v.mp4"></video>`:                       `<video controls muted src="/r/v.mp4"></video>`,
		`<details><summary>More</summary></details>`:                       ``,
	} {
		got, found, err := rewriteMedia(raw, nil, testResolve)
		require.NoError(t, err)
		require.Equal(t, want, got, raw)
		require.Equal(t, want != "", found, raw)
	}
}

// TestMediaURLEdits tests that the Markdown copy of raw HTML replaces the URLs
// of its media elements alone and keeps the rest as written: a tag that spans
// lines, the quotes of a value, which a value without them gains, an
// attribute without a value, and a tag that the raw HTML ends in the middle of.
func TestMediaURLEdits(t *testing.T) {
	for raw, want := range map[string]string{
		`<p><img class="x" src="a.png"> <b>B</b></p>`:       `<p><img class="x" src="/r/a.png"> <b>B</b></p>`,
		"<img\n  src=\"a.png\"\n  alt=\"A\"\n/>":            "<img\n  src=\"/r/a.png\"\n  alt=\"A\"\n/>",
		"<source srcset=\"a.png 1x,\n  b.png 2x\">":         `<source srcset="/r/a.png 1x, /r/b.png 2x">`,
		`<img src='a.png' alt=x/>`:                          `<img src='/r/a.png' alt=x/>`,
		`<IMG SRC=a.png Width=10>`:                          `<IMG SRC="/r/a.png" Width=10>`,
		`<img src="a.png?w=1&amp;h=2">`:                     `<img src="/r/a.png?w=1&amp;h=2">`,
		`<img src="a.png" src="b.png">`:                     `<img src="/r/a.png" src="b.png">`,
		`<video src controls poster="https://x.org/p.png">`: `<video src controls poster="https://x.org/p.png">`,
		`<b>B</b> <img alt="A" src="a.png"`:                 `<b>B</b> <img alt="A" src="a.png"`,
	} {
		edits, err := mediaURLEdits(raw, testResolve)
		require.NoError(t, err)
		require.Equal(t, want, string(applyEdits([]byte(raw), edits)), raw)
	}
}

// TestAttrValues tests that attrValues finds the attributes of a tag that the
// tokenizer of golang.org/x/net/html reads, with their values.
func TestAttrValues(t *testing.T) {
	for _, tag := range []string{
		`<img>`,
		`<img/>`,
		`<img src="a" alt='b "c"' width=10 controls>`,
		`<IMG SRC="a" Alt=B>`,
		"<img\tsrc = \"a\"\n alt\f=\r'b'>",
		`<img src="a" src="b" SRC="c">`,
		`<img ="a" src="b">`,
		`<img src=a/>`,
		`<img src="a"/>`,
		`<img / src="a" / alt="b">`,
		`<img src=>`,
		`<img src= >`,
		`<img src="a"alt="b">`,
		`<img src=a"b alt=c'd>`,
		`<img a=b=c d==e>`,
		"<img src\x00=\"a\">",
	} {
		z := html.NewTokenizer(strings.NewReader(tag))
		tt := z.Next()
		require.Contains(t,
			[]html.TokenType{html.StartTagToken, html.SelfClosingTagToken}, tt, tag)
		tok := z.Token()
		vals := attrValues(tag)
		require.Len(t, vals, len(tok.Attr), tag)
		for i, a := range tok.Attr {
			require.Equal(t, a.Key, vals[i].key, tag)
			require.Equal(t, a.Val, tag[vals[i].start:vals[i].end], tag)
		}
	}
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
	require.Contains(t,
		html, `<img src="../`+mediaName("logo", "logo.png")+`" width="10">`)
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

// TestSnapshotMediaAcrossLines tests that a snapshot keeps a tag of raw HTML
// whose lines lie apart in the file whole and as written, with its URL replaced,
// which the release then shows.
func TestSnapshotMediaAcrossLines(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte(mediaAcrossLines("../../assets/shot.png")),
	}
	fsys["assets/shot.png"] = &fstest.MapFile{Data: []byte("shot")}
	_, files, err := snapshot(fsys, "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	require.Equal(t, mediaAcrossLines("../media/assets/shot.png"),
		string(files["docs/versions/v1.2/2-guides/10-testing.md"]))

	s, err := read(withReleases(t, fsys, "v1.2.3"), "docs", noRemote, false)
	require.NoError(t, err)
	html := editionPageHTML(t, s, "v1.2", "guides/testing")
	shot := mediaName("shot", "shot.png")
	require.Contains(t, html, `<img src="../`+shot+`" alt="Shot" width="10">`)
	require.Equal(t, 2, strings.Count(html, `<img src="../`+shot+`" alt="Shot">`))
}
