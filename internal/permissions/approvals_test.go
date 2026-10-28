package permissions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApprovalLogRedactsAndChains(t *testing.T) {
	home := t.TempDir()
	e := New(home, nil)
	event := ApprovalEvent{Decision: Allow, Permission: "execute_commands", PolicyRevision: "r1"}
	event.Operation.Summary = "run --token=abc123"
	event.Requester.AgentID = "agent"
	if err := e.RecordApproval(event); err != nil {
		t.Fatal(err)
	}
	event.EventID = "event-2"
	event.Operation.Summary = "safe command"
	if err := e.RecordApproval(event); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "approvals", "events.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "abc123") {
		t.Fatal("secret was persisted")
	}
	if _, err := lastApprovalHash(path); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var first, second ApprovalEvent
	_ = json.Unmarshal([]byte(lines[0]), &first)
	_ = json.Unmarshal([]byte(lines[1]), &second)
	if second.PreviousHash != first.Hash {
		t.Fatal("hash chain broken")
	}
	if !strings.Contains(first.Operation.Summary, "[REDACTED]") {
		t.Fatalf("target not redacted: %s", first.Operation.Summary)
	}
}

func TestApprovalTamperingFailsClosed(t *testing.T) {
	home := t.TempDir()
	e := New(home, nil)
	if err := e.RecordApproval(ApprovalEvent{Decision: Deny, Permission: "network_access"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "approvals", "events.jsonl")
	data, _ := os.ReadFile(path)
	data = []byte(strings.Replace(string(data), `"decision":"deny"`, `"decision":"allow"`, 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.RecordApproval(ApprovalEvent{Decision: Allow, Permission: "network_access"}); err == nil {
		t.Fatal("tampered tail accepted")
	}
}

func TestApprovers(t *testing.T) {
	ctx := context.Background()
	req := Request{Action: "write_project"}
	d, err := (NonInteractive{}).Ask(ctx, req)
	if err != nil || d.Effect != Deny {
		t.Fatalf("non-interactive=%+v err=%v", d, err)
	}
	s := &Scripted{Decisions: []Decision{{Effect: Allow}}}
	d, err = s.Ask(ctx, req)
	if err != nil || d.Effect != Allow {
		t.Fatalf("scripted=%+v err=%v", d, err)
	}
}
