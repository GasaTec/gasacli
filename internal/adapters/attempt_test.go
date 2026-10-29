package adapters

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
)

func attemptFixture(t *testing.T, script string) (*Executor, Request) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell")
	}
	home, project := t.TempDir(), t.TempDir()
	store := trust.NewStore(home)
	if err := store.Grant(project); err != nil {
		t.Fatal(err)
	}
	policy := permissions.New(home, store)
	if err := policy.SetProfile("balanced"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, "fake-agent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return &Executor{Policy: policy, LookPath: func(string) (string, error) { return path, nil }}, Request{Agent: "claude", Project: project, Prompt: "test", ReadOnly: true}
}

func TestAttemptEventsRetryAndSuccess(t *testing.T) {
	executor, request := attemptFixture(t, "if [ ! -f .attempt ]; then touch .attempt; exit 1; fi\nprintf '%s\\n' '{\"type\":\"result\"}'")
	var events []AttemptEvent
	executor.OnAttempt = func(event AttemptEvent) { events = append(events, event) }
	if _, err := executor.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		attempt int
		status  string
	}{{1, "started"}, {1, "failed"}, {2, "started"}, {2, "succeeded"}}
	if len(events) != len(want) {
		t.Fatalf("events: %#v", events)
	}
	for i, expected := range want {
		if events[i].ExecutionID == 0 || events[i].ExecutionID != events[0].ExecutionID || events[i].Attempt != expected.attempt || events[i].Status != expected.status {
			t.Fatalf("event %d: %#v", i, events[i])
		}
	}
}

func TestAttemptEventsCancellation(t *testing.T) {
	executor, request := attemptFixture(t, "exec sleep 30")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []AttemptEvent
	executor.OnAttempt = func(event AttemptEvent) {
		events = append(events, event)
		if event.Status == "started" {
			cancel()
		}
	}
	if _, err := executor.Execute(ctx, request); err == nil {
		t.Fatal("expected cancellation")
	}
	if len(events) != 2 || events[0].Status != "started" || events[1].Status != "canceled" {
		t.Fatalf("events: %#v", events)
	}
}

func TestAttemptEventsTimeout(t *testing.T) {
	executor, request := attemptFixture(t, "exec sleep 30")
	request.Timeout = 50 * time.Millisecond
	var events []AttemptEvent
	executor.OnAttempt = func(event AttemptEvent) { events = append(events, event) }
	if _, err := executor.Execute(context.Background(), request); err == nil {
		t.Fatal("expected timeout")
	}
	if len(events) != 2 || events[0].Status != "started" || events[1].Status != "timed_out" {
		t.Fatalf("events: %#v", events)
	}
}

func TestAttemptEventsConcurrentExecutionsHaveDistinctIDs(t *testing.T) {
	executor, request := attemptFixture(t, "printf '%s\\n' '{\"type\":\"result\"}'")
	var mu sync.Mutex
	var events []AttemptEvent
	executor.OnAttempt = func(event AttemptEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, event) }
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := executor.Execute(context.Background(), request); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	ids := map[uint64][]string{}
	for _, e := range events {
		ids[e.ExecutionID] = append(ids[e.ExecutionID], e.Status)
	}
	if len(ids) != 2 {
		t.Fatalf("events: %#v", events)
	}
	for id, statuses := range ids {
		if id == 0 || len(statuses) != 2 || statuses[0] != "started" || statuses[1] != "succeeded" {
			t.Fatalf("events: %#v", events)
		}
	}
}
