package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/sessions"
)

type historyContext struct {
	fakeHandlerContext
	project string
	item    sessions.Session
	trusted bool
}

func (c *historyContext) ProjectPath() string              { return c.project }
func (c *historyContext) Trusted() bool                    { return c.trusted }
func (c *historyContext) listSessions() []sessions.Session { return []sessions.Session{c.item} }
func (*historyContext) sendToSession(context.Context, sessions.SendRequest) (<-chan sessions.Event, error) {
	return nil, nil
}

func TestHistoryRejectsOtherProjectsAndTerminalControls(t *testing.T) {
	project := t.TempDir()
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	row, err := json.Marshal(map[string]any{"type": "user", "sessionId": "one", "cwd": project, "message": map[string]any{"role": "user", "content": "Hello \u001b[31mworld\u001b[0m"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(row, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	items := sessions.ScanMetadata(sessions.Roots{Claude: root})
	if len(items) != 1 {
		t.Fatalf("sessions: %d", len(items))
	}
	c := &historyContext{project: project, item: items[0], trusted: true}
	output, err := historyCommand(context.Background(), c, []string{c.item.ID})
	if err != nil || !strings.Contains(output, "Hello world") || strings.Contains(output, "\x1b") {
		t.Fatalf("output=%q error=%v", output, err)
	}
	c.project = t.TempDir()
	if _, err := historyCommand(context.Background(), c, []string{c.item.ID}); err == nil {
		t.Fatal("cross-project history accepted")
	}
	c.trusted = false
	if _, err := historyCommand(context.Background(), c, []string{c.item.ID}); err == nil {
		t.Fatal("untrusted history accepted")
	}
}
