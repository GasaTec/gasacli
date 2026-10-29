package sessions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type cancelDuringScan struct {
	context.Context
	checks int
}

func (c *cancelDuringScan) Err() error {
	c.checks++
	if c.checks >= 5 {
		return context.Canceled
	}
	return nil
}

func TestScanMetadataContextDiscardsPartialResultsOnCancellation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.jsonl", "b.jsonl", "c.jsonl"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(`{"cwd":"/fixture","sessionId":"fixture"}`+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &cancelDuringScan{Context: context.Background()}
	items, err := ScanMetadataContext(ctx, Roots{Claude: root})
	if !errors.Is(err, context.Canceled) || len(items) != 0 {
		t.Fatalf("items=%v err=%v", items, err)
	}
}

func TestScanClaudeDefensiveParsingAndRedaction(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(t.TempDir(), "trusted-project")
	projectJSON := filepath.ToSlash(project)
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "encoded-project", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	fixture := strings.Join([]string{
		`{broken`,
		`{"type":"file-history-snapshot","snapshot":{}}`,
		`{"type":"user","sessionId":"session-1","cwd":"` + projectJSON + `","timestamp":"2026-10-20T10:00:00Z","message":{"role":"user","content":"Fix the dashboard"}}`,
		`{"type":"assistant","sessionId":"session-1","cwd":"` + projectJSON + `","timestamp":"2026-10-20T10:01:00Z","message":{"role":"assistant","model":"claude-sonnet","content":[{"type":"text","text":"Use sk-secret-value"},{"type":"tool_use","name":"Read","input":{"file_path":".env"}}]}}`,
		strings.Repeat("x", maxLineBytes+10),
		`{"type":"assistant","cwd":"` + projectJSON + `","message":{"role":"assistant","content":"Still readable after oversized line"}}`,
		`{"type":"user","cwd":"` + projectJSON + `","message":{"role":"user","content":"truncated tail"`,
	}, "\n")
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	got := Scan(Roots{Claude: root})
	if len(got) != 1 {
		t.Fatalf("Scan returned %d sessions, want 1", len(got))
	}
	s := got[0]
	if s.Agent != "Claude Code" || filepath.Clean(s.CWD) != filepath.Clean(project) || s.Title != "Fix the dashboard" || s.Model != "claude-sonnet" {
		t.Fatalf("unexpected session metadata: %#v", s)
	}
	joined := s.Messages[1].Text + s.Messages[1].ToolCalls[0].Input
	if strings.Contains(joined, "sk-secret-value") || !strings.Contains(joined, "[redacted]") {
		t.Fatalf("secret not redacted: %q", joined)
	}
	if len(s.Messages) != 3 || !strings.Contains(s.Messages[2].Text, "Still readable") {
		t.Fatalf("messages after oversized line were not read: %#v", s.Messages)
	}
	if s.MessageCount != 3 || s.MessageOffset != 0 {
		t.Fatalf("message pagination metadata = %d/%d", s.MessageOffset, s.MessageCount)
	}
	metadata := ScanMetadata(Roots{Claude: root})
	if len(metadata) != 1 || len(metadata[0].Messages) != 0 || metadata[0].SourcePath != path {
		t.Fatalf("metadata scan retained transcript data or lost its index: %#v", metadata)
	}
	loaded, ok := LoadTranscript(metadata[0].Agent, metadata[0].SourcePath)
	if !ok || len(loaded.Messages) != 3 || strings.Contains(loaded.Messages[1].Text, "sk-secret-value") {
		t.Fatalf("individual transcript did not load safely: %#v", loaded)
	}
}

