package docpages

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// mediaName returns media file name's content-addressed, edition-relative copy path.
func mediaName(data, name string) string {
	sum := sha256.Sum256([]byte(data))
	return path.Join(mediaDir, hex.EncodeToString(sum[:8]), name)
}

// TestMedia checks copies of displayed media from docs or elsewhere in the repository,
// and links to them from Markdown images and raw-HTML media on the page,
// in its Markdown copy, and in llms-full.txt.
// [TestLint] covers rejected raw HTML and attributes.
func TestMedia(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte(dedent(`
		# Testing

		![Shot](img/shot.png) ![Logo](../../assets/logo.png) ![Badge](https://example.com/badge.svg)

		<video src="img/demo.mp4" poster="img/shot.png" controls></video>

		<picture>
		  <source srcset="img/shot.png 2x, img/shot.png">
		  <img src="img/shot.png" alt="Shot">
		</picture>
	`))}
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

// TestMediaSharedReference checks that a page resolves a shared link/image reference
// for each use, while its one copy destination opens the media file for both.
func TestMediaSharedReference(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte(dedent(`
		# Testing

		See [the shot][shot] and ![The shot][shot].

		[shot]: img/shot.png
	`))}
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
	require.Equal(t, fmt.Sprintf(dedent(`
		# Testing

		See [the shot][shot] and ![The shot][shot].

		[shot]: ../%s
	`), shot),
		string(files["main/guides/testing.md"]))
	require.Contains(t, string(files["main/llms-full.txt"]), "[shot]: "+shot+"\n")
}

// mediaAcrossLines is a page with split raw-HTML tags: after an unclosed line
// that starts no HTML block, in a list item, and in a blockquote. dest names the media.
func mediaAcrossLines(dest string) string {
	return fmt.Sprintf(dedent(`
		# Testing

		<img src="%[1]s"
		     alt="Shot" width="10">

		1. Open it:

		   <img src="%[1]s"
		        alt="Shot">

		> [!TIP]
		> <img src="%[1]s"
		>   alt="Shot">
	`), dest)
}

// TestMediaAcrossLines checks that a page copy preserves split raw-HTML tags
// exactly except for replaced URLs, as displayed.
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

// TestMediaErr checks failure for missing media and release media outside its directory.
func TestMediaErr(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte(dedent(`
			# Testing

			![None](img/none.png)
		`)),
	}
	_, err := read(fsys, "docs", noRemote, false)
	require.ErrorContains(t, err, `docs/2-guides/10-testing.md: `+
		`media "img/none.png": no file docs/2-guides/img/none.png`)

	fsys = withReleases(t, testRepoFS(), "v1.2.0")
	fsys["assets/logo.png"] = &fstest.MapFile{Data: []byte("logo")}
	fsys["docs/versions/v1.2/2-guides/10-testing.md"] = &fstest.MapFile{
		Data: []byte(dedent(`
			# Testing

			<img src="../../../../assets/logo.png">
		`)),
	}
	_, err = read(fsys, "docs", noRemote, false)
	require.ErrorContains(t,
		err, `media "../../../../assets/logo.png" is outside docs/versions/v1.2`)
}

// testResolve resolves test raw-HTML URLs, preserving schemes like [site.mediaURL].
func testResolve(dest string) (string, error) {
	if strings.Contains(dest, ":") {
		return dest, nil
	}
	return "/r/" + dest, nil
}

// TestRewriteMedia checks that page HTML keeps only raw-HTML media, resolved
// URLs, and [mediaAttrs], with no script-running URL.
func TestRewriteMedia(t *testing.T) {
	for raw, want := range map[string]string{
		`<p align="center"><img src="a.png" width="50" onerror="x()"></p>`: `<img src="/r/a.png" width="50">`,
		`<img src="javascript:alert(1)" alt="x">`:                          `<img alt="x">`,
		`<source srcset="a.png 1x, b.png 2x">`:                             `<source srcset="/r/a.png 1x, /r/b.png 2x">`,
		`<video controls muted src="v.mp4"></video>`:                       `<video controls muted src="/r/v.mp4"></video>`,
		`<details><summary>More</summary></details>`:                       ``,
	} {
		got, found, err := rewriteMedia(raw, nil, testResolve, testHeader)
		require.NoError(t, err)
		require.Equal(t, want, got, raw)
		require.Equal(t, want != "", found, raw)
	}
}

