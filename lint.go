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
	// unspacedItem matches a line of a paragraph that starts as a list item
	// without the space after its marker, as -item or 1.item. A * is left
	// out, which starts emphasis as often.
	unspacedItem = regexp.MustCompile(`^\s*([-+]|[0-9]{1,9}[.)])\pL`)
	// versionX matches a line that starts with a version whose minor version is x,
	// as 2.x, which [unspacedItem] would take for an item.
	versionX = regexp.MustCompile(`^\s*[0-9]+\.[xX]\b`)
	// orderedItem matches a line of a paragraph that starts as an item of an
	// ordered list. A number of 4 digits or more, as the year in "2019.
	// It grew.", starts a sentence rather than a list.
	orderedItem = regexp.MustCompile(`^\s*([0-9]{1,3})[.)][ \t]+\S`)
)

// lintMarkdown returns the mistakes in the Markdown of the tree n of src that
// the site would show other than GitHub does, or not at all:
//
//   - A code block in a language that Chroma doesn't highlight, which would
//     stay plain.
//   - Raw HTML that the page would leave out: anything but the elements of
//     [mediaElements] with the attributes of [mediaAttrs] and what
//     [collapsibles] writes of a <details>. A comment is no mistake, which
//     GitHub doesn't show either.
//   - Lines that start with "|" without the delimiter row of a table, which
//     would be text.
//   - A list item without a space after its marker, as -item, and an ordered
//     list that starts at another number than 1 right below a paragraph, which
//     CommonMark reads as text of the paragraph.
func lintMarkdown(n ast.Node, src []byte) []lintError {
	var errs []lintError
	// The raw HTML of a page is read as the page renders it: in the order of
	// its nodes, through 1 [collapsibles], whose <details> span nodes. A
	// <details> out of order, which the rendering reports, ends the reading.
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

// lintHTML returns the mistakes in raw, raw HTML at the byte offset pos, which
// c reads as the page renders it, and whether its <details> are in order.
func lintHTML(c *collapsibles, raw string, pos int) (errs []lintError, ordered bool) {
	report := func(format string, args ...any) {
		errs = append(errs, lintError{pos, fmt.Sprintf(format, args...)})
	}
	z := html.NewTokenizer(strings.NewReader(raw))
	var scratch strings.Builder
	// The text of an element that is reported, as a <script>, is not.
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
					report("attribute %s of <%s>, which a page leaves out", a.Key, tok.Data)
				case (a.Key == "src" || a.Key == "poster") && goldhtml.IsDangerousURL(a.Val):
					report("%s %q of <%s>, a URL that a page leaves out", a.Key, a.Val, tok.Data)
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
				// [collapsibles] keeps the open of a <details> and no other attribute.
				for _, a := range tok.Attr {
					if tok.Data != "details" || a.Key != "open" {
						report("attribute %s of <%s>, which a page leaves out", a.Key, tok.Data)
					}
				}
			}
			// An end tag that writes nothing closes an element that its start
			// tag reported.
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
				"%q starts as a list item without the space after %q, which makes it text",
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

// excerpt returns line, without the space around it, shortened to 40 bytes.
func excerpt(line string) string {
	line = strings.TrimSpace(line)
	if len(line) > 40 {
		return line[:40] + "..."
	}
	return line
}

// lintErrors returns errs, the mistakes in src, the Markdown of file, each as
// file:line: mistake. src leaves out the front matter of file, whose lines the
// line counts when fsys holds file.
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

// lineOf returns the number of the line of src that holds the byte offset pos,
// counted from 1.
func lineOf(src []byte, pos int) int {
	if pos < 0 || pos > len(src) {
		return 0
	}
	return bytes.Count(src[:pos], []byte("\n")) + 1
}

// settingFiles returns the files of the docs directory dir that the settings of
// the site c take: the logo, the favicon, style.css and the icons of links, the
// header and the footer as written, and of the host h, nil without a setting.
func settingFiles(dir string, c chrome, links []frontMatterLink, h *hostSettings) []string {
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
	for _, l := range links {
		if l.Icon != "" {
			r = append(r, path.Join(dir, l.Icon))
		}
	}
	if h != nil && h.Icon != "" {
		r = append(r, path.Join(dir, h.Icon))
	}
	return r
}

// unusedFiles returns an error for each file of the docs directory of each
// edition of s in fsys that s doesn't use: no page shows it, no link names it
// and no setting takes it, where settings are the files that the settings
// take. Such a file is an image that no page shows any more, or a Markdown file
// in a directory without a position, which the navigation leaves out. A docs
// directory at the root of the repository has its groups checked alone, as the
// rest of it is the repository.
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
			// The icon of a group is that of its directory.
			case d.Name() == "icon.svg" && path.Dir(rel) != "." &&
				!strings.Contains(path.Dir(rel), "/") && orderPrefix.MatchString(path.Dir(rel)):
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
