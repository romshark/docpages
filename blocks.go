package docpages

import (
	"html"
	"io"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/renderer"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

// headingLinks decorates heading renderer next, ending each heading below level 1
// with a shareable self-link.
func headingLinks(next goldhtml.NodeRenderer) goldhtml.NodeRenderer {
	return renderer.NodeRendererFunc(func(
		w io.Writer, source []byte, node ast.Node, entering bool, rc renderer.Context,
	) (ast.WalkStatus, error) {
		h := node.(*ast.Heading)
		if id, ok := h.Attribute("id"); ok && !entering && h.Level > 1 {
			href := "#" + html.EscapeString(id.Value(source))
			_, _ = io.WriteString(w, `<a class="heading-link" href="`+href+
				`" aria-label="Link to this section"></a>`)
		}
		return next.Render(w, source, node, entering, rc)
	})
}

// calloutAttr marks blockquotes converted by [markCallouts];
// its value is a kind such as NOTE.
const calloutAttr = "data-callout"

// calloutTitles maps GitHub alert kinds to displayed titles.
var calloutTitles = map[string]string{
	"NOTE":      "Note",
	"TIP":       "Tip",
	"IMPORTANT": "Important",
	"WARNING":   "Warning",
	"CAUTION":   "Caution",
}

// calloutMarker matches a GitHub-alert blockquote's first line, such as "> [!NOTE]".
var calloutMarker = regexp.MustCompile(`^\[!([A-Z]+)\]\s*$`)

// markCallouts converts GitHub-alert blockquotes in doc,
// removing marker lines and setting [calloutAttr].
func markCallouts(doc ast.Node, src []byte) {
	var quotes []*ast.Blockquote
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if q, ok := n.(*ast.Blockquote); ok && entering {
			quotes = append(quotes, q)
		}
		return ast.WalkContinue, nil
	})
	for _, q := range quotes {
		p, ok := q.FirstChild().(*ast.Paragraph)
		if !ok || len(p.Source()) == 0 {
			continue
		}
		m := calloutMarker.FindSubmatch(p.Source()[0].Bytes(src))
		if m == nil || calloutTitles[string(m[1])] == "" {
			continue
		}
		// The marker spans through the first line's end.
		for c := p.FirstChild(); c != nil; {
			next := c.NextSibling()
			p.RemoveChild(c)
			if t, ok := c.(*ast.Text); ok && (t.SoftLineBreak() || t.HardLineBreak()) {
				break
			}
			c = next
		}
		if p.ChildCount() == 0 {
			q.RemoveChild(p)
		}
		q.SetAttribute(calloutAttr,
			text.NewMultiLineValueFromString(string(m[1]), text.IdentityDecoder))
	}
}

// callouts decorates blockquote renderer next, rendering [markCallouts] callouts.
func callouts(next goldhtml.NodeRenderer) goldhtml.NodeRenderer {
	return renderer.NodeRendererFunc(func(
		w io.Writer, source []byte, node ast.Node, entering bool, rc renderer.Context,
	) (ast.WalkStatus, error) {
		attr, ok := node.Attribute(calloutAttr)
		if !ok {
			return next.Render(w, source, node, entering, rc)
		}
		if !entering {
			_, _ = io.WriteString(w, "</div>\n")
			return ast.WalkContinue, nil
		}
		kind := attr.Value(source)
		_, _ = io.WriteString(w, `<div class="callout callout-`+strings.ToLower(kind)+
			`" role="note"><p class="callout-title">`+calloutTitles[kind]+"</p>\n")
		return ast.WalkContinue, nil
	})
}
