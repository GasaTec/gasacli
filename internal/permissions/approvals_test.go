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

func TestApprovalLogRotatesAtSizeLimit(t *testing.T) {
	old := approvalMaxBytes
	approvalMaxBytes = 1
	defer func() { approvalMaxBytes = old }()
	home := t.TempDir()
	e := New(home, nil)
	if err := e.RecordApproval(ApprovalEvent{Decision: Allow, Permission: "run_tests"}); err != nil {
		t.Fatal(err)
	}
	if err := e.RecordApproval(ApprovalEvent{Decision: Deny, Permission: "network_access"}); err != nil {
		t.Fatal(err)
	}
	if err := e.RecordApproval(ApprovalEvent{Decision: Allow, Permission: "run_tests"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(home, "approvals"))
	if err != nil {
		t.Fatal(err)
	}
	rotated := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "events-") && strings.HasSuffix(entry.Name(), ".jsonl") {
			rotated++
		}
	}
	if rotated != 2 {
		t.Fatalf("rotated files=%d entries=%v", rotated, entries)
	}
	if _, err := lastApprovalHash(filepath.Join(home, "approvals", "events.jsonl")); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "events-") && strings.HasSuffix(entry.Name(), ".jsonl") {
			path := filepath.Join(home, "approvals", entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			tampered := []byte(strings.Replace(string(data), `"permission":"run_tests"`, `"permission":"write_vault"`, 1))
			if err := os.WriteFile(path, tampered, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := lastApprovalHash(filepath.Join(home, "approvals", "events.jsonl")); err == nil {
				t.Fatal("tampered rotated history accepted")
			}
			break
		}
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
