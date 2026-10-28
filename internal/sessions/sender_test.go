package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
)

func TestFakeResumeProcess(t *testing.T) {
	mode := ""
	for i := range os.Args {
		if os.Args[i] == "--fake-mode" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
		}
	}
	if mode == "" {
		return
	}
	switch mode {
	case "sleep":
		time.Sleep(20 * time.Second)
	case "failure":
		_, _ = fmt.Fprintln(os.Stderr, "not logged in")
		os.Exit(9)
	default:
		_, _ = fmt.Fprintln(os.Stdout, `{"type":"result","result":"pong"}`)
		os.Exit(0)
	}
	os.Exit(0)
}

func TestCLISenderSuccessAndProviderError(t *testing.T) {
	for _, tc := range []struct {
		name, mode, want string
	}{
		{name: "success", mode: "success", want: "pong"},
		{name: "not signed in", mode: "failure", want: "Claude Code is not signed in"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sender, sessionID := testSender(t, tc.mode, true)
			stream, err := sender.Send(context.Background(), SendRequest{SessionID: sessionID, Text: "say pong"})
			if err != nil {
				t.Fatal(err)
			}
			var events []Event
			for event := range stream {
				events = append(events, event)
			}
			joined := ""
			for _, event := range events {
				joined += event.Text + event.Err
			}
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("events %#v do not contain %q", events, tc.want)
			}
			if strings.Contains(joined, "not logged in") {
				t.Fatal("provider diagnostics were exposed verbatim")
			}
		})
	}
}

func TestCLISenderCodexResumeArguments(t *testing.T) {
	sender := NewCLISender(Roots{}, nil, nil)
	sender.LookPath = func(name string) (string, error) {
		if name != "codex" {
			t.Fatalf("LookPath(%q), want codex", name)
		}
		return "fake-codex", nil
	}
	name, args, err := sender.command(Session{Agent: "Codex", ProviderID: "550e8400-e29b-41d4-a716-446655440000"}, "say pong")
	if err != nil {
		t.Fatal(err)
	}
	if name != "fake-codex" || strings.Join(args, " ") != "exec resume 550e8400-e29b-41d4-a716-446655440000 say pong --json" {
		t.Fatalf("Codex command = %q %#v", name, args)
	}
}

func TestCLISenderCancelAndConcurrentLock(t *testing.T) {
	sender, sessionID := testSender(t, "sleep", true)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := sender.Send(ctx, SendRequest{SessionID: sessionID, Text: "wait"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sender.Send(context.Background(), SendRequest{SessionID: sessionID, Text: "second"}); err == nil || !strings.Contains(err.Error(), "already being sent") {
		t.Fatalf("concurrent send error = %v", err)
	}
	cancel()
	for event := range stream {
		if event.Type == "error" && !strings.Contains(event.Err, "cancelled") {
			t.Fatalf("unexpected cancellation event: %#v", event)
		}
	}
}

func TestCLISenderRefusesUntrustedProject(t *testing.T) {
	sender, sessionID := testSender(t, "success", false)
	if _, err := sender.Send(context.Background(), SendRequest{SessionID: sessionID, Text: "say pong"}); err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("untrusted project error = %v", err)
	}
}

func TestCLISenderRejectsWithoutApproval(t *testing.T) {
	sender, sessionID := testSender(t, "success", true)
	sender.Approver = nil
	if _, err := sender.Send(context.Background(), SendRequest{SessionID: sessionID, Text: "say pong"}); err == nil || !strings.Contains(err.Error(), "requires approval") {
		t.Fatalf("missing approval error = %v", err)
	}
}

func TestCLISenderRequiresNetworkPermission(t *testing.T) {
	sender, sessionID := testSender(t, "success", true)
	if err := sender.Policy.SetRule(permissions.UserScope, "", "", "network_access", permissions.Deny); err != nil {
		t.Fatal(err)
	}
	if _, err := sender.Send(context.Background(), SendRequest{SessionID: sessionID, Text: "say pong"}); err == nil || !strings.Contains(err.Error(), "denied by policy") {
		t.Fatalf("network policy error = %v", err)
	}
}

func testSender(t *testing.T, mode string, trusted bool) (*CLISender, string) {
	t.Helper()
	home, root, transcripts := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	store := trust.NewStore(home)
	if trusted {
		if err := store.Grant(root); err != nil {
			t.Fatal(err)
		}
	}
	providerID := "550e8400-e29b-41d4-a716-446655440000"
	row, err := json.Marshal(map[string]any{"type": "user", "sessionId": providerID, "cwd": root, "message": map[string]any{"role": "user", "content": "existing"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(transcripts, "session.jsonl"), append(row, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	policy := permissions.New(home, store)
	decision := permissions.Decision{Effect: permissions.Allow, Reason: "user decision"}
	sender := NewCLISender(Roots{Claude: transcripts}, policy, &permissions.Scripted{Decisions: []permissions.Decision{decision, decision}})
	sender.LookPath = func(string) (string, error) { return os.Args[0], nil }
	sender.Command = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		wrapped := append([]string{"-test.run=TestFakeResumeProcess", "--", "--fake-mode", mode}, args...)
		return exec.CommandContext(ctx, os.Args[0], wrapped...)
	}
	items := ScanMetadata(Roots{Claude: transcripts})
	if len(items) != 1 {
		t.Fatalf("fixture sessions = %d", len(items))
	}
	t.Cleanup(func() { sessionSendStates.Delete(items[0].ID) })
	return sender, items[0].ID
}
