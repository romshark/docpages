package docpages

import (
	"bytes"
	"html"
	"io"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

// mathLanguage is the language of a code block of TeX.
// A block between 2 lines of $$ becomes one too.
const mathLanguage = "math"

// mathExtension parses math: $...$ and $`...`$ in the text,
// $$...$$ on a line of its own, and a block between 2 lines of $$.
// Pages hold the TeX, which docs.ts typesets with KaTeX.
type mathExtension struct{}

func (mathExtension) ParserOptions(*parser.Config) []parser.Option {
	return []parser.Option{
		// Before paragraphs at 1000, which would hold a $$ line.
		parser.WithBlockParsers(util.Prioritized[parser.BlockParser](mathBlockParser{}, 750)),
		parser.WithInlineParsers(util.Prioritized[parser.InlineParser](mathInlineParser{}, 150)),
	}
}

func (mathExtension) RendererOptions(*goldhtml.Config) []goldhtml.Option {
	return []goldhtml.Option{
		goldhtml.WithNodeRenderers(map[ast.NodeKind]goldhtml.NodeRenderer{
			kindMath: renderer.NodeRendererFunc(renderMath),
		}),
	}
}

// mathNode is math in the text of a block.
type mathNode struct {
	ast.BaseInline
	tex     string
	display bool // Whether it is $$...$$, which shows on a line of its own.
}

var kindMath = ast.NewNodeKind("Math")

func (n *mathNode) Kind() ast.NodeKind { return kindMath }

func (n *mathNode) Dump([]byte) *ast.NodeDump {
	return ast.NewNodeDump(n, map[string]any{"TeX": n.tex, "Display": n.display})
}

// mathInlineParser parses math at a '$', with the rules of Pandoc.
// The TeX of $...$ neither starts nor ends with a space, and a digit doesn't follow it,
// so "$5 and $10" stays text. A \$ in the TeX is a dollar sign.
// The math can span the lines of its paragraph.
type mathInlineParser struct{}

func (mathInlineParser) Trigger() []byte { return []byte{'$'} }

func (mathInlineParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	n := &mathNode{}
	open, closer := "$", "$"
	switch {
	case bytes.HasPrefix(line, []byte("$`")):
		open, closer = "$`", "`$"
	case bytes.HasPrefix(line, []byte("$$")):
		open, closer, n.display = "$$", "$$", true
	case len(line) < 2 || util.IsSpace(line[1]):
		return nil
	}
	block.Advance(len(open))
	var tex strings.Builder
	for {
		line, _ := block.PeekLine()
		if line == nil {
			return nil
		}
		i := mathCloser(line, closer, open == "$`")
		if i < 0 {
			tex.Write(line)
			block.AdvanceLine()
			continue
		}
		tex.Write(line[:i])
		block.Advance(i + len(closer))
		break
	}
	n.tex = tex.String()
	if strings.TrimSpace(n.tex) == "" {
		return nil
	}
	return n
}

// mathCloser returns the index of closer in line, or -1. raw reads $`...`$,
// in which a backslash escapes nothing. A $ at 0 follows the opener or a
// line break, so it closes no $...$.
func mathCloser(line []byte, closer string, raw bool) int {
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '\\' && !raw:
			i++
		case !bytes.HasPrefix(line[i:], []byte(closer)):
		case closer != "$":
			return i
		case i == 0, util.IsSpace(line[i-1]):
		case i+1 < len(line) && '0' <= line[i+1] && line[i+1] <= '9':
		default:
			return i
		}
	}
	return -1
}

func renderMath(
	w io.Writer, _ []byte, node ast.Node, entering bool, _ renderer.Context,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*mathNode)
	class := "math-inline"
	if n.display {
		class = "math-display"
	}
	_, err := io.WriteString(w,
		`<span class="`+class+`">`+html.EscapeString(n.tex)+`</span>`)
	return ast.WalkSkipChildren, err
}

// mathBlockParser parses the lines between 2 lines of $$ into a math code block.
// Like a fenced code block, it can interrupt a paragraph, and without its
// closing line it ends with its container.
type mathBlockParser struct{}

func (mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (mathBlockParser) Open(
	_ ast.Node, reader text.Reader, pc parser.Context,
) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || !isMathFence(line[pos:]) {
		return nil, parser.NoChildren
	}
	reader.AdvanceToEOL()
	info := text.NewSingleLineValueFromString(mathLanguage, reader.Decoder())
	return ast.NewCodeBlock(ast.CodeBlockKindFenced, text.Lines{},
		ast.WithCodeBlockInfo(info)), parser.NoChildren
}

func (mathBlockParser) Continue(
	node ast.Node, reader text.Reader, _ parser.Context,
) parser.State {
	line, segment := reader.PeekLine()
	if w, pos := util.IndentWidth(line, reader.LineOffset()); w < 4 &&
		isMathFence(line[pos:]) {
		reader.AdvanceToEOL()
		return parser.Close
	}
	seg := text.NewSegment(segment.Start, segment.Stop)
	seg.ForceNewline = true
	node.(*ast.CodeBlock).Value.AppendSegment(seg)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

func (mathBlockParser) Close(ast.Node, text.Reader, parser.Context) {}

func (mathBlockParser) CanInterruptParagraph() bool { return true }

func (mathBlockParser) CanAcceptIndentedLine() bool { return false }

// isMathFence reports whether line is $$ and spaces.
func isMathFence(line []byte) bool {
	rest, ok := bytes.CutPrefix(line, []byte("$$"))
	return ok && util.IsBlank(rest)
}
