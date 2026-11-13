package docpages

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/renderer"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
	"golang.org/x/net/html"
)

// mediaDir holds an edition's displayed media copies.
// Content-named subdirectories separate files with the same name.
const mediaDir = "media"

// mediaHashSize is the number of bytes of the SHA-256 of a copy's content
// that name its subdirectory in hex.
const mediaHashSize = 8

// isMediaCopy reports whether version-relative name is a media copy,
// as media/0a1b2c3d4e5f6a7b/shot.webp.
// The pages of a docs group named media lie in media/ too, without the hash.
func isMediaCopy(name string) bool {
	rest, ok := strings.CutPrefix(name, mediaDir+"/")
	if !ok {
		return false
	}
	hash, file, ok := strings.Cut(rest, "/")
	return ok && len(hash) == 2*mediaHashSize &&
		strings.Trim(hash, "0123456789abcdef") == "" &&
		file != "" && !strings.Contains(file, "/")
}

// mediaCopy is the copy of a media file in the site.
type mediaCopy struct {
	name string // The path of the copy relative to the directory of the edition.
	data []byte
	// header is that of an image that [readImageHeader] reads, or zero.
	header imageHeader
}

// mediaElements are raw Markdown HTML that pages show: images, video, audio,
// sources, and text tracks. goldmark omits all other raw HTML.
var mediaElements = map[string]bool{
	"img":     true,
	"picture": true,
	"source":  true,
	"video":   true,
	"audio":   true,
	"track":   true,
}

// mediaAttrs are preserved media attributes;
// scripts such as onerror and styles are omitted.
var mediaAttrs = map[string]bool{
	"alt": true, "title": true, "width": true, "height": true, "align": true,
	"loading": true, "decoding": true, "src": true, "srcset": true, "sizes": true,
	"media": true, "type": true, "poster": true, "controls": true, "autoplay": true,
	"loop": true, "muted": true, "playsinline": true, "preload": true, "kind": true,
	"srclang": true, "label": true, "default": true,
}

// mediaURL returns dest's site URL as named by pages[from]. base is the
// version-root URL at the read location. URLs with a scheme or host stay
// unchanged. Other paths resolve file-relative, as on GitHub, to repository
// files copied into [mediaDir].
func (s *site) mediaURL(from int, dest, base string) (string, error) {
	u, c, err := s.mediaFile(from, dest)
	if err != nil {
		return "", err
	}
	if c == nil {
		return dest, nil
	}
	return withQueryFragment(base+(&url.URL{Path: c.name}).EscapedPath(), u), nil
}

// mediaHeader returns the header of the image file that dest names on pages[from].
// It's zero for a URL with a scheme or host, and for a file that
// [readImageHeader] doesn't read.
func (s *site) mediaHeader(from int, dest string) (imageHeader, error) {
	_, c, err := s.mediaFile(from, dest)
	if err != nil || c == nil {
		return imageHeader{}, err
	}
	return c.header, nil
}

// mediaFile returns dest parsed and the copy of the repository file that it
// names on pages[from], which it reads the first time. The copy is nil for a
// URL with a scheme or host, which pages keep as it is.
func (s *site) mediaFile(from int, dest string) (*url.URL, *mediaCopy, error) {
	u, err := url.Parse(dest)
	if err != nil {
		return nil, nil, fmt.Errorf("media %q: %w", dest, err)
	}
	if u.Scheme != "" || u.Host != "" || u.Path == "" {
		return u, nil, nil
	}
	c, err := s.mediaCopyOf(repoPath(s.pages[from].file, u.Path), dest)
	if err != nil {
		return nil, nil, err
	}
	return u, c, nil
}

// mediaCopyOf returns the copy of repository-relative media file, named by
// dest, which it reads the first time.
func (s *site) mediaCopyOf(file, dest string) (*mediaCopy, error) {
	c, ok := s.media[file]
	if !ok {
		data, err := fs.ReadFile(s.fsys, file)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("media %q: no file %s", dest, file)
			}
			return nil, fmt.Errorf("media %q: %w", dest, err)
		}
		sum := sha256.Sum256(data)
		c = mediaCopy{
			name: path.Join(
				mediaDir,
				hex.EncodeToString(sum[:mediaHashSize]),
				path.Base(file),
			),
			data: data,
		}
		c.header, _ = readImageHeader(data)
		if s.media == nil {
			s.media = make(map[string]mediaCopy)
		}
		s.media[file] = c
	}
	return &c, nil
}