func TestScanCodexRecordsUnknownAndMissingRoots(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(t.TempDir(), "codex-project")
	projectJSON := filepath.ToSlash(project)
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "2026", "rollout-test.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	rows := []string{
		`{"type":"session_meta","payload":{"id":"id-1","cwd":"` + projectJSON + `","model":"gpt-6.1-sol"}}`,
		`{"type":"future_record","payload":{"secret":"sk-ignore"}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"Review this"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Done safely"}]}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"git status\"}"}}`,
		`{"type":"event_msg","payload":{"type":"agent_reasoning","message":"must not expose hidden reasoning"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(rows, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.jsonl"), []byte(rows[2]), 0600); err != nil {
		t.Fatal(err)
	}
	got := Scan(Roots{Codex: root})
	if len(got) != 1 {
		t.Fatalf("Scan returned %d sessions, want 1", len(got))
	}
	s := got[0]
	if s.Model != "gpt-6.1-sol" || s.Title != "Review this" || len(s.Messages) != 3 {
		t.Fatalf("unexpected parsed session: %#v", s)
	}
	for _, m := range s.Messages {
		if strings.Contains(m.Text, "hidden reasoning") {
			t.Fatal("reasoning event was exposed")
		}
	}
	if empty := Scan(Roots{Claude: filepath.Join(root, "missing")}); len(empty) != 0 {
		t.Fatal("missing roots should be skipped")
	}
}

func TestParseCodexExternalAgentToolBlocks(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	input := "[external_agent_tool_call: Bash]\ncommand: print sk-1234567890abcdefghijklmnop\n[/external_agent_tool_call]\n" +
		"[external_agent_tool_result]\noutput: xoxb-1234567890-abcdef123456\n[/external_agent_tool_result]\n" +
		"Final answer"
	text, calls := parseCodexToolBlocks(input)
	if text != "Final answer" || len(calls) != 2 {
		t.Fatalf("parsed text/tools = %q/%#v", text, calls)
	}
	if calls[0].Kind != "Bash" || !strings.Contains(calls[0].Input, "[redacted]") || strings.Contains(calls[0].Input, "sk-123") {
		t.Fatalf("tool call was not summarized and redacted: %#v", calls[0])
	}
	if calls[1].Output == "" || strings.Contains(calls[1].Output, "xoxb-") {
		t.Fatalf("tool result was not redacted: %#v", calls[1])
	}
}

func TestParseCodexToolBlocksStructuredAndDamaged(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	for _, tc := range []struct {
		name      string
		input     string
		wantText  string
		wantCalls int
	}{
		{
			name: "description command and markdown result",
			input: "Before\n[external_agent_tool_call: Bash]\ndescription: List files\ncommand: git status --short\n" +
				"[/external_agent_tool_call]\n[external_agent_tool_result]\noutput: **clean**\n- no changes\n[/external_agent_tool_result]\nAfter",
			wantText: "Before\n\nAfter", wantCalls: 2,
		},
		{
			name: "nested wrappers are extracted",
			input: "[external_agent_tool_call: Bash]\ncommand: echo nested\n" +
				"[external_agent_tool_result]\noutput: nested result\n[/external_agent_tool_result]\n[/external_agent_tool_call]",
			wantCalls: 2,
		},
		{
			name:     "unterminated wrapper is consumed",
			input:    "Visible\n[external_agent_tool_result]\noutput: tail without close",
			wantText: "Visible", wantCalls: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, calls := parseCodexToolBlocks(tc.input)
			if text != tc.wantText || len(calls) != tc.wantCalls {
				t.Fatalf("parsed text/tools = %q/%#v, want %q/%d", text, calls, tc.wantText, tc.wantCalls)
			}
			if strings.Contains(text, "external_agent_") {
				t.Fatalf("wrapper markup leaked into prose: %q", text)
			}
			if tc.name == "description command and markdown result" {
				if calls[0].Description != "List files" || calls[0].Command != "git status --short" || !strings.Contains(calls[1].Output, "**clean**") {
					t.Fatalf("structured fields or markdown output lost: %#v", calls)
				}
			}
		})
	}
	large := strings.Repeat("x", maxMessageBytes+100)
	_, calls := parseCodexToolBlocks("[external_agent_tool_result]\noutput: " + large + "\n[/external_agent_tool_result]")
	s := Session{}
	appendMessage(&s, Message{Role: "tool", ToolCalls: calls})
	if len(s.Messages[0].ToolCalls[0].Output) > maxMessageBytes+20 {
		t.Fatalf("oversized tool output was not bounded: %d bytes", len(s.Messages[0].ToolCalls[0].Output))
	}
}

func TestScanKeepsCodexSessionsWhenClaudeRootIsFull(t *testing.T) {
	claudeRoot, codexRoot := t.TempDir(), t.TempDir()
	project := filepath.ToSlash(t.TempDir())
	for i := 0; i < maxSessions/2+1; i++ {
		path := filepath.Join(claudeRoot, fmt.Sprintf("session-%03d.jsonl", i))
		row := `{"type":"user","cwd":"` + project + `","message":{"role":"user","content":"Claude session"}}`
		if err := os.WriteFile(path, []byte(row), 0600); err != nil {
			t.Fatal(err)
		}
	}
	codexFile := filepath.Join(codexRoot, "rollout-test.jsonl")
	rows := []string{
		`{"type":"session_meta","payload":{"id":"codex-1","cwd":"` + project + `"}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"Codex session"}}`,
	}
	if err := os.WriteFile(codexFile, []byte(strings.Join(rows, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	got := ScanMetadata(Roots{Claude: claudeRoot, Codex: codexRoot})
	want := maxSessions/2 + 1
	if len(got) != want {
		t.Fatalf("ScanMetadata returned %d sessions, want %d", len(got), want)
	}
	for _, session := range got {
		if session.Agent == "Codex" {
			return
		}
	}
	t.Fatal("Codex session was omitted when Claude exceeded its scan share")
}

func TestRedactPrivateKeyBlock(t *testing.T) {
	got := redact("before\n-----BEGIN PRIVATE KEY-----\nSECRET MATERIAL\n-----END PRIVATE KEY-----\nafter")
	if strings.Contains(got, "SECRET MATERIAL") || !strings.Contains(got, "[redacted]") || !strings.Contains(got, "after") {
		t.Fatalf("redaction failed: %q", got)
	}
}

func TestRedactCredentialShapesWithoutChangingBenignText(t *testing.T) {
	input := "<task-id> 0123456789abcdef0123456789abcdef 550e8400-e29b-41d4-a716-446655440000\n" +
		"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature\n" +
		"AWS AKIA1234567890ABCDEF\n" +
		"XOX xoxb-1234567890-abcdef123456\n" +
		"GITHUB ghp_abcdefghijklmnopqrstuvwxyz1234567890\n" +
		"SERVICE_URL=https://example.test/.env/path\n" +
		"OPENAI_API_KEY=sk-1234567890abcdefghijklmnop"
	got := redact(input)
	for _, benign := range []string{"<task-id>", "0123456789abcdef0123456789abcdef", "550e8400-e29b-41d4-a716-446655440000", "SERVICE_URL=https://example.test/.env/path"} {
		if !strings.Contains(got, benign) {
			t.Errorf("benign text %q was changed: %q", benign, got)
		}
	}
	for _, secret := range []string{"eyJhbGciOiJIUzI1NiJ9", "AKIA1234567890ABCDEF", "xoxb-1234567890-abcdef123456", "ghp_abcdefghijklmnopqrstuvwxyz1234567890", "sk-1234567890abcdefghijklmnop"} {
		if strings.Contains(got, secret) {
			t.Errorf("secret %q was not removed: %q", secret, got)
		}
	}
	if !strings.Contains(got, "OPENAI_API_KEY=[redacted]") {
		t.Fatalf("secret-looking environment assignment was not redacted: %q", got)
	}
}

func TestClassifyStatusUsesRecentTranscriptOrLiveProcess(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name         string
		updatedAt    time.Time
		processAlive bool
		want         string
	}{
		{name: "recent transcript", updatedAt: now.Add(-9 * time.Second), want: "working"},
		{name: "old transcript", updatedAt: now.Add(-11 * time.Second), want: "done"},
		{name: "live process", updatedAt: now.Add(-time.Hour), processAlive: true, want: "working"},
		{name: "future timestamp", updatedAt: now.Add(time.Second), want: "done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyStatus(tc.updatedAt, now, tc.processAlive); got != tc.want {
				t.Fatalf("classifyStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}
