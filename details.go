package docpages

import (
	"context"
	"errors"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// summaryElements are attribute-free HTML elements preserved around <summary> text.
var summaryElements = map[string]bool{
	"code": true, "b": true, "strong": true, "i": true, "em": true,
}

// detailsIcon returns a <summary>'s leading icon, which CSS rotates on open.
// It renders once after assets.go initializes icons for <neo-icon> to inline.
var detailsIcon = sync.OnceValue(func() string {
	var b strings.Builder
	_ = icon("chevron-down").Render(context.Background(), &b)
	return b.String()
})

// rawTextElements contain code or markup rather than text; <details> omits them and it.
var rawTextElements = map[string]bool{
	"iframe": true, "noembed": true, "noframes": true, "noscript": true,
	"plaintext": true, "script": true, "style": true, "textarea": true,
	"title": true, "xmp": true,
}

// collapsibles renders page <details> like GitHub: a <summary> and hidden text.
// One element's tags span raw-HTML nodes around Markdown rendered between them.
// Morpheus <neo-elastic> wraps the text to animate its height. Closed details
// start at height 0 with open="false", avoiding measurement of browser-hidden text.
type collapsibles struct {
	// outer lists surrounding <details>, innermost last.
	outer   []openDetails
	summary bool // Whether the node is in a <summary>.
	// skip is the containing [rawTextElements] element whose text is omitted;
	// empty outside one.
	skip string
}

// write emits to b the visible part of non-media raw-HTML token tok of type tt:
// <details>/<summary> tags, [summaryElements] content, and text within either.
// It omits everything else and reports whether it wrote anything.
func (c *collapsibles) write(
	b *strings.Builder, tt html.TokenType, tok html.Token,
) (bool, error) {
	start := tt == html.StartTagToken || tt == html.SelfClosingTagToken
	switch {
	case c.skip != "":
		if tt == html.EndTagToken && tok.Data == c.skip {
			c.skip = ""
		}
		return false, nil
	case tt == html.TextToken:
		// Spaces between <summary> tags are text too.
		if !c.summary && (!c.inBody() || strings.TrimSpace(tok.Data) == "") {
			return false, nil
		}
		b.WriteString(html.EscapeString(tok.Data))
		return true, nil
	case tt == html.StartTagToken && rawTextElements[tok.Data]:
		c.skip = tok.Data
		return false, nil
	}
	switch {
	case tok.Data == "details" && start:
		if c.summary {
			return false, errors.New("<details> inside a <summary>")
		}
		open := hasAttr(tok, "open")
		c.outer = append(c.outer, openDetails{open: open})
		b.WriteString(`<details class="details"`)
		if open {
			b.WriteString(" open")
		}
		b.WriteByte('>')
	case tok.Data == "details" && tt == html.EndTagToken:
		if len(c.outer) == 0 {
			return false, errors.New("</details> closes no <details>")
		}
		// Close malformed <details> so later page content needs no extra end tag.
		body, summary := c.inBody(), c.summary
		c.outer, c.summary = c.outer[:len(c.outer)-1], false
		switch {
		case summary:
			return false, errors.New("<summary> without </summary>")
		case !body:
			return false, errors.New("<details> has no <summary>")
		}
		b.WriteString("</neo-elastic></details>")
	case tok.Data == "summary" && start:
		if len(c.outer) == 0 || c.inBody() || c.summary {
			return false, errors.New("<summary> is not the first in a <details>")
		}
		c.summary = true
		b.WriteString("<summary>")
		b.WriteString(detailsIcon())
		b.WriteString(`<span class="details-title">`)
	case tok.Data == "summary" && tt == html.EndTagToken:
		if !c.summary {
			return false, errors.New("</summary> closes no <summary>")
		}
		c.summary = false
		d := &c.outer[len(c.outer)-1]
		d.body = true
		b.WriteString(`</span></summary><neo-elastic class="details-body"`)
		if !d.open {
			b.WriteString(` open="false"`)
		}
		b.WriteByte('>')
	case c.summary && summaryElements[tok.Data] && tt != html.SelfClosingTagToken:
		b.WriteByte('<')
		if tt == html.EndTagToken {
			b.WriteByte('/')
		}
		b.WriteString(tok.Data)
		b.WriteByte('>')
	default:
		return false, nil
	}
	return true, nil
}

// openDetails is a <details> around a node of raw HTML.
type openDetails struct {
	open bool // Whether it has the attribute open, which shows its text at first.
	body bool // Whether its <summary> has ended, which begins its text.
}

// inBody reports whether the node is in the text of a <details>.
func (c *collapsibles) inBody() bool {
	return len(c.outer) > 0 && c.outer[len(c.outer)-1].body
}

// done reports an error if a <details> of the page is still open at its end.
func (c *collapsibles) done() error {
	if len(c.outer) > 0 {
		return errors.New("<details> without </details>")
	}
	return nil
}

// hasAttr reports whether tok has the attribute key.
func hasAttr(tok html.Token, key string) bool {
	for _, a := range tok.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}
