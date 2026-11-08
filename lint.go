package docpages

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v3/lexers"
	"github.com/yuin/goldmark/v2/ast"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
	"golang.org/x/net/html"
)

// lintError is a mistake in the Markdown of a page at the byte offset pos.
type lintError struct {
	pos int
	msg string
}

// mediaElementList names [mediaElements] for the errors.
const mediaElementList = "<img>, <picture>, <source>, <video>, <audio> and <track>"

// chromaLanguages is where Chroma lists the languages that it highlights.
const chromaLanguages = "https://github.com/alecthomas/chroma#supported-languages"

var (
	// tableRow matches a line of a paragraph that starts as a row of a table.
	tableRow = regexp.MustCompile(`^\s*\|`)
	// unspacedItem matches paragraph lines beginning with an unspaced list
	// marker, such as -item or 1.item. It omits *, which often begins emphasis.
	unspacedItem = regexp.MustCompile(`^\s*([-+]|[0-9]{1,9}[.)])\pL`)
	// versionX matches versions such as 2.x that [unspacedItem] would mistake for items.
	versionX = regexp.MustCompile(`^\s*[0-9]+\.[xX]\b`)
	// orderedItem matches a paragraph line beginning an ordered-list item.
	// Four-plus digits, such as the year in "2019. It grew.", begin a sentence.
	orderedItem = regexp.MustCompile(`^\s*([0-9]{1,3})[.)][ \t]+\S`)
)

// lintMarkdown returns src/tree n differences from GitHub rendering or content
// the site would omit:
//
//   - Code blocks in languages Chroma cannot highlight, which stay plain.
//   - Raw HTML except [mediaElements] with [mediaAttrs] and <details> output
//     from [collapsibles]. Comments are valid because GitHub also hides them.
//   - Lines beginning with "|" but lacking a table delimiter row, which stay text.
//   - List items without marker spacing, such as -item, and ordered lists
//     starting above 1 directly below a paragraph, which CommonMark folds into it.
func lintMarkdown(n ast.Node, src []byte) []lintError {
	var errs []lintError
	// Read raw HTML in render order through one [collapsibles], whose <details>
	// span nodes. Stop on malformed order, which rendering also reports.
	c := &collapsibles{}
	ordered := true
	readHTML := func(raw string, pos int) {
		if ordered {
			var e []lintError
			e, ordered = lintHTML(c, raw, pos)
			errs = append(errs, e...)
		}
	}
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.CodeBlock:
			lang, _ := n.Language(src)
			if lang != "" && lang != "mermaid" && lexers.Get(lang) == nil {
				pos := n.Pos()
				if !n.Info.IsOwned() && !n.Info.IsEmpty() {
					pos = n.Info.Index().Start
				}
				errs = append(errs, lintError{pos, fmt.Sprintf(
					"code block in %q, a language that Chroma doesn't highlight: see %s",
					lang, chromaLanguages)})
			}
		case *ast.RawHTML:
			if !n.Value.IsOwned() {
				readHTML(n.Value.Value(src), n.Pos())
			}
		case *ast.HTMLBlock:
			if segs := n.Value.Segments(); len(segs) > 0 {
				readHTML(string(n.Value.Bytes(src)), segs[0].Start)
			}
		case *ast.Paragraph:
			errs = append(errs, lintParagraph(n, src)...)
		}
		return ast.WalkContinue, nil
	})
	return errs
}

// shownElements names what a page shows of raw HTML, for the errors.
const shownElements = mediaElementList + ", and <details> with its <summary>"

// lintHTML returns mistakes in raw HTML at byte offset pos as c renders it,
// plus whether <details> are ordered.
func lintHTML(c *collapsibles, raw string, pos int) (errs []lintError, ordered bool) {
	report := func(format string, args ...any) {
		errs = append(errs, lintError{pos, fmt.Sprintf(format, args...)})
	}
	z := html.NewTokenizer(strings.NewReader(raw))
	var scratch strings.Builder
	// Do not also report text inside an already reported element such as <script>.
	left := false
	for {
		tt := z.Next()
		tok := z.Token()
		tag := tt == html.StartTagToken || tt == html.SelfClosingTagToken
		switch {
		case tt == html.ErrorToken:
			if !errors.Is(z.Err(), io.EOF) {
				report("raw HTML: %v", z.Err())
			}
			return errs, true
		case tt == html.CommentToken:
		case tt == html.DoctypeToken:
			report("raw HTML <!DOCTYPE>, which a page leaves out")
		case (tag || tt == html.EndTagToken) && mediaElements[tok.Data]:
			for _, a := range tok.Attr {
				switch {
				case !mediaAttrs[a.Key]:
					report(
						"attribute %s of <%s>, which a page leaves out",
						a.Key, tok.Data,
					)
				case (a.Key == "src" || a.Key == "poster") &&
					goldhtml.IsDangerousURL(a.Val):
					report(
						"%s %q of <%s>, a URL that a page leaves out",
						a.Key, a.Val, tok.Data,
					)
				}
			}
		default:
			skipped := c.skip != ""
			wrote, err := c.write(&scratch, tt, tok)
			switch {
			case err != nil:
				return errs, false
			case !wrote && tag && !skipped:
				report("raw HTML <%s>, which a page leaves out: it shows %s",
					tok.Data, shownElements)
				left = true
			case !wrote && tt == html.TextToken && !skipped && !left &&
				strings.TrimSpace(tok.Data) != "":
				report("text %q of raw HTML, which a page leaves out: it shows %s",
					excerpt(tok.Data), shownElements)
			case wrote && tag:
				// [collapsibles] keeps only a <details> open attribute.
				for _, a := range tok.Attr {
					if tok.Data != "details" || a.Key != "open" {
						report(
							"attribute %s of <%s>, which a page leaves out",
							a.Key, tok.Data,
						)
					}
				}
			}
			// A non-rendered end tag closes an element reported at its start.
		}
	}
}

