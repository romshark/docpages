package docpages

import (
	"context"
	"errors"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// summaryElements are the HTML elements that a page keeps in the <summary> of
// a <details>, without their attributes, around the text that it keeps.
var summaryElements = map[string]bool{
	"code": true, "b": true, "strong": true, "i": true, "em": true,
}

// detailsIcon returns the markup of the icon at the start of the <summary> of a
// <details>, which CSS turns as the <details> opens. It renders the icon once,
// after the init of assets.go gives <neo-icon> the icons to inline.
var detailsIcon = sync.OnceValue(func() string {
	var b strings.Builder
	_ = icon("chevron-down").Render(context.Background(), &b)
	return b.String()
})

// rawTextElements are the HTML elements whose text is code or markup rather
// than text, which a <details> leaves out with them.
var rawTextElements = map[string]bool{
	"iframe": true, "noembed": true, "noframes": true, "noscript": true,
	"plaintext": true, "script": true, "style": true, "textarea": true,
	"title": true, "xmp": true,
}

// collapsibles writes the <details> of a page, a <summary> and the text that it
// hides, as GitHub shows them. The tags of 1 <details> lie in several nodes of
// raw HTML, around the Markdown of its text, which the page renders in between.
// A <neo-elastic> of Morpheus holds the text, which animates its height as it
// shows and hides. That of a closed <details> starts at the height 0, with
// open="false", which spares it measuring the text that the browser hides.
type collapsibles struct {
	// outer are the <details> around the node, the innermost last.
	outer   []openDetails
	summary bool // Whether the node is in a <summary>.
	// skip is the element of [rawTextElements] that the node is in, whose
	// text it leaves out. Empty outside one.
	skip string
}

// write writes to b what the page shows of tok, a token of raw HTML other than
// a media element, of type tt, and reports whether it wrote any: the tags of
// a <details> and of its <summary> with what the <summary> holds of [summaryElements],
// and the text in either. It leaves out the rest.
func (c *collapsibles) write(b *strings.Builder, tt html.TokenType, tok html.Token) (bool, error) {
	start := tt == html.StartTagToken || tt == html.SelfClosingTagToken
	switch {
	case c.skip != "":
		if tt == html.EndTagToken && tok.Data == c.skip {
			c.skip = ""
		}
		return false, nil
	case tt == html.TextToken:
		// The spaces between the tags of a <summary> are its text too.
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
		// The <details> closes even with a mistake, which leaves the rest of
		// the page to read without another for its end.
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
