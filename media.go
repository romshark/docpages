package docpages

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"slices"
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

// mediaCopy is the copy of a media file in the site.
type mediaCopy struct {
	name string // The path of the copy relative to the directory of the edition.
	data []byte
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
	u, err := url.Parse(dest)
	if err != nil {
		return "", fmt.Errorf("media %q: %w", dest, err)
	}
	if u.Scheme != "" || u.Host != "" || u.Path == "" {
		return dest, nil
	}
	file := repoPath(s.pages[from].file, u.Path)
	if s.root != "" && !inDir(file, s.root) {
		return "", fmt.Errorf("media %q is outside %s, "+
			"which a release holds its media in", dest, s.root)
	}
	c, ok := s.media[file]
	if !ok {
		data, err := fs.ReadFile(s.fsys, file)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("media %q: no file %s", dest, file)
			}
			return "", fmt.Errorf("media %q: %w", dest, err)
		}
		sum := sha256.Sum256(data)
		c = mediaCopy{
			name: path.Join(mediaDir, hex.EncodeToString(sum[:8]), path.Base(file)),
			data: data,
		}
		if s.media == nil {
			s.media = make(map[string]mediaCopy)
		}
		s.media[file] = c
	}
	return withQueryFragment(base+(&url.URL{Path: c.name}).EscapedPath(), u), nil
}

// rewriteMedia returns the visible parts of raw HTML: media elements limited to
// [mediaAttrs], with src, srcset, and poster URLs replaced by resolve and
// script-running URLs removed, plus anything c writes (nil means none). found
// reports whether it kept media or c output. [mediaURLEdits] handles page copies.
func rewriteMedia(
	raw string, c *collapsibles, resolve func(string) (string, error),
) (out string, found bool, err error) {
	z := html.NewTokenizer(strings.NewReader(raw))
	var b strings.Builder
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				return b.String(), found, nil
			}
			return "", false, z.Err()
		}
		tok := z.Token()
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
			b.WriteString("</")
			b.WriteString(tok.Data)
			b.WriteByte('>')
			continue
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
}

// resolveMediaAttr replaces each URL in attribute key's val with resolve's result.
// A srcset contains URLs with optional widths or densities.
func resolveMediaAttr(key, val string, resolve func(string) (string, error)) (string, error) {
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
// URLs replaced by resolve and <details> written by c. The decorated renderer
// handles other raw HTML, which goldmark omits.
func rawHTML(
	c *collapsibles, resolve func(string) (string, error),
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
			out, found, err := rewriteMedia(raw, c, resolve)
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