// testHeader returns the headers of the test images that [TestRewriteMediaSize]
// and [TestRewriteMediaOpaque] name, and zero for other URLs.
// The .png images are transparent and the .jpg images opaque.
func testHeader(dest string) (imageHeader, error) {
	switch dest {
	case "wide.png":
		return imageHeader{width: 1920, height: 1200}, nil
	case "wide-480.png":
		return imageHeader{width: 480, height: 300}, nil
	case "tall.png":
		return imageHeader{width: 780, height: 1200}, nil
	case "poster.png":
		return imageHeader{width: 960, height: 540}, nil
	case "photo.jpg", "photo-dark.jpg":
		return imageHeader{width: 1920, height: 1200, opaque: true}, nil
	case "photo-480.jpg":
		return imageHeader{width: 480, height: 300, opaque: true}, nil
	}
	return imageHeader{}, nil
}

// TestRewriteMediaSize checks the width and height that page HTML gives raw-HTML
// media, including an author's width or height and a <source> whose proportions
// differ from its <img>.
func TestRewriteMediaSize(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		{
			`<img src="wide.png" alt="A">`,
			`<img src="/r/wide.png" alt="A" width="1920" height="1200">`,
		},
		{
			`<img src="wide.png" width="480">`,
			`<img src="/r/wide.png" width="480" height="300">`,
		},
		{
			`<img src="wide.png" height="600">`,
			`<img src="/r/wide.png" height="600" width="960">`,
		},
		{
			`<img src="wide.png" width="480" height="100">`,
			`<img src="/r/wide.png" width="480" height="100">`,
		},
		{
			`<img src="wide.png" width="50%">`,
			`<img src="/r/wide.png" width="50%">`,
		},
		{
			`<img src="https://x.org/a.png">`,
			`<img src="https://x.org/a.png">`,
		},
		{
			`<img srcset="wide.png 2x" alt="A">`,
			`<img srcset="/r/wide.png 2x" alt="A" width="960" height="600">`,
		},
		{
			`<video src="v.mp4" poster="poster.png" controls></video>`,
			`<video src="/r/v.mp4" poster="/r/poster.png" controls` +
				` width="960" height="540"></video>`,
		},
		{
			`<video poster="poster.png" width="480">` +
				`<source src="v.mp4" type="video/mp4"></video>`,
			`<video poster="/r/poster.png" width="480" height="270">` +
				`<source src="/r/v.mp4" type="video/mp4"></video>`,
		},
		// A source with the proportions of the <img> takes its size.
		{
			`<picture><source media="(prefers-color-scheme: dark)" srcset="wide.png">` +
				`<img src="wide.png" alt="A"></picture>`,
			`<picture><source media="(prefers-color-scheme: dark)" ` +
				`srcset="/r/wide.png">` +
				`<img src="/r/wide.png" alt="A" width="1920" height="1200"></picture>`,
		},
		{
			`<picture><source srcset="wide-480.png 480w, wide.png 1920w">` +
				`<img src="wide.png" alt="A"></picture>`,
			`<picture><source srcset="/r/wide-480.png 480w, /r/wide.png 1920w">` +
				`<img src="/r/wide.png" alt="A" width="1920" height="1200"></picture>`,
		},
		{
			`<picture><source media="(max-width: 30rem)" srcset="tall.png">` +
				`<img src="wide.png" alt="A"></picture>`,
			`<picture><source media="(max-width: 30rem)" srcset="/r/tall.png" width="780" height="1200">` +
				`<img src="/r/wide.png" alt="A" width="1920" height="1200"></picture>`,
		},
		{
			`<picture><source srcset="tall.png">` +
				`<img src="wide.png" width="480"></picture>`,
			`<picture><source srcset="/r/tall.png" width="480" height="738">` +
				`<img src="/r/wide.png" width="480" height="300"></picture>`,
		},
		{
			`<picture><source srcset="tall.png">` +
				`<img src="wide.png" width="480" height="300"></picture>`,
			`<picture><source srcset="/r/tall.png"><img src="/r/wide.png" width="480" height="300"></picture>`,
		},
		{
			`<picture><source srcset="tall.png" width="390">` +
				`<img src="wide.png"></picture>`,
			`<picture><source srcset="/r/tall.png" width="390" height="600">` +
				`<img src="/r/wide.png" width="1920" height="1200"></picture>`,
		},
		{
			`<picture><source srcset="tall.png 2x"><img src="wide.png"></picture>`,
			`<picture><source srcset="/r/tall.png 2x" width="390" height="600">` +
				`<img src="/r/wide.png" width="1920" height="1200"></picture>`,
		},
		// Without its <img>, a source can't tell whether it needs a size.
		{
			`<picture><source srcset="tall.png">`,
			`<picture><source srcset="/r/tall.png">`,
		},
	} {
		got, _, err := rewriteMedia(tt.raw, nil, testResolve, testHeader)
		require.NoError(t, err)
		require.Equal(t, tt.want, got, tt.raw)
	}
}

