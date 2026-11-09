package docpages

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestImageSize checks the size and the opacity that readImageHeader reads
// from encoded images, the files of testdata/size and SVG, and that it reads
// nothing from other data.
func TestImageSize(t *testing.T) {
	rect := image.Rect(0, 0, 3, 2)
	rgba := image.NewRGBA(rect)
	encode := func(enc func(*bytes.Buffer) error) []byte {
		var b bytes.Buffer
		require.NoError(t, enc(&b))
		return b.Bytes()
	}
	pngOf := func(img image.Image) []byte {
		return encode(func(b *bytes.Buffer) error { return png.Encode(b, img) })
	}
	alphaPNG := pngOf(rgba)
	file := func(name string) []byte {
		data, err := os.ReadFile("testdata/size/" + name)
		require.NoError(t, err)
		return data
	}
	opaque := func(w, h int) imageHeader { return imageHeader{w, h, true} }
	transparent := func(w, h int) imageHeader { return imageHeader{w, h, false} }
	for name, tt := range map[string]struct {
		data []byte
		want imageHeader
	}{
		"png alpha":  {alphaPNG, transparent(3, 2)},
		"png opaque": {pngOf(image.NewGray(rect)), opaque(3, 2)},
		// A palette with a transparent color writes a tRNS chunk.
		"png tRNS": {pngOf(image.NewPaletted(rect,
			color.Palette{color.Transparent, color.Black})), transparent(3, 2)},
		"jpeg": {encode(func(b *bytes.Buffer) error {
			return jpeg.Encode(b, rgba, nil)
		}), opaque(3, 2)},
		"gif": {encode(func(b *bytes.Buffer) error {
			return gif.Encode(b, rgba, nil)
		}), transparent(3, 2)},
		// Exif orientation 6 in big-endian TIFF and 8 in little-endian TIFF.
		"jpeg turned right": {file("right-top.jpg"), opaque(2, 3)},
		"jpeg turned left":  {file("left-bottom.jpg"), opaque(2, 3)},
		"webp lossy":        {file("lossy.webp"), opaque(3, 2)},
		"webp lossless":     {file("lossless.webp"), opaque(3, 2)},
		"webp lossless alpha": {
			file("lossless-alpha.webp"), transparent(3, 2),
		},
		"webp extended": {file("alpha.webp"), transparent(3, 2)},
		"avif":          {file("plain.avif"), opaque(3, 2)},
		"avif alpha":    {file("alpha.avif"), transparent(3, 2)},
		"avif irot":     {file("irot.avif"), opaque(2, 3)},
		// The primary item is a grid of 2 tiles of 128x128.
		"avif grid": {file("grid.avif"), opaque(256, 128)},
		"avif clap": {file("clap.avif"), imageHeader{}},
		// The ispe of another item comes before or after that of the primary item.
		"avif primary item":           {testAVIF(false), opaque(64, 128)},
		"avif primary item 32-bit id": {testAVIF(true), opaque(64, 128)},
		"svg": {
			[]byte(`<svg width="24" height="16"></svg>`), transparent(24, 16),
		},
		// A prolog before the root, and a fractional width in px.
		"svg px": {[]byte(dedent(`
			<?xml version="1.0" encoding="UTF-8"?>
			<!-- Logo -->
			<svg xmlns="http://www.w3.org/2000/svg" width="24.6px" height="16px"/>
		`)), transparent(25, 16)},
		"svg em":      {[]byte(`<svg width="2em" height="1em"/>`), imageHeader{}},
		"svg percent": {[]byte(`<svg width="100%" height="50%"/>`), imageHeader{}},
		"svg viewBox": {[]byte(`<svg viewBox="0 0 24 16"/>`), imageHeader{}},
		"svg in html": {
			[]byte(`<html><svg width="24" height="16"/></html>`), imageHeader{},
		},
		"text":           {[]byte("shot"), imageHeader{}},
		"png truncated":  {alphaPNG[:16], imageHeader{}},
		"webp truncated": {[]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), imageHeader{}},
		"mp4": {
			[]byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2"),
			imageHeader{},
		},
	} {
		img, ok := readImageHeader(tt.data)
		require.Equal(t, tt.want, img, name)
		require.Equal(t, tt.want.width > 0, ok, name)
	}
}

// testAVIF returns an AVIF header whose primary item 2 has a 128x64 ispe and a
// 90-degree irot, and item 1 a 64x64 ispe. wide puts the ispe of item 2 first,
// with 32-bit item IDs and 16-bit property indices.
func testAVIF(wide bool) []byte {
	u16 := func(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }
	u32 := func(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
	ispe := func(w, h uint32) []byte { return testBox("ispe", u32(0), u32(w), u32(h)) }
	irot := testBox("irot", []byte{1})
	// Full boxes start with a version and 3 bytes of flags.
	// The top bit of an ipma property index marks it essential.
	pitm := testBox("pitm", u32(0), u16(2))
	ipco := testBox("ipco", ispe(64, 64), ispe(128, 64), irot)
	ipma := testBox("ipma", u32(0), u32(2),
		u16(1), []byte{1, 1},
		u16(2), []byte{2, 0x82, 3})
	if wide {
		pitm = testBox("pitm", []byte{1, 0, 0, 0}, u32(2))
		ipco = testBox("ipco", ispe(128, 64), ispe(64, 64), irot)
		ipma = testBox("ipma", []byte{1, 0, 0, 1}, u32(2),
			u32(1), []byte{1}, u16(2),
			u32(2), []byte{2}, u16(0x8001), u16(3))
	}
	return slices.Concat(
		testBox("ftyp", []byte("avif"), u32(0), []byte("mif1avif")),
		testBox("meta", u32(0), pitm, testBox("iprp", ipco, ipma)),
	)
}

// testBox returns an ISO BMFF box of type typ around the concatenated payload.
func testBox(typ string, payload ...[]byte) []byte {
	p := slices.Concat(payload...)
	return slices.Concat(
		binary.BigEndian.AppendUint32(nil, uint32(8+len(p))),
		[]byte(typ),
		p,
	)
}