// lintParagraph returns the mistakes in the lines of the paragraph p of src.
func lintParagraph(p *ast.Paragraph, src []byte) []lintError {
	var errs []lintError
	segs := p.Source()
	rows := 0
	for i, seg := range segs {
		line := strings.TrimRight(string(src[seg.Start:seg.Stop]), "\r\n")
		if tableRow.MatchString(line) {
			rows++
		}
		if m := unspacedItem.FindStringSubmatch(line); m != nil &&
			!versionX.MatchString(line) {
			errs = append(errs, lintError{seg.Start, fmt.Sprintf(
				"%q starts as a list item without the space after %q,"+
					" which makes it text",
				excerpt(line), m[1])})
		}
		if m := orderedItem.FindStringSubmatch(line); m != nil && i > 0 {
			if n, _ := strconv.Atoi(m[1]); n != 1 {
				errs = append(errs, lintError{seg.Start, fmt.Sprintf(
					"%q starts a list at %d right below a paragraph, "+
						"which makes it text: put a blank line above it",
					excerpt(line), n)})
			}
		}
	}
	if rows >= 2 && tableRow.Match(src[segs[0].Start:segs[0].Stop]) {
		errs = append(errs, lintError{
			segs[0].Start,
			`lines that start with "|" but make no table, which needs a delimiter ` +
				`row below its first row, as | --- | --- |, with as many cells`,
		})
	}
	return errs
}

// excerpt trims line and shortens it to 40 bytes.
func excerpt(line string) string {
	line = strings.TrimSpace(line)
	if len(line) > 40 {
		return line[:40] + "..."
	}
	return line
}

// lintErrors formats src mistakes as file:line: mistake. src omits front matter,
// but line numbers include it when fsys contains file.
func lintErrors(fsys fs.FS, file string, src []byte, errs []lintError) error {
	offset := 0
	if fsys != nil {
		if data, err := fs.ReadFile(fsys, file); err == nil {
			offset = bytes.Count(data, []byte("\n")) - bytes.Count(src, []byte("\n"))
		}
	}
	r := make([]error, len(errs))
	for i, e := range errs {
		r[i] = fmt.Errorf("%s:%d: %s", file, offset+lineOf(src, e.pos), e.msg)
	}
	return errors.Join(r...)
}

// lineOf returns the 1-based src line containing byte offset pos.
func lineOf(src []byte, pos int) int {
	if pos < 0 || pos > len(src) {
		return 0
	}
	return bytes.Count(src[:pos], []byte("\n")) + 1
}

// settingFiles returns files used by settings c in docs directory dir:
// logo, favicon, style.css, h's configured host icon, and templates.
// Go and text files written beside a .templ file by templ generate also count,
// though the site builds the templates itself.
func settingFiles(
	dir string, c chrome, h *hostSettings, templates []templateFile,
) []string {
	var r []string
	if c.logo != "" {
		r = append(r, path.Join(dir, "logo.svg"))
	}
	if c.favicon != "" {
		r = append(r, path.Join(dir, c.favicon))
	}
	if c.style {
		r = append(r, path.Join(dir, "style.css"))
	}
	for _, t := range templates {
		base := strings.TrimSuffix(t.name, ".templ")
		r = append(r, t.name, base+"_templ.go", base+"_templ.txt")
	}
	if h != nil && h.Icon != "" {
		r = append(r, path.Join(dir, h.Icon))
	}
	return r
}

// unusedFiles reports every edition docs file in fsys unused by s: not shown by
// a page, named by a link, or used by settings. Examples are an obsolete image
// or Markdown in an unpositioned directory omitted from navigation.
// For root-level docs, it checks only groups because the rest is repository content.
func unusedFiles(fsys fs.FS, s *Site, settings []string) error {
	used := make(map[string]bool)
	for _, f := range settings {
		used[f] = true
	}
	var linked []string
	for _, e := range s.editions {
		for _, p := range e.pages {
			used[p.file] = true
			if p.stub != "" {
				used[p.stub] = true
			}
		}
		for f := range e.links.media {
			used[f] = true
		}
		for f := range e.links.linked {
			linked = append(linked, f)
		}
	}
	var errs []error
	for _, e := range s.editions {
		err := fs.WalkDir(fsys, e.dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || p == e.dir {
				return err
			}
			rel := p
			if e.dir != "." {
				rel = strings.TrimPrefix(p, e.dir+"/")
			}
			top := !strings.Contains(rel, "/")
			switch {
			case strings.HasPrefix(d.Name(), "."),
				e.isMain() && rel == versionsDir,
				e.dir == "." && top && (!d.IsDir() || !orderPrefix.MatchString(rel)):
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			case d.IsDir(), used[p]:
				return nil
			// A group icon belongs to its directory.
			case d.Name() == "icon.svg" && path.Dir(rel) != "." &&
				!strings.Contains(path.Dir(rel), "/") &&
				orderPrefix.MatchString(path.Dir(rel)):
				return nil
			case slices.ContainsFunc(linked, func(l string) bool { return inDir(p, l) }):
				return nil
			}
			errs = append(errs, fmt.Errorf(
				"%s: no page shows this file, and no link or setting names it", p))
			return nil
		})
		if err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}