// opaqueAttr marks an image without transparent pixels,
// which hides the CSS placeholder behind it once it loads.
const opaqueAttr = "data-opaque"

// sizeImage sets the width, height and [opaqueAttr] of Markdown image img from h.
func sizeImage(img *ast.Image, h imageHeader) {
	if h.width == 0 {
		return
	}
	attr := func(key, val string) {
		img.SetAttribute(key, text.NewMultiLineValueFromString(val, text.IdentityDecoder))
	}
	attr("width", strconv.Itoa(h.width))
	attr("height", strconv.Itoa(h.height))
	if h.opaque {
		attr(opaqueAttr, "")
	}
}

// rewriteMedia returns the visible parts of raw HTML: media elements limited to
// [mediaAttrs], with src, srcset, and poster URLs replaced by resolve, the
// attributes that [sizeMedia] adds from header, and script-running URLs removed,
// plus anything c writes (nil means none). found reports whether it kept media
// or c output. [mediaURLEdits] handles page copies.
func rewriteMedia(
	raw string, c *collapsibles, resolve func(string) (string, error), header headerFunc,
) (out string, found bool, err error) {
	toks, err := htmlTokens(raw)
	if err != nil {
		return "", false, err
	}
	var b strings.Builder
	// pic is the <img> of the open <picture>.
	var pic *pictureImg
	for i, t := range toks {
		tt, tok := t.typ, t.Token
		tag := tt == html.StartTagToken || tt == html.SelfClosingTagToken ||
			tt == html.EndTagToken
		if !tag || !mediaElements[tok.Data] {
			if c != nil {
				wrote, err := c.write(&b, tt, tok)
				if err != nil {
					return "", false, err
				}
				found = found || wrote
			}
			continue
		}
		found = true
		if tt == html.EndTagToken {
			if tok.Data == "picture" {
				pic = nil
			}
			b.WriteString("</")
			b.WriteString(tok.Data)
			b.WriteByte('>')
			continue
		}
		if tok.Data == "picture" && tt == html.StartTagToken {
			if pic, err = findPictureImg(toks[i+1:], header); err != nil {
				return "", false, err
			}
		}
		var attrs []html.Attribute
		for _, a := range tok.Attr {
			if a.Key == "src" || a.Key == "srcset" || a.Key == "poster" {
				v, err := resolveMediaAttr(a.Key, a.Val, resolve)
				if err != nil {
					return "", false, fmt.Errorf("<%s>: %w", tok.Data, err)
				}
				a.Val = v
				if goldhtml.IsDangerousURL(v) {
					continue
				}
			}
			if !mediaAttrs[a.Key] {
				continue
			}
			attrs = append(attrs, a)
		}
		if attrs, err = sizeMedia(tok, attrs, pic, header); err != nil {
			return "", false, fmt.Errorf("<%s>: %w", tok.Data, err)
		}
		b.WriteByte('<')
		b.WriteString(tok.Data)
		for _, a := range attrs {
			b.WriteByte(' ')
			b.WriteString(a.Key)
			if a.Val != "" {
				b.WriteString(`="`)
				b.WriteString(html.EscapeString(a.Val))
				b.WriteByte('"')
			}
		}
		b.WriteByte('>')
	}
	return b.String(), found, nil
}

// headerFunc returns the header of the image file at URL dest as a page writes it,
// or zero when unknown.
type headerFunc func(dest string) (imageHeader, error)

// htmlToken is a token of raw HTML and its type.
type htmlToken struct {
	typ html.TokenType
	html.Token
}

// htmlTokens tokenizes raw HTML up front, for [findPictureImg] to look ahead.
func htmlTokens(raw string) ([]htmlToken, error) {
	z := html.NewTokenizer(strings.NewReader(raw))
	var toks []htmlToken
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				return toks, nil
			}
			return nil, z.Err()
		}
		toks = append(toks, htmlToken{typ: tt, Token: z.Token()})
	}
}

// pictureImg is the <img> of a <picture>, which its <source> elements size by.
type pictureImg struct {
	w, h   int  // The size of its image, 0 when unknown.
	aw, ah int  // Its width and height attributes, 0 when absent.
	ok     bool // Whether the attributes are integers.
	opaque bool // Whether every image of the picture is opaque.
}

