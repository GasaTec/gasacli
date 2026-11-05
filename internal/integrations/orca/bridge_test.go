package orca

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

type fakeRunner struct {
	commands []Command
	runtime  string
}

func (f *fakeRunner) Run(_ context.Context, c Command) (Result, error) {
	f.commands = append(f.commands, c)
	if c.Args[0] == "agent-context" {
		return Result{Started: true, ExitCode: 0, Stdout: `{"schemaVersion":1,"commands":[{"command":"orchestration run-show","flags":["id","json"]},{"command":"orchestration worker-show","flags":["dispatch","json"]},{"command":"orchestration worker-read","flags":["dispatch","source","limit","cursor","json"]},{"command":"orchestration inbox","flags":["terminal","limit","json"]},{"command":"orchestration request-show","flags":["request","json"]},{"command":"agent launch","flags":["agent","worktree","surface","prompt","retry-launch","json"]}]}`}, nil
	}
	if c.Args[0] == "status" {
		v := map[string]any{"ok": true, "result": map[string]any{"runtime": map[string]any{"runtimeId": f.runtime, "appVersion": "test", "state": "ready", "reachable": true, "capabilities": []string{"orchestration.contract.v1", "agent.launch.replay.v1"}}}, "_meta": map[string]string{"runtimeId": f.runtime}}
		data, _ := json.Marshal(v)
		return Result{Started: true, ExitCode: 0, Stdout: string(data)}, nil
	}
	return Result{Started: true, ExitCode: 0, Stdout: `{"ok":true,"result":{},"_meta":{"runtimeId":"runtime-one"}}`}, nil
}
func TestBridgeDoesNotConsumeInboxOrImpersonateCoordinator(t *testing.T) {
	fake := &fakeRunner{runtime: "runtime-one"}
	dir, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := NewWithRunner("orca-test", dir, fake)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := bridge.Probe(context.Background())
	if err != nil || !capability.CanObserve || capability.CanExecute {
		t.Fatal("unexpected readiness", capability, err)
	}
	if _, err := bridge.Inbox(context.Background(), "dispatch:one"); err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.StopWorker(context.Background(), "one"); !errors.Is(err, ErrCoordinatorAuthority) {
		t.Fatal("unbound gateway acquired coordinator authority")
	}
	for _, c := range fake.commands {
		for _, arg := range c.Args {
			if arg == "check" || arg == "--from" {
				t.Fatal("consumed inbox or fabricated identity")
			}
		}
	}
}
func TestChangedRuntimeBlocksCoordinatorLaunchBeforeEffects(t *testing.T) {
	fake := &fakeRunner{runtime: "runtime-one"}
	bridge, err := NewWithRunner("orca-test", t.TempDir(), fake)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.runtime = "replacement"
	if _, err := bridge.LaunchCoordinator(context.Background(), LaunchRequest{AgentID: "codex", Worktree: "identity:known", Prompt: "Authorized coordinator intent"}); err == nil {
		t.Fatal("launched against replacement runtime")
	}
	for _, c := range fake.commands {
		if c.Args[0] == "agent" {
			t.Fatal("runtime fence happened after launch")
		}
	}
}

func TestPublicTranscriptReadNeverRequestsTerminalFallback(t *testing.T) {
	fake := &fakeRunner{runtime: "runtime-one"}
	bridge, err := NewWithRunner("orca-test", t.TempDir(), fake)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.ReadWorkerTranscript(context.Background(), "dispatch-one", "", 50); err != nil {
		t.Fatal(err)
	}
	args := fake.commands[len(fake.commands)-1].Args
	want := []string{"orchestration", "worker-read", "--dispatch", "dispatch-one", "--source", "transcript", "--limit", "50", "--json"}
	if len(args) != len(want) {
		t.Fatalf("unexpected transcript command: %v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("unexpected transcript command: %v", args)
		}
	}
	count := len(fake.commands)
	if _, err := bridge.ReadWorkerTranscript(context.Background(), "dispatch-one", "", 0); err == nil {
		t.Fatal("unbounded request accepted")
	}
	if len(fake.commands) != count {
		t.Fatal("invalid read contacted host")
	}
}
