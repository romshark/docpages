package docpages

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/v2/ast"
)

// searchEntry is a page section searched by docs.ts from search.json.
// Short names keep the file small.
type searchEntry struct {
	Page    string `json:"p"` // Title of the page.
	Section string `json:"s"` // Heading of the section, empty above the first.
	URL     string `json:"u"` // Relative to the root of the version.
	Text    string `json:"t"`
}

// searchTextLimit bounds [searchEntry] text in bytes. Specification sections
// can exceed 1,000 words but remain findable by heading and opening lines.
const searchTextLimit = 1500

// searchEntries returns d's entries from document node. The first holds text
// above the first subheading; every heading listed in contents starts another.
// Code blocks are omitted.
func searchEntries(d doc, node ast.Node, src []byte) []searchEntry {
	entries := []searchEntry{{Page: d.page.title, URL: d.page.path}}
	var text strings.Builder
	flush := func() {
		e := &entries[len(entries)-1]
		words := strings.Fields(text.String())
		e.Text = truncate(strings.Join(words, " "), searchTextLimit)
		text.Reset()
	}
	for n := node.FirstChild(); n != nil; n = n.NextSibling() {
		switch n := n.(type) {
		case *ast.Heading:
			if n.Level == 1 || n.Level > d.depth {
				writePlainText(&text, n, src)
				text.WriteString(" ")
				continue
			}
			flush()
			id, _ := n.Attribute("id")
			entries = append(entries, searchEntry{
				Page:    d.page.title,
				Section: plainText(n, src),
				URL:     d.page.path + "#" + id.Value(src),
			})
		case *ast.CodeBlock, *ast.HTMLBlock, *ast.ThematicBreak:
		default:
			writePlainText(&text, n, src)
			text.WriteString(" ")
		}
	}
	flush()
	return entries
}

// truncate returns s cut at the last space before n bytes, or unchanged if shorter.
// With no space, it cuts before the character containing byte n, as in CJK.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if i := strings.LastIndexByte(s[:n], ' '); i > 0 {
		return s[:i]
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// searchJSON returns search.json, the entries of all docs.
func searchJSON(docs []doc) ([]byte, error) {
	entries := []searchEntry{}
	for _, d := range docs {
		entries = append(entries, d.search...)
	}
	return json.Marshal(entries)
}
