package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/sessions"
)

type streamTestContext struct {
	fakeHandlerContext
	request sessions.SendRequest
	events  []sessions.Event
}

func (*streamTestContext) Ask(PermissionRequest) (bool, error) { return true, nil }
func (*streamTestContext) listSessions() []sessions.Session {
	return []sessions.Session{{ID: "codex-12345678", Agent: "Codex"}}
}
func (c *streamTestContext) sendToSession(_ context.Context, request sessions.SendRequest) (<-chan sessions.Event, error) {
	c.request = request
	ch := make(chan sessions.Event, len(c.events))
	for _, event := range c.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func TestLiveSendPreservesTextAndCombinesStreamFragments(t *testing.T) {
	c := &streamTestContext{events: []sessions.Event{{Type: "text", Text: "hel"}, {Type: "text", Text: "lo\nwor"}, {Type: "text", Text: "ld"}, {Type: "done"}}}
	input := "first line\n\"second line\""
	result, err := sendLiveSession(context.Background(), c, "codex-12345678", input)
	if err != nil || result != "" || c.request.Text != input || !reflect.DeepEqual(c.emitted, []string{"hello\n", "world"}) {
		t.Fatalf("result=%q err=%v request=%q emitted=%q", result, err, c.request.Text, c.emitted)
	}
}
func TestLiveSendFlushesPartialOutputOnErrorAndBoundsLine(t *testing.T) {
	text := strings.Repeat("a", (64<<10)+1)
	c := &streamTestContext{events: []sessions.Event{{Type: "text", Text: text}, {Type: "error", Err: "cancelled"}}}
	result, err := sendLiveSession(context.Background(), c, "codex-12345678", "hello")
	if err != nil || !strings.Contains(result, "cancelled") || len(c.emitted) != 2 || strings.Join(c.emitted, "") != text {
		t.Fatalf("result=%q err=%v chunks=%d", result, err, len(c.emitted))
	}
}
func TestLiveSendRemovesTerminalControlSequences(t *testing.T) {
	c := &streamTestContext{events: []sessions.Event{{Type: "text", Text: "safe\x1b]0;"}, {Type: "text", Text: "bad\x07\n"}, {Type: "error", Err: "oops\x1b[2J"}}}
	result, err := sendLiveSession(context.Background(), c, "codex-12345678", "hello")
	if err != nil || strings.ContainsAny(result+strings.Join(c.emitted, ""), "\x1b\x07") {
		t.Fatalf("unsafe output: %q %q (%v)", result, c.emitted, err)
	}
}
func TestLiveSendBoundsLongEscapeSequence(t *testing.T) {
	value := "before\x1b]0;" + strings.Repeat("x", 70<<10) + "\x07after\n"
	c := &streamTestContext{events: []sessions.Event{{Type: "text", Text: value}}}
	_, err := sendLiveSession(context.Background(), c, "codex-12345678", "hello")
	joined := strings.Join(c.emitted, "")
	if err != nil || strings.ContainsAny(joined, "\x1b\x07") || !strings.Contains(joined, "before") || !strings.Contains(joined, "after") {
		t.Fatalf("unsafe or lost ordinary text, length=%d err=%v", len(joined), err)
	}
}
func TestLiveChatRequiresSelectedTrustedSession(t *testing.T) {
	result, err := executeLiveChat(context.Background(), &Session{}, "project", false, "", "text", nil, nil)
	if err != nil || !strings.Contains(result, "/use") {
		t.Fatalf("result=%q err=%v", result, err)
	}
	if _, err = selectLiveSession(context.Background(), &Session{}, "project", false, "id"); err == nil {
		t.Fatal("untrusted selection accepted")
	}
}
