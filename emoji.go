package docpages

import (
	"github.com/yuin/goldmark-emoji/definition"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

// emojiExtension replaces GitHub emoji shortcodes, such as :tada:, with their characters.
// A name that GitHub doesn't know, or shows as an image, such as :octocat:, stays text.
type emojiExtension struct{}

func (emojiExtension) ParserOptions(*parser.Config) []parser.Option {
	return []parser.Option{
		parser.WithInlineParsers(
			util.Prioritized[parser.InlineParser](emojiParser{}, 999),
		),
	}
}

// emojiParser parses a shortcode at a ':'.
type emojiParser struct{}

func (emojiParser) Trigger() []byte { return []byte{':'} }

func (emojiParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	end := 1
	for end < len(line) && isEmojiNameByte(line[end]) {
		end++
	}
	if end == 1 || end == len(line) || line[end] != ':' {
		return nil
	}
	e, ok := definition.Github().Get(string(line[1:end]))
	if !ok || !e.IsUnicode() {
		return nil
	}
	block.Advance(end + 1)
	return ast.NewText(
		text.NewSingleLineValueFromString(
			string(e.Unicode),
			block.Decoder(),
		),
	)
}

// isEmojiNameByte reports whether c may appear in a shortcode, as in :+1:,
// :t-rex: and :1st_place_medal:.
func isEmojiNameByte(c byte) bool {
	return 'a' <= c && c <= 'z' || '0' <= c &&
		c <= '9' || c == '_' || c == '+' || c == '-'
}
