package docpages

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
)

// imageHeader is what docpages reads from the header of an image file.
type imageHeader struct {
	// width and height are the pixel size, swapped when a JPEG Exif orientation
	// or an AVIF irot turns the image a quarter.
	width, height int
	// opaque reports whether the image has no transparent pixels.
	opaque bool
}

// readImageHeader reads PNG, JPEG, GIF, WebP, AVIF, and SVG with an absolute
// width and height. GIF and SVG count as transparent, since their headers don't say.
func readImageHeader(data []byte) (img imageHeader, ok bool) {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		img, ok = decodeConfig(png.DecodeConfig, data)
		img.opaque = ok && pngOpaque(data)
		return img, ok
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		img, ok = decodeConfig(jpeg.DecodeConfig, data)
		if ok && jpegTransposed(data) {
			img.width, img.height = img.height, img.width
		}
		img.opaque = ok
		return img, ok
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return decodeConfig(gif.DecodeConfig, data)
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return webpHeader(data)
	case len(data) >= 8 && string(data[4:8]) == "ftyp":
		return avifHeader(data)
	}
	return svgHeader(data)
}

// decodeConfig returns the size that decode reads, leaving opaque false.
func decodeConfig(
	decode func(io.Reader) (image.Config, error), data []byte,
) (img imageHeader, ok bool) {
	c, err := decode(bytes.NewReader(data))
	if err != nil || c.Width <= 0 || c.Height <= 0 {
		return imageHeader{}, false
	}
	return imageHeader{width: c.Width, height: c.Height}, true
}

// pngOpaque reports whether PNG data has no alpha channel and no tRNS chunk,
// which both precede IDAT.
func pngOpaque(data []byte) bool {
	p := data[8:]
	// A chunk is length, type, data and CRC.
	for len(p) >= 12 {
		n := int64(binary.BigEndian.Uint32(p))
		switch string(p[4:8]) {
		case "IHDR":
			// Byte 9 of IHDR is the color type, and types 4 and 6 have alpha.
			if n < 13 || len(p) < 8+13 || p[8+9] == 4 || p[8+9] == 6 {
				return false
			}
		case "tRNS":
			return false
		case "IDAT":
			return true
		}
		if 12+n > int64(len(p)) {
			return false
		}
		p = p[12+n:]
	}
	return false
}

// jpegTransposed reports whether the Exif orientation of JPEG data is 5 to 8.
// Browsers apply it, turning the image a quarter.
func jpegTransposed(data []byte) bool {
	p := data[2:]
	for len(p) >= 4 && p[0] == 0xff {
		marker := p[1]
		switch marker {
		// A marker may follow fill bytes.
		case 0xff:
			p = p[1:]
			continue
		// Exif comes before the scan, which isn't made of segments.
		case 0xda, 0xd9:
			return false
		}
		n := int(binary.BigEndian.Uint16(p[2:]))
		if n < 2 || 2+n > len(p) {
			return false
		}
		seg := p[4 : 2+n]
		if marker == 0xe1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			o := exifOrientation(seg[6:])
			return o >= 5 && o <= 8
		}
		p = p[2+n:]
	}
	return false
}

// exifOrientation returns the Orientation tag of the first IFD of Exif TIFF
// data, or 0 without one.
func exifOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	if bo.Uint16(tiff[2:]) != 42 {
		return 0
	}
	off := int64(bo.Uint32(tiff[4:]))
	if off+2 > int64(len(tiff)) {
		return 0
	}
	ifd := tiff[off:]
	n := int(bo.Uint16(ifd))
	ifd = ifd[2:]
	// 12-byte entries of tag, type, count and a SHORT value padded to 4 bytes.
	for ; n > 0 && len(ifd) >= 12; n-- {
		if bo.Uint16(ifd) == 0x0112 {
			return int(bo.Uint16(ifd[8:]))
		}
		ifd = ifd[12:]
	}
	return 0
}