// TestRewriteMediaOpaque checks that page HTML marks an <img> opaque only when
// its src, its srcset and the srcsets of its <picture> are all opaque.
func TestRewriteMediaOpaque(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		{
			`<img src="photo.jpg" alt="A">`,
			`<img src="/r/photo.jpg" alt="A" width="1920" height="1200" data-opaque>`,
		},
		{
			`<img src="photo.jpg" width="480" height="100">`,
			`<img src="/r/photo.jpg" width="480" height="100" data-opaque>`,
		},
		{
			`<img src="photo.jpg" srcset="photo-480.jpg 480w, photo.jpg 1920w">`,
			`<img src="/r/photo.jpg" srcset="/r/photo-480.jpg 480w, /r/photo.jpg 1920w"` +
				` width="1920" height="1200" data-opaque>`,
		},
		{
			`<img src="photo.jpg" srcset="photo-480.jpg 480w, wide.png 1920w">`,
			`<img src="/r/photo.jpg" srcset="/r/photo-480.jpg 480w, /r/wide.png 1920w"` +
				` width="1920" height="1200">`,
		},
		{
			`<img src="https://x.org/a.jpg">`,
			`<img src="https://x.org/a.jpg">`,
		},
		{
			`<picture><source media="(prefers-color-scheme: dark)"` +
				` srcset="photo-dark.jpg"><img src="photo.jpg" alt="A"></picture>`,
			`<picture><source media="(prefers-color-scheme: dark)"` +
				` srcset="/r/photo-dark.jpg">` +
				`<img src="/r/photo.jpg" alt="A" width="1920" height="1200" data-opaque>` +
				`</picture>`,
		},
		{
			`<picture><source media="(prefers-color-scheme: dark)"` +
				` srcset="wide.png"><img src="photo.jpg" alt="A"></picture>`,
			`<picture><source media="(prefers-color-scheme: dark)"` +
				` srcset="/r/wide.png">` +
				`<img src="/r/photo.jpg" alt="A" width="1920" height="1200"></picture>`,
		},
		// A <video> shows its poster in a box of its own, without a placeholder.
		{
			`<video poster="photo.jpg"></video>`,
			`<video poster="/r/photo.jpg" width="1920" height="1200"></video>`,
		},
	} {
		got, _, err := rewriteMedia(tt.raw, nil, testResolve, testHeader)
		require.NoError(t, err)
		require.Equal(t, tt.want, got, tt.raw)
	}
}

