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
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/renderer"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
	"golang.org/x/net/html"
)

// mediaDir is the directory of an edition that holds the copies of the media
// files that its pages show, each in a directory named after its content,
// which keeps 2 files of the same name apart.
const mediaDir = "media"

// mediaCopy is the copy of a media file in the site.
type mediaCopy struct {
	name string // The path of the copy relative to the directory of the edition.
	data []byte
}

// mediaElements are the HTML elements that a page shows of the raw HTML of its
// Markdown: images, videos and audio, with their sources and text tracks.
// goldmark leaves out the rest of the raw HTML.
var mediaElements = map[string]bool{
	"img":     true,
	"picture": true,
	"source":  true,
	"video":   true,
	"audio":   true,
	"track":   true,
}

// mediaAttrs are the attributes that the media elements keep in a page,
// which leaves out scripts, as onerror, and styles.
var mediaAttrs = map[string]bool{
	"alt": true, "title": true, "width": true, "height": true, "align": true,
	"loading": true, "decoding": true, "src": true, "srcset": true, "sizes": true,
	"media": true, "type": true, "poster": true, "controls": true, "autoplay": true,
	"loop": true, "muted": true, "playsinline": true, "preload": true, "kind": true,
	"srclang": true, "label": true, "default": true,
}

// mediaURL returns the URL on the site of the media file dest, as the file of
// pages[from] names it, where base is the URL of the root of the version from
// where the URL is read. A URL with a scheme or a host stays as it is. A path
// names a file of the repository relative to the file, as on GitHub, which
// the site holds a copy of in [mediaDir].
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

// rewriteMedia returns the raw HTML raw with each URL of the src, srcset and
// poster attributes of its media elements replaced by what resolve returns for
// it. With only, the result holds those elements alone, with the attributes of
// [mediaAttrs] and safe URLs, and what c writes of the rest, as a page shows
// them; c is nil for none. Otherwise it keeps the rest of raw as it is, as the
// Markdown copy of a page does. found reports whether the result holds a
// media element or what c wrote.
func rewriteMedia(
	raw string, only bool, c *collapsibles, resolve func(string) (string, error),
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
		source := string(z.Raw())
		tok := z.Token()
		tag := tt == html.StartTagToken || tt == html.SelfClosingTagToken ||
			tt == html.EndTagToken
		if !tag || !mediaElements[tok.Data] {
			if !only {
				b.WriteString(source)
			} else if c != nil {
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
			if only {
				b.WriteString("</")
				b.WriteString(tok.Data)
				b.WriteByte('>')
			} else {
				b.WriteString(source)
			}
			continue
		}
		changed := false
		var attrs []html.Attribute
		for _, a := range tok.Attr {
			if a.Key == "src" || a.Key == "srcset" || a.Key == "poster" {
				v, err := resolveMediaAttr(a.Key, a.Val, resolve)
				if err != nil {
					return "", false, fmt.Errorf("<%s>: %w", tok.Data, err)
				}
				changed = changed || v != a.Val
				a.Val = v
				if only && goldhtml.IsDangerousURL(v) {
					continue
				}
			}
			if only && !mediaAttrs[a.Key] {
				continue
			}
			attrs = append(attrs, a)
		}
		if !only && !changed {
			b.WriteString(source)
			continue
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

// resolveMediaAttr returns the value of the attribute key, val, with each URL
// in it replaced by what resolve returns for it. A srcset is a list of URLs,
// each with an optional width or density after it.
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

// rawHTML returns the decorator of the renderer of raw HTML, as an HTML block
// or an inline tag, that shows its media elements with the URLs of their files
// replaced by what resolve returns for each, and its <details>, which c
// writes. The renderer that it decorates renders raw HTML without either,
// which goldmark leaves out.
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
			out, found, err := rewriteMedia(raw, true, c, resolve)
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

// rawHTMLEdits returns the edits of the Markdown file src that replace each URL
// of the media elements of raws, the raw HTML parsed from src, by what resolve
// returns for it. The lines of an HTML block that follow each other in src are
// read together, which keeps a tag that spans lines whole.
func rawHTMLEdits(
	src []byte, raws []ast.Node, resolve func(string) (string, error),
) ([]edit, error) {
	var spans []text.Index
	for _, n := range raws {
		var indices []text.Index
		switch n := n.(type) {
		case *ast.RawHTML:
			if n.Value.IsOwned() {
				continue
			}
			indices = n.Value.Indices()
		case *ast.HTMLBlock:
			if n.Value.IsOwned() {
				continue
			}
			for _, seg := range n.Value.Segments() {
				indices = append(indices, text.Index{Start: seg.Start, Stop: seg.Stop})
			}
		}
		for _, i := range indices {
			if k := len(spans) - 1; k >= 0 && spans[k].Stop == i.Start {
				spans[k].Stop = i.Stop
				continue
			}
			spans = append(spans, i)
		}
	}
	var edits []edit
	for _, span := range spans {
		raw := string(src[span.Start:span.Stop])
		out, _, err := rewriteMedia(raw, false, nil, resolve)
		if err != nil {
			return nil, err
		}
		if out != raw {
			edits = append(edits, edit{Index: span, text: out})
		}
	}
	return edits, nil
}
