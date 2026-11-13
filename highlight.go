package docpages

import (
	"context"
	"html"
	"io"
	"regexp"
	"strconv"

	"github.com/alecthomas/chroma/v3"
	chromahtml "github.com/alecthomas/chroma/v3/formatters/html"
	"github.com/alecthomas/chroma/v3/lexers"
	"github.com/alecthomas/chroma/v3/styles"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/renderer"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
)

// codeFormatter writes tokens as <span> elements with Chroma's short classes,
// such as "k" for keywords, colored by style.css.
var codeFormatter = chromahtml.New(
	chromahtml.WithClasses(true),
	chromahtml.PreventSurroundingPre(true),
)

// codeTitle finds a title in a code-block info string such as
// ```go title="app/app.go". GitHub uses its first word as the language and
// ignores the rest.
var codeTitle = regexp.MustCompile(`\btitle="([^"]*)"`)

// codeBlocks returns one page's code-block renderer. It shows optional titles
// above blocks, adds copy buttons, and uses Chroma when the info-string language
// has a lexer. docs.ts draws mermaid blocks as diagrams, as GitHub does.
//
// It is a decorator because goldmark applies extension renderers, including
// CommonMark, after option renderers. Per-page block numbering supplies copy IDs,
// so one renderer cannot span pages.
func codeBlocks() func(goldhtml.NodeRenderer) goldhtml.NodeRenderer {
	n := 0
	return func(goldhtml.NodeRenderer) goldhtml.NodeRenderer {
		return renderer.NodeRendererFunc(func(
			w io.Writer, source []byte, node ast.Node, entering bool, _ renderer.Context,
		) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			b := node.(*ast.CodeBlock)
			if lang, _ := b.Language(source); lang == "mermaid" {
				_, err := io.WriteString(w, `<pre class="mermaid">`+
					html.EscapeString(b.Value.Str(source))+"</pre>\n")
				return ast.WalkSkipChildren, err
			}
			n++
			// goldmark omits "_" from heading IDs, keeping this ID distinct.
			id := "code_" + strconv.Itoa(n)
			button := func() error {
				return copyButton(id).Render(context.Background(), w)
			}
			// Put a titled block's button in the title to center them together.
			title := codeTitle.FindStringSubmatch(b.Info.Value(source))
			_, _ = io.WriteString(w, `<div class="code-block">`)
			if title != nil {
				_, _ = io.WriteString(w,
					`<div class="code-title">`+html.EscapeString(title[1]))
				if err := button(); err != nil {
					return ast.WalkStop, err
				}
				_, _ = io.WriteString(w, `</div>`)
			}
			_, _ = io.WriteString(w, `<div class="code-body" id="`+id+`">`)
			if err := writeCode(w, b, source); err != nil {
				return ast.WalkStop, err
			}
			_, _ = io.WriteString(w, `</div>`)
			if title == nil {
				if err := button(); err != nil {
					return ast.WalkStop, err
				}
			}
			_, _ = io.WriteString(w, "</div>\n")
			return ast.WalkSkipChildren, nil
		})
	}
}

// writeCode writes b as <pre>, using its info-string language when Chroma supports it.
func writeCode(w io.Writer, b *ast.CodeBlock, source []byte) error {
	code := b.Value.Str(source)
	lang, _ := b.Language(source)
	if lang == "" {
		_, err := io.WriteString(w, "<pre><code>"+html.EscapeString(code)+"</code></pre>")
		return err
	}
	_, _ = io.WriteString(w, `<pre><code class="language-`+html.EscapeString(lang)+`">`)
	if lexer := lexers.Get(lang); lexer == nil {
		_, _ = io.WriteString(w, html.EscapeString(code))
	} else {
		tokens, err := chroma.Coalesce(lexer).Tokenise(nil, code)
		if err != nil {
			return err
		}
		// The classes leave colors to style.css, so the style sets colors only.
		if err := codeFormatter.Format(w, styles.Fallback, tokens); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "</code></pre>")
	return err
}
