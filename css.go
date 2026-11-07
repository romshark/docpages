package docpages

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

// cssAtRules are allowed at-rules without "@":
// W3C rules plus page-margin and font-feature rules.
var cssAtRules = map[string]bool{
	"annotation": true, "bottom-center": true, "bottom-left": true,
	"bottom-left-corner": true, "bottom-right": true, "bottom-right-corner": true,
	"character-variant": true, "charset": true, "container": true,
	"counter-style": true, "custom-media": true, "font-face": true,
	"font-feature-values": true, "font-palette-values": true, "function": true,
	"historical-forms": true, "import": true, "keyframes": true, "layer": true,
	"left-bottom": true, "left-middle": true, "left-top": true, "media": true,
	"namespace": true, "ornaments": true, "page": true, "position-try": true,
	"property": true, "right-bottom": true, "right-middle": true, "right-top": true,
	"scope": true, "starting-style": true, "styleset": true, "stylistic": true,
	"supports": true, "swash": true, "top-center": true, "top-left": true,
	"top-left-corner": true, "top-right": true, "top-right-corner": true,
	"view-transition": true,
}

// cssMistake is a stylesheet error at a 1-based line and column.
type cssMistake struct {
	line, col int
	msg       string
}

// minifyCSS returns stylesheet src from name minified by esbuild, or errors as
// name:line:column: mistake. It catches browser-skipped CSS: esbuild errors such
// as unmatched {, missing declaration colons, and unknown properties, plus two
// cases that esbuild accepts and [scanCSS] finds.
func minifyCSS(name string, src []byte) ([]byte, error) {
	r := api.Transform(string(src), api.TransformOptions{
		Loader:           api.LoaderCSS,
		Sourcefile:       name,
		LogLevel:         api.LogLevelSilent,
		MinifyWhitespace: true,
		MinifySyntax:     true,
	})
	mistakes := scanCSS(src)
	for _, m := range slices.Concat(r.Errors, r.Warnings) {
		mistakes = append(mistakes, esbuildMistake(m))
	}
	if len(mistakes) == 0 {
		return []byte(r.Code), nil
	}
	slices.SortStableFunc(mistakes, func(a, b cssMistake) int {
		return cmp.Or(cmp.Compare(a.line, b.line), cmp.Compare(a.col, b.col))
	})
	errs := make([]error, len(mistakes))
	for i, m := range mistakes {
		errs[i] = fmt.Errorf("%s:%d:%d: %s", name, m.line, m.col, m.msg)
	}
	return nil, errors.Join(errs...)
}

// esbuildMistake converts message m and its notes—such as typo suggestions or
// an unmatched {'s location—into a mistake.
func esbuildMistake(m api.Message) cssMistake {
	r := cssMistake{msg: m.Text}
	if m.Location != nil {
		r.line, r.col = m.Location.Line, m.Location.Column+1
	}
	var notes []string
	for _, n := range m.Notes {
		if n.Location != nil {
			notes = append(notes, fmt.Sprintf("%s %d:%d", n.Text, n.Location.Line, n.Location.Column+1))
		} else if n.Text != "" {
			notes = append(notes, n.Text)
		}
	}
	if len(notes) > 0 {
		r.msg += " (" + strings.Join(notes, "; ") + ")"
	}
	return r
}

// scanCSS finds errors esbuild accepts: at-rules outside [cssAtRules], such as
// typos, and valueless declarations such as color: ;. Vendor-prefixed rules
// such as @-webkit-keyframes and valueless custom properties pass.
func scanCSS(src []byte) []cssMistake {
	type token struct {
		tt     css.TokenType
		data   string
		offset int
	}
	var toks []token
	l := css.NewLexer(parse.NewInputBytes(src))
	for offset := 0; ; {
		tt, data := l.Next()
		if tt == css.ErrorToken {
			break
		}
		if tt != css.WhitespaceToken && tt != css.CommentToken {
			toks = append(toks, token{tt, string(data), offset})
		}
		offset += len(data)
	}
	var mistakes []cssMistake
	report := func(offset int, format string, args ...any) {
		line := bytes.Count(src[:offset], []byte("\n")) + 1
		col := offset - bytes.LastIndexByte(src[:offset], '\n')
		mistakes = append(mistakes, cssMistake{line, col, fmt.Sprintf(format, args...)})
	}
	for i, t := range toks {
		switch {
		case t.tt == css.AtKeywordToken:
			rule := strings.ToLower(strings.TrimPrefix(t.data, "@"))
			if !cssAtRules[rule] && !strings.HasPrefix(rule, "-") {
				report(t.offset, "unknown at-rule @%s", rule)
			}
		// No selector holds a name, a colon and then a ; or a }.
		case t.tt == css.IdentToken &&
			i+2 < len(toks) &&
			toks[i+1].tt == css.ColonToken &&
			(toks[i+2].tt == css.SemicolonToken || toks[i+2].tt == css.RightBraceToken):
			report(t.offset, "%s without a value", strings.ToLower(t.data))
		}
	}
	return mistakes
}
