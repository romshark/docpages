package docpages

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEmoji checks that GitHub emoji shortcodes become their characters,
// and that unknown and image-only names, code and times stay as written.
func TestEmoji(t *testing.T) {
	d := convert(t, dedentMD(`
		# T

		Ship it :rocket: :+1: :t-rex:, not :nope: or :octocat:, ~:tada:~ or 10:30:00.
	`))
	const text = "Ship it 🚀 👍 🦖, not :nope: or :octocat:, :tada: or 10:30:00."
	require.Contains(t, d.body, "<p>Ship it 🚀 👍 🦖, not :nope: or :octocat:, "+
		"<code>:tada:</code> or 10:30:00.</p>")
	require.Equal(t, text, d.description)
	require.Equal(t, "T "+text, d.search[0].Text)
}
