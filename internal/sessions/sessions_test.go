package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	if len(metadata) != 1 || len(metadata[0].Messages) != 0 || metadata[0].SourcePath != path || metadata[0].MessageCount != 3 {
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