// webpHeader reads the VP8, VP8L or VP8X chunk that starts WebP data,
// as RFC 9649 lays them out.
func webpHeader(data []byte) (img imageHeader, ok bool) {
	if len(data) < 30 {
		return imageHeader{}, false
	}
	b := data[20:]
	switch string(data[12:16]) {
	case "VP8 ":
		// A 3-byte frame tag, the start code and 14-bit sizes. VP8 has no alpha.
		if b[3] != 0x9d || b[4] != 0x01 || b[5] != 0x2a {
			return imageHeader{}, false
		}
		img = imageHeader{
			width:  int(binary.LittleEndian.Uint16(b[6:]) & 0x3fff),
			height: int(binary.LittleEndian.Uint16(b[8:]) & 0x3fff),
			opaque: true,
		}
		return img, img.width > 0 && img.height > 0
	case "VP8L":
		// Signature, then width-1 and height-1 in 14 bits each, then the alpha hint.
		if b[0] != 0x2f {
			return imageHeader{}, false
		}
		bits := binary.LittleEndian.Uint32(b[1:])
		return imageHeader{
			width:  int(bits&0x3fff) + 1,
			height: int(bits>>14&0x3fff) + 1,
			opaque: bits>>28&1 == 0,
		}, true
	case "VP8X":
		// Flags, 3 reserved bytes, then width-1 and height-1 in 24 bits each.
		// Animation frames may leave the canvas transparent.
		const alpha, animation = 0x10, 0x02
		return imageHeader{
			width:  int(uint24(b[4:])) + 1,
			height: int(uint24(b[7:])) + 1,
			opaque: b[0]&(alpha|animation) == 0,
		}, true
	}
	return imageHeader{}, false
}

// uint24 reads a little-endian 24-bit integer.
func uint24(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
}

// avifHeader reads the ispe and irot of the primary item of AVIF data, from the
// HEIF boxes of ISO/IEC 23008-12. ok is false with a clap, which crops the
// image, and for MP4 and HEIC.
func avifHeader(data []byte) (img imageHeader, ok bool) {
	top, ok := isoBoxes(data)
	if !ok || len(top) == 0 || top[0].typ != "ftyp" || !avifBrand(top[0].data) {
		return imageHeader{}, false
	}
	// meta and pitm are full boxes, which start with a version and 3 bytes of flags.
	meta := findBox(top, "meta")
	if len(meta) < 4 {
		return imageHeader{}, false
	}
	inMeta, ok := isoBoxes(meta[4:])
	if !ok {
		return imageHeader{}, false
	}
	var primary uint32
	switch pitm := findBox(inMeta, "pitm"); {
	case len(pitm) >= 6 && pitm[0] == 0:
		primary = uint32(binary.BigEndian.Uint16(pitm[4:]))
	case len(pitm) >= 8 && pitm[0] > 0:
		primary = binary.BigEndian.Uint32(pitm[4:])
	default:
		return imageHeader{}, false
	}
	inIprp, ok := isoBoxes(findBox(inMeta, "iprp"))
	if !ok {
		return imageHeader{}, false
	}
	props, ok := isoBoxes(findBox(inIprp, "ipco"))
	if !ok {
		return imageHeader{}, false
	}
	// An alpha plane is an auxiliary item with an auxC property that names it.
	opaque := !slices.ContainsFunc(props, alphaAux)
	var w, h int
	var turned bool
	for _, b := range inIprp {
		if b.typ != "ipma" {
			continue
		}
		indices, ok := ipmaIndices(b.data, primary)
		if !ok {
			return imageHeader{}, false
		}
		for _, i := range indices {
			if i < 1 || i > len(props) {
				return imageHeader{}, false
			}
			switch p := props[i-1]; p.typ {
			case "ispe":
				if len(p.data) < 12 {
					return imageHeader{}, false
				}
				w = int(binary.BigEndian.Uint32(p.data[4:]))
				h = int(binary.BigEndian.Uint32(p.data[8:]))
			case "irot":
				if len(p.data) < 1 {
					return imageHeader{}, false
				}
				turned = p.data[0]&1 == 1
			case "clap":
				return imageHeader{}, false
			}
		}
	}
	if w <= 0 || h <= 0 {
		return imageHeader{}, false
	}
	if turned {
		w, h = h, w
	}
	return imageHeader{width: w, height: h, opaque: opaque}, true
}

// alphaAux reports whether b is the auxC property of an alpha plane,
// in the URN of AV1 or of HEVC.
func alphaAux(b isoBox) bool {
	if b.typ != "auxC" || len(b.data) < 4 {
		return false
	}
	urn, _, _ := bytes.Cut(b.data[4:], []byte{0})
	switch string(urn) {
	case "urn:mpeg:mpegB:cicp:systems:auxiliary:alpha", "urn:mpeg:hevc:2015:auxid:1":
		return true
	}
	return false
}