// TestMediaSize checks that page HTML sizes and marks an opaque Markdown image
// and raw <img>, while the Markdown copy keeps them as written.
func TestMediaSize(t *testing.T) {
	var shotPNG bytes.Buffer
	require.NoError(t, png.Encode(&shotPNG, image.NewGray(image.Rect(0, 0, 30, 20))))
	page := dedent(`
		# Testing

		![Shot](img/shot.png)

		<img src="img/shot.png" alt="Shot" width="15">
	`)
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte(page)}
	fsys["docs/2-guides/img/shot.png"] = &fstest.MapFile{Data: shotPNG.Bytes()}
	s, err := read(fsys, "docs", noRemote, false)
	require.NoError(t, err)
	shot := "../" + mediaName(shotPNG.String(), "shot.png")

	html := pageHTML(t, s, "guides/testing")
	require.Contains(t, html,
		`<img src="`+shot+`" alt="Shot" width="30" height="20" data-opaque="">`)
	require.Contains(t, html,
		`<img src="`+shot+`" alt="Shot" width="15" height="10" data-opaque>`)

	files, err := s.Files()
	require.NoError(t, err)
	require.Equal(t,
		strings.ReplaceAll(page, "img/shot.png", shot),
		string(files["main/guides/testing.md"]))
}

// TestMediaURLEdits checks that a raw-HTML Markdown copy changes only media URLs,
// preserving split tags, existing quotes, valueless attributes, and trailing
// partial tags, while adding quotes to unquoted values.
func TestMediaURLEdits(t *testing.T) {
	for raw, want := range map[string]string{
		`<p><img class="x" src="a.png"> <b>B</b></p>`: `<p><img class="x" src="/r/a.png"> <b>B</b></p>`,
		dedent(`
			<img
			  src="a.png"
			  alt="A"
			/>`): dedent(`
			<img
			  src="/r/a.png"
			  alt="A"
			/>`),
		dedent(`
			<source srcset="a.png 1x,
			  b.png 2x">`): `<source srcset="/r/a.png 1x, /r/b.png 2x">`,
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

// TestAttrValues checks that attrValues finds the tag attributes and values
// read by golang.org/x/net/html.
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

// TestSnapshotMedia checks that snapshots keep docs media in place,
// move other displayed media under media/, and show the copies.
func TestSnapshotMedia(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte(dedent(`
		# Testing

		![Shot](../images/shot.png) ![Logo](../../assets/logo.png)

		<img src="../../assets/logo.png" width="10">
	`))}
	fsys["docs/images/shot.png"] = &fstest.MapFile{Data: []byte("shot")}
	fsys["assets/logo.png"] = &fstest.MapFile{Data: []byte("logo")}
	_, files, err := snapshot(fsys, "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	const v = "docs/versions/v1.2/"
	require.Equal(t, "shot", string(files[v+"images/shot.png"]))
	require.Equal(t, "logo", string(files[v+"media/assets/logo.png"]))
	require.Equal(t, dedent(`
		# Testing

		![Shot](../images/shot.png) ![Logo](../media/assets/logo.png)

		<img src="../media/assets/logo.png" width="10">
	`),
		string(files[v+"2-guides/10-testing.md"]))

	s, err := read(withReleases(t, fsys, "v1.2.3"), "docs", noRemote, false)
	require.NoError(t, err)
	html := editionPageHTML(t, s, "v1.2", "guides/testing")
	require.Contains(t,
		html, `<img src="../`+mediaName("logo", "logo.png")+`" width="10">`)
}

// TestSnapshotMediaSharedReference checks that a shared link/image reference
// uses the image's media copy, not the tagged URL a lone link would use.
func TestSnapshotMediaSharedReference(t *testing.T) {
	fsys := testRepoFS()
	fsys["docs/2-guides/10-testing.md"] = &fstest.MapFile{Data: []byte(dedent(`
		# Testing

		See [the logo][logo] and ![The logo][logo].

		[logo]: ../../assets/logo.png
	`))}
	fsys["assets/logo.png"] = &fstest.MapFile{Data: []byte("logo")}
	_, files, err := snapshot(fsys, "docs", "v1.2.3", noRemote)
	require.NoError(t, err)
	const v = "docs/versions/v1.2/"
	require.Equal(t, "logo", string(files[v+"media/assets/logo.png"]))
	require.Equal(t, dedent(`
		# Testing

		See [the logo][logo] and ![The logo][logo].

		[logo]: ../media/assets/logo.png
	`),
		string(files[v+"2-guides/10-testing.md"]))
}

// TestSnapshotMediaAcrossLines checks that a snapshot preserves split raw-HTML
// tags except for replaced URLs, which the release displays.
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