// findPictureImg returns the <img> in toks, the tokens after a <picture>,
// or nil when the picture or the raw HTML ends first.
func findPictureImg(toks []htmlToken, header headerFunc) (*pictureImg, error) {
	opaque := true
	for _, t := range toks {
		if t.typ == html.EndTagToken {
			if t.Data == "picture" {
				return nil, nil
			}
			continue
		}
		switch t.Data {
		case "source":
			o, err := srcsetOpaque(attrOf(t.Attr, "srcset"), header)
			if err != nil {
				return nil, fmt.Errorf("<source>: %w", err)
			}
			opaque = opaque && o
		case "img":
			img, o, err := imgImage(t.Attr, header)
			if err != nil {
				return nil, fmt.Errorf("<img>: %w", err)
			}
			p := pictureImg{w: img.width, h: img.height, opaque: opaque && o}
			p.aw, p.ah, p.ok = dimensionAttrs(t.Attr)
			return &p, nil
		}
	}
	return nil, nil
}

// sizeMedia adds the width and height of tok's image to attrs, its kept
// attributes, from the src of an <img> or the poster of a <video>.
// An author's width or height scales the other, and both stay as written.
// A <source> of pic's <picture> gets a size only if its proportions
// differ from the <img>. An <img> with only opaque images gets [opaqueAttr].
func sizeMedia(
	tok html.Token, attrs []html.Attribute, pic *pictureImg, header headerFunc,
) ([]html.Attribute, error) {
	aw, ah, ok := dimensionAttrs(attrs)
	var img imageHeader
	var opaque bool
	var err error
	switch tok.Data {
	case "img":
		img, opaque, err = imgImage(tok.Attr, header)
		if pic != nil {
			opaque = pic.opaque
		}
	case "video":
		img, err = header(attrOf(tok.Attr, "poster"))
	case "source":
		if pic == nil || !pic.ok {
			return attrs, nil
		}
		img, err = srcsetImage(attrOf(tok.Attr, "srcset"), header)
		if aw == 0 && ah == 0 {
			if pic.aw > 0 && pic.ah > 0 ||
				sameRatio(img.width, img.height, pic.w, pic.h) {
				return attrs, nil
			}
			aw, ah = pic.aw, pic.ah
		}
	}
	if err != nil {
		return nil, err
	}
	if ok && img.width > 0 {
		w, h := fitSize(img.width, img.height, aw, ah)
		attrs = setAttr(attrs, "width", strconv.Itoa(w))
		attrs = setAttr(attrs, "height", strconv.Itoa(h))
	}
	if opaque {
		attrs = append(attrs, html.Attribute{Key: opaqueAttr})
	}
	return attrs, nil
}

// imgImage returns the header of the src of an <img>, or of its first srcset
// candidate without one. opaque reports whether all its images are opaque.
func imgImage(
	attrs []html.Attribute, header headerFunc,
) (img imageHeader, opaque bool, err error) {
	srcset := attrOf(attrs, "srcset")
	if src := attrOf(attrs, "src"); src != "" {
		img, err = header(src)
	} else {
		img, err = srcsetImage(srcset, header)
	}
	if err != nil {
		return imageHeader{}, false, err
	}
	opaque, err = srcsetOpaque(srcset, header)
	return img, opaque && img.opaque, err
}

// srcsetImage returns the header of the first candidate of srcset,
// its size divided by a density such as "2x".
func srcsetImage(srcset string, header headerFunc) (imageHeader, error) {
	first, _, _ := strings.Cut(srcset, ",")
	fields := strings.Fields(first)
	if len(fields) == 0 {
		return imageHeader{}, nil
	}
	img, err := header(fields[0])
	if err != nil || img.width == 0 || len(fields) < 2 {
		return img, err
	}
	if x, ok := strings.CutSuffix(fields[1], "x"); ok {
		if d, err := strconv.ParseFloat(x, 64); err == nil && d > 0 {
			img.width = max(1, round(float64(img.width)/d))
			img.height = max(1, round(float64(img.height)/d))
		}
	}
	return img, nil
}

// srcsetOpaque reports whether every candidate of srcset is an opaque image.
func srcsetOpaque(srcset string, header headerFunc) (bool, error) {
	for c := range strings.SplitSeq(srcset, ",") {
		fields := strings.Fields(c)
		if len(fields) == 0 {
			continue
		}
		img, err := header(fields[0])
		if err != nil || !img.opaque {
			return false, err
		}
	}
	return true, nil
}

// dimensionAttrs returns the width and height of attrs, 0 when absent.
// ok is false for a value that isn't a positive integer.
func dimensionAttrs(attrs []html.Attribute) (w, h int, ok bool) {
	for _, a := range attrs {
		if a.Key != "width" && a.Key != "height" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(a.Val))
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if a.Key == "width" {
			w = n
		} else {
			h = n
		}
	}
	return w, h, true
}

