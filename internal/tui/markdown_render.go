package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/termenv"
)

// renderTranscript renders untrusted provider text as Markdown. The cap bounds
// work and memory when a provider produces an unusually large response. The
// caller should cache output and rerender only on content or width changes.
func renderTranscript(text string, width int, light bool) string {
	const maxBytes = 128 << 10
	if len(text) > maxBytes {
		text = strings.ToValidUTF8(text[:maxBytes], "�")
		text += "\n\n[output truncated for display]"
	}
	text = terminalText(text)
	// Glamour's wrapping of very long, unbroken lines can be quadratic. Keep
	// such output readable and bounded without running it through the parser.
	for _, line := range strings.Split(text, "\n") {
		if len(line) > 1024 {
			return text
		}
	}
	if width < 20 {
		width = 20
	}
	if width > 240 {
		width = 240
	}
	style := "dark"
	if light {
		style = "light"
	}
	profile := termenv.ANSI256
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		profile = termenv.Ascii
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(width), glamour.WithColorProfile(profile))
	if err != nil {
		return text
	}
	rendered, err := renderer.Render(text)
	if err != nil {
		return text
	}
	return strings.Trim(rendered, "\n")
}
