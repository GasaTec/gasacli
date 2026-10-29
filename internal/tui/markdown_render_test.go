package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderTranscriptMarkdownAndSafeControls(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	got := renderTranscript("# Heading\n\n```go\nfunc main() {}\n```\n\n\x1b]52;c;c2VjcmV0\x07safe", 60, false)
	if strings.Contains(got, "\x1b]52") || strings.Contains(got, "c2VjcmV0") {
		t.Fatalf("unsafe escape survived: %q", got)
	}
	plain := ansi.Strip(got)
	for _, want := range []string{"Heading", "func main()", "safe"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in %q", want, plain)
		}
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected styled Markdown and code: %q", got)
	}
}

func TestRenderTranscriptBoundedUTF8(t *testing.T) {
	got := ansi.Strip(renderTranscript(strings.Repeat("é", 100000), 40, true))
	if !strings.Contains(got, "output truncated for display") {
		t.Fatal("missing truncation indication")
	}
}