// fitSize returns the size of an element that shows a w by h image.
// The author's width aw or height ah, 0 when absent, scales the other.
func fitSize(w, h, aw, ah int) (int, int) {
	switch {
	case aw > 0 && ah > 0:
		return aw, ah
	case aw > 0:
		return aw, max(1, round(float64(aw)*float64(h)/float64(w)))
	case ah > 0:
		return max(1, round(float64(ah)*float64(w)/float64(h))), ah
	}
	return w, h
}

// sameRatio reports whether w1/h1 and w2/h2 match within the 1% that rounding
// leaves in a resized image. A width of 0 matches nothing.
func sameRatio(w1, h1, w2, h2 int) bool {
	if w1 == 0 || w2 == 0 {
		return false
	}
	a, b := float64(w1)*float64(h2), float64(w2)*float64(h1)
	return math.Abs(a-b) <= 0.01*max(a, b)
}

func round(f float64) int {
	return int(math.Round(f))
}

// attrOf returns the value of attribute key in attrs, or "".
func attrOf(attrs []html.Attribute, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// setAttr sets attribute key of attrs to val, adding it at the end if absent.
func setAttr(attrs []html.Attribute, key, val string) []html.Attribute {
	for i := range attrs {
		if attrs[i].Key == key {
			attrs[i].Val = val
			return attrs
		}
	}
	return append(attrs, html.Attribute{Key: key, Val: val})
}

// resolveMediaAttr replaces each URL in attribute key's val with resolve's result.
// A srcset contains URLs with optional widths or densities.
func resolveMediaAttr(
	key, val string, resolve func(string) (string, error),
) (string, error) {
	if key != "srcset" {
		return resolve(strings.TrimSpace(val))
	}
	var r []string
	for c := range strings.SplitSeq(val, ",") {
		fields := strings.Fields(c)
		if len(fields) == 0 {
			continue
		}
		u, err := resolve(fields[0])
		if err != nil {
			return "", err
		}
		r = append(r, strings.Join(append([]string{u}, fields[1:]...), " "))
	}
	return strings.Join(r, ", "), nil
}

// rawHTML decorates block or inline raw-HTML rendering. It shows media with
// URLs replaced by resolve, attributes from header and <details> written by c.
// The decorated renderer handles other raw HTML, which goldmark omits.
func rawHTML(
	c *collapsibles, resolve func(string) (string, error), header headerFunc,
) goldhtml.NodeRendererDecorator {
	return func(next goldhtml.NodeRenderer) goldhtml.NodeRenderer {
		return renderer.NodeRendererFunc(func(
			w io.Writer, source []byte, node ast.Node, entering bool, rc renderer.Context,
		) (ast.WalkStatus, error) {
			if !entering {
				return next.Render(w, source, node, entering, rc)
			}
			var raw string
			block := false
			switch n := node.(type) {
			case *ast.RawHTML:
				raw = n.Value.Value(source)
			case *ast.HTMLBlock:
				raw, block = string(n.Value.Bytes(source)), true
			}
			out, found, err := rewriteMedia(raw, c, resolve, header)
			if err != nil {
				return ast.WalkStop, err
			}
			if !found {
				return next.Render(w, source, node, entering, rc)
			}
			if block {
				out += "\n"
			}
			_, _ = io.WriteString(w, out)
			return ast.WalkSkipChildren, nil
		})
	}
}

// rawHTMLEdits returns src edits replacing media URLs in parsed raw-HTML nodes
// with resolve's results. It reads each node from first through last line,
// treating omitted inter-line text—list indentation, blockquote ">", or
// paragraph indentation—as spaces. This keeps split tags whole.
func rawHTMLEdits(
	src []byte, raws []ast.Node, resolve func(string) (string, error),
) ([]edit, error) {
	var edits []edit
	for _, n := range raws {
		var lines []text.Index
		switch n := n.(type) {
		case *ast.RawHTML:
			if !n.Value.IsOwned() {
				lines = n.Value.Indices()
			}
		case *ast.HTMLBlock:
			if !n.Value.IsOwned() {
				for _, seg := range n.Value.Segments() {
					lines = append(lines, text.Index{Start: seg.Start, Stop: seg.Stop})
				}
			}
		}
		if len(lines) == 0 {
			continue
		}
		// raw offsets equal src offsets minus start.
		start := lines[0].Start
		raw := slices.Clone(src[start:lines[len(lines)-1].Stop])
		for i := 1; i < len(lines); i++ {
			for p := lines[i-1].Stop; p < lines[i].Start; p++ {
				if raw[p-start] != '\n' {
					raw[p-start] = ' '
				}
			}
		}
		urls, err := mediaURLEdits(string(raw), resolve)
		if err != nil {
			return nil, err
		}
		for _, e := range urls {
			e.Start += start
			e.Stop += start
			edits = append(edits, e)
		}
	}
	return edits, nil
}

// mediaURLEdits returns raw-HTML edits replacing media src, srcset, and poster
// URLs with resolve's results, as shown in a page's Markdown copy. Only values
// change, preserving all other raw text, including a trailing partial tag.
// Unquoted values gain quotes.
func mediaURLEdits(raw string, resolve func(string) (string, error)) ([]edit, error) {
	z := html.NewTokenizer(strings.NewReader(raw))
	var edits []edit
	// at is the token's raw offset.
	for at := 0; ; {
		tt := z.Next()
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				return edits, nil
			}
			return nil, z.Err()
		}
		source := string(z.Raw())
		tok := z.Token()
		if (tt == html.StartTagToken || tt == html.SelfClosingTagToken) &&
			mediaElements[tok.Data] {
			vals := attrValues(source)
			if !slices.EqualFunc(vals, tok.Attr,
				func(v attrValue, a html.Attribute) bool {
					return v.key == a.Key
				}) {
				return nil, fmt.Errorf("<%s>: docpages can't find the attributes of %q",
					tok.Data, source)
			}
			for i, a := range tok.Attr {
				if a.Key != "src" && a.Key != "srcset" && a.Key != "poster" {
					continue
				}
				v := vals[i]
				// A valueless attribute names and keeps no file.
				if v.start == v.end && !v.quoted {
					continue
				}
				val, err := resolveMediaAttr(a.Key, a.Val, resolve)
				if err != nil {
					return nil, fmt.Errorf("<%s>: %w", tok.Data, err)
				}
				if val == a.Val {
					continue
				}
				val = html.EscapeString(val)
				if !v.quoted {
					val = `"` + val + `"`
				}
				edits = append(edits, edit{
					Index: text.Index{Start: at + v.start, Stop: at + v.end},
					text:  val,
				})
			}
		}
		at += len(source)
	}
}