// avifBrand reports whether an ftyp box names the brand of
// an AVIF image or image sequence.
func avifBrand(ftyp []byte) bool {
	if len(ftyp) < 8 {
		return false
	}
	// The major brand, a minor version, then the compatible brands.
	brands := append(ftyp[:4:4], ftyp[8:]...)
	for ; len(brands) >= 4; brands = brands[4:] {
		if b := string(brands[:4]); b == "avif" || b == "avis" {
			return true
		}
	}
	return false
}

// ipmaIndices returns the 1-based ipco indices of the properties that the
// full box ipma associates with item id.
func ipmaIndices(ipma []byte, id uint32) (indices []int, ok bool) {
	if len(ipma) < 8 {
		return nil, false
	}
	version, wide := ipma[0], ipma[3]&1 == 1
	n := binary.BigEndian.Uint32(ipma[4:])
	p := ipma[8:]
	for ; n > 0; n-- {
		var item uint32
		switch {
		case version == 0 && len(p) >= 3:
			item, p = uint32(binary.BigEndian.Uint16(p)), p[2:]
		case version > 0 && len(p) >= 5:
			item, p = binary.BigEndian.Uint32(p), p[4:]
		default:
			return nil, false
		}
		count := int(p[0])
		p = p[1:]
		size := 1
		if wide {
			size = 2
		}
		if len(p) < count*size {
			return nil, false
		}
		// The top bit of each association marks it essential.
		for range count {
			i := int(p[0] & 0x7f)
			if wide {
				i = int(binary.BigEndian.Uint16(p) & 0x7fff)
			}
			if item == id {
				indices = append(indices, i)
			}
			p = p[size:]
		}
	}
	return indices, true
}

// isoBox is the 4-character type and the payload of an ISO BMFF box.
type isoBox struct {
	typ  string
	data []byte
}

// isoBoxes splits data into boxes. ok is false when a box overruns data.
func isoBoxes(data []byte) (boxes []isoBox, ok bool) {
	for len(data) > 0 {
		if len(data) < 8 {
			return nil, false
		}
		size, head := uint64(binary.BigEndian.Uint32(data)), uint64(8)
		switch size {
		// The last box may run to the end of the file.
		case 0:
			size = uint64(len(data))
		// A 64-bit size follows the type.
		case 1:
			if len(data) < 16 {
				return nil, false
			}
			size, head = binary.BigEndian.Uint64(data[8:]), 16
		}
		if size < head || size > uint64(len(data)) {
			return nil, false
		}
		boxes = append(boxes, isoBox{typ: string(data[4:8]), data: data[head:size]})
		data = data[size:]
	}
	return boxes, true
}

// findBox returns the payload of the first box of type typ, or nil.
func findBox(boxes []isoBox, typ string) []byte {
	for _, b := range boxes {
		if b.typ == typ {
			return b.data
		}
	}
	return nil
}

// svgHeader reads the width and height of the root <svg> when both are in px
// or unitless. Other units depend on the page, and a viewBox gives only a ratio.
func svgHeader(data []byte) (img imageHeader, ok bool) {
	doc := bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\ufeff")), " \t\r\n")
	if !bytes.HasPrefix(doc, []byte("<")) {
		return imageHeader{}, false
	}
	d := xml.NewDecoder(bytes.NewReader(doc))
	for {
		t, err := d.Token()
		if err != nil {
			return imageHeader{}, false
		}
		el, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		if el.Name.Local != "svg" {
			return imageHeader{}, false
		}
		var w, h int
		var okW, okH bool
		for _, a := range el.Attr {
			switch {
			case a.Name.Space != "":
			case a.Name.Local == "width":
				w, okW = svgLength(a.Value)
			case a.Name.Local == "height":
				h, okH = svgLength(a.Value)
			}
		}
		if !okW || !okH {
			return imageHeader{}, false
		}
		return imageHeader{width: w, height: h}, true
	}
}

// svgLength returns a length in px, as "24" or "24.5px", rounded to a pixel.
func svgLength(s string) (int, bool) {
	f, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "px"), 64)
	if err != nil || f <= 0 || math.IsInf(f, 0) {
		return 0, false
	}
	return max(1, int(math.Round(f))), true
}
