package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/storage"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	e, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRunTransitionsAndCheckpoints(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	run, err := e.CreateRun(ctx, t.TempDir(), "goal")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != TrustPending {
		t.Fatalf("initial status = %s", run.Status)
	}
	if err = e.Transition(ctx, run.ID, Done, ""); err == nil {
		t.Fatal("expected invalid transition")
	}
	if err = e.AddTasks(ctx, run.ID, []Task{{ID: "a", Name: "first"}}); err != nil {
		t.Fatal(err)
	}
	if err = e.Checkpoint(ctx, Checkpoint{RunID: run.ID, TaskID: "a", Name: "progress", Payload: []byte(`{"step":2}`)}); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := e.LastCheckpoint(ctx, run.ID, "a")
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Name != "progress" || string(checkpoint.Payload) != `{"step":2}` {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
	events, err := e.Events(ctx, run.ID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "run.created" {
		t.Fatalf("events = %#v", events)
	}
}

func TestRunDAGRespectsDependenciesAndRetries(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	run, err := e.CreateRun(ctx, t.TempDir(), "dag")
	if err != nil {
		t.Fatal(err)
	}
	tasks := []Task{{ID: "a", Name: "A"}, {ID: "b", Name: "B", Dependencies: []string{"a"}}, {ID: "c", Name: "C"}}
	if err = e.AddTasks(ctx, run.ID, tasks); err != nil {
		t.Fatal(err)
	}
	var aCalls, bCalls atomic.Int32
	worker := workerFunc(func(_ context.Context, task Task) error {
		switch task.ID {
		case "a":
			if aCalls.Add(1) == 1 {
				return errors.New("retry")
			}
		case "b":
			bCalls.Add(1)
			if aCalls.Load() < 2 {
				return errors.New("dependency ran too early")
			}
		}
		return nil
	})
	if err = e.RunDAG(ctx, run.ID, worker, SchedulerOptions{Workers: 2, MaxRetries: 1}); err != nil {
		t.Fatal(err)
	}
	if aCalls.Load() != 2 || bCalls.Load() != 1 {
		t.Fatalf("calls A=%d B=%d", aCalls.Load(), bCalls.Load())
	}
	got, err := e.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Gates {
		t.Fatalf("status = %s", got.Status)
	}
	gotTasks, err := e.Tasks(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range gotTasks {
		if task.Status != TaskDone {
			t.Fatalf("task %s status = %s", task.ID, task.Status)
		}
	}
}

func TestRunDAGBlocksAfterRetryLimit(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	run, err := e.CreateRun(ctx, t.TempDir(), "fail")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(ctx, run.ID, []Task{{ID: "a", Name: "A"}}); err != nil {
		t.Fatal(err)
	}
	err = e.RunDAG(ctx, run.ID, workerFunc(func(context.Context, Task) error { return errors.New("no") }), SchedulerOptions{MaxRetries: 1})
	if err == nil {
		t.Fatal("expected task failure")
	}
	got, readErr := e.GetRun(ctx, run.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got.Status != Blocked {
		t.Fatalf("run status = %s", got.Status)
	}
	gotTasks, readErr := e.Tasks(ctx, run.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(gotTasks) != 1 || gotTasks[0].Status != TaskBlocked || gotTasks[0].Retries != 1 {
		t.Fatalf("tasks = %#v", gotTasks)
	}
}

func TestAddTasksRejectsCyclesAndUnknownDependencies(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	run, err := e.CreateRun(ctx, t.TempDir(), "bad dag")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(ctx, run.ID, []Task{{ID: "a", Name: "A", Dependencies: []string{"missing"}}}); err == nil {
		t.Fatal("expected missing dependency failure")
	}
	if err = e.AddTasks(ctx, run.ID, []Task{{ID: "a", Name: "A", Dependencies: []string{"b"}}, {ID: "b", Name: "B", Dependencies: []string{"a"}}}); err == nil {
		t.Fatal("expected cycle failure")
	}
}

func TestRunDAGHonorsTaskTimeout(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	run, err := e.CreateRun(ctx, t.TempDir(), "timeout")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(ctx, run.ID, []Task{{ID: "a", Name: "A", Timeout: 10 * time.Millisecond}}); err != nil {
		t.Fatal(err)
	}
	err = e.RunDAG(ctx, run.ID, workerFunc(func(ctx context.Context, _ Task) error { <-ctx.Done(); return ctx.Err() }), SchedulerOptions{})
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestDoneRequiresDistinctReviewerAndPassedEvidence(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	run, err := e.CreateRun(ctx, t.TempDir(), "finish")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(ctx, run.ID, []Task{{ID: "a", Name: "A"}}); err != nil {
		t.Fatal(err)
	}
	if err = e.RunDAG(ctx, run.ID, workerFunc(func(context.Context, Task) error { return nil }), SchedulerOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = e.CompleteRun(ctx, run.ID, CompletionEvidence{ChecksPassed: true, ReviewApproved: true, Developer: "codex", Reviewer: "claude", Commit: "abc"}); err == nil {
		t.Fatal("DONE accepted missing gate and review events")
	}
	if err = e.addEvent(ctx, run.ID, "", "", "gate.cycle.started", map[string]string{"cycle": "verify-1"}); err != nil {
		t.Fatal(err)
	}
	if err = e.addEvent(ctx, run.ID, "", "", "gate.finished", map[string]any{"cycle": "verify-1", "result": GateResult{Name: "test", Passed: true}}); err != nil {
		t.Fatal(err)
	}
	if err = e.addEvent(ctx, run.ID, "", "", "review.finished", map[string]any{"cycle": "verify-1", "reviewer": "claude", "verdict": ReviewApprove}); err != nil {
		t.Fatal(err)
	}
	if err = e.Transition(ctx, run.ID, Done, ""); err == nil {
		t.Fatal("generic transition must not set DONE")
	}
	if err = e.CompleteRun(ctx, run.ID, CompletionEvidence{ChecksPassed: true, ReviewApproved: true, Developer: "claude", Reviewer: "claude", Commit: "abc"}); err == nil {
		t.Fatal("same reviewer and developer must not pass")
	}
	if err = e.CompleteRun(ctx, run.ID, CompletionEvidence{ChecksPassed: true, ReviewApproved: true, Developer: "codex", Reviewer: "claude", Commit: "abc"}); err != nil {
		t.Fatal(err)
	}
}

func TestRunDAGCancellationStopsWorkerAndPersistsState(t *testing.T) {
	e := testEngine(t)
	run, err := e.CreateRun(context.Background(), t.TempDir(), "cancel")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(context.Background(), run.ID, []Task{{ID: "a", Name: "long task"}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- e.RunDAG(ctx, run.ID, workerFunc(func(ctx context.Context, _ Task) error { close(started); <-ctx.Done(); return ctx.Err() }), SchedulerOptions{})
	}()
	<-started
	cancel()
	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("RunDAG error = %v", err)
	}
	saved, err := e.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != Cancelled {
		t.Fatalf("run status = %s", saved.Status)
	}
	tasks, err := e.Tasks(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Status != TaskCancelled {
		t.Fatalf("tasks = %#v", tasks)
	}
}

type workerFunc func(context.Context, Task) error

func (f workerFunc) Run(ctx context.Context, task Task) error { return f(ctx, task) }