// attrValue locates an attribute value in its raw tag.
type attrValue struct {
	key        string // Lower-cased, as [html.Token] holds it.
	start, end int    // The value, inside its quotes if it has them.
	quoted     bool
}

// attrValues locates attribute values in raw start tag tag, in [html.Token] order.
// It follows golang.org/x/net/html tokenization, omitting keyless
// attributes and duplicates after the first key.
func attrValues(tag string) []attrValue {
	space := func(i int) bool {
		switch tag[i] {
		case ' ', '\n', '\r', '\t', '\f':
			return true
		}
		return false
	}
	skip := func(i int) int {
		for i < len(tag) && space(i) {
			i++
		}
		return i
	}
	var vals []attrValue
	seen := make(map[string]bool)
	// Name after "<".
	i := 1
	for i < len(tag) && !space(i) && tag[i] != '/' && tag[i] != '>' {
		i++
	}
	for i = skip(i); i < len(tag) && tag[i] != '>'; i = skip(i) {
		// Key, including a leading "=".
		k := i
		for i < len(tag) && !space(i) && tag[i] != '/' && tag[i] != '>' &&
			(tag[i] != '=' || i == k) {
			i++
		}
		key := lowerASCII(tag[k:i])
		v := attrValue{start: i, end: i}
		// Value after "=" and surrounding spaces.
		switch j := skip(i); {
		case j < len(tag) && tag[j] == '/':
			i = j + 1
		case j == len(tag) || tag[j] != '=':
			i = j
		default:
			j = skip(j + 1)
			switch {
			case j == len(tag) || tag[j] == '>':
				i = j
			case tag[j] == '"' || tag[j] == '\'':
				v.start, v.quoted = j+1, true
				if e := strings.IndexByte(tag[j+1:], tag[j]); e >= 0 {
					v.end, i = j+1+e, j+2+e
				} else {
					v.end, i = len(tag), len(tag)
				}
			default:
				v.start = j
				for j++; j < len(tag) && !space(j) && tag[j] != '>'; j++ {
				}
				v.end, i = j, j
			}
		}
		if key != "" && !seen[key] {
			seen[key] = true
			v.key = strings.ReplaceAll(key, "\x00", "\ufffd")
			vals = append(vals, v)
		}
	}
	return vals
}

// lowerASCII lowercases s's ASCII letters, matching HTML key tokenization.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}
