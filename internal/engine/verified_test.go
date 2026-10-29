package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestVerifiedRunRetriesGatesAndReviewThenCompletes(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	run, err := e.CreateRun(ctx, t.TempDir(), "goal")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(ctx, run.ID, []Task{{ID: "implementation", Name: "Implement"}}); err != nil {
		t.Fatal(err)
	}
	developerCalls, gateCalls, reviewCalls := 0, 0, 0
	developer := func(feedback string) Worker {
		developerCalls++
		if developerCalls > 1 && feedback == "" {
			t.Fatal("fixing worker did not receive feedback")
		}
		if developerCalls == 3 && feedback != "Independent review changes_requested: add an assertion" {
			t.Fatalf("developer feedback = %q", feedback)
		}
		return workerFunc(func(context.Context, Task) error { return nil })
	}
	gates := func(context.Context) ([]GateResult, error) {
		gateCalls++
		if gateCalls == 1 {
			return []GateResult{{Name: "test", Passed: false, ExitCode: 1}}, errors.New("test failed")
		}
		return []GateResult{{Name: "test", Passed: true, ExitCode: 0}}, nil
	}
	review := func(context.Context) (ReviewResult, error) {
		reviewCalls++
		if reviewCalls == 1 {
			return ReviewResult{Verdict: ReviewChangesRequested, Findings: []string{"add an assertion"}}, nil
		}
		return ReviewResult{Verdict: ReviewApprove}, nil
	}
	err = e.RunVerified(ctx, run.ID, developer, gates, review, VerifiedOptions{MaxFixRetries: 2, Developer: "claude:developer", Reviewer: "codex:reviewer", ReviewedChange: func() (string, error) { return "sha256:diff", nil }, Scheduler: SchedulerOptions{Workers: 1}})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := e.GetRun(ctx, run.ID)
	if err != nil || saved.Status != Done {
		t.Fatalf("run = %#v, err=%v", saved, err)
	}
	if developerCalls != 3 || gateCalls != 3 || reviewCalls != 2 {
		t.Fatalf("calls developer=%d gates=%d reviewer=%d", developerCalls, gateCalls, reviewCalls)
	}
	events, err := e.Events(ctx, run.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	states, gatesSeen, reviewsSeen := []string{}, 0, 0
	for _, event := range events {
		switch event.Type {
		case "run.state":
			states = append(states, string(event.Payload))
		case "gate.finished":
			gatesSeen++
		case "review.finished":
			reviewsSeen++
			if reviewsSeen == 1 {
				var payload struct {
					Findings []string `json:"findings"`
				}
				if err := json.Unmarshal(event.Payload, &payload); err != nil || len(payload.Findings) != 1 || payload.Findings[0] != "add an assertion" {
					t.Fatalf("review findings event = %s, err=%v", event.Payload, err)
				}
			}
		}
	}
	if gatesSeen != 3 || reviewsSeen != 2 || !strings.Contains(strings.Join(states, " "), `"to":"FIXING"`) || !strings.Contains(strings.Join(states, " "), `"to":"DONE"`) {
		t.Fatalf("events gates=%d reviews=%d states=%s", gatesSeen, reviewsSeen, strings.Join(states, " "))
	}
}

func TestRunVerifiedBoundsReviewFindingsAndRetriesIndeterminate(t *testing.T) {
	e := testEngine(t)
	run, err := e.CreateRun(context.Background(), t.TempDir(), "goal")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(context.Background(), run.ID, []Task{{ID: "implementation", Name: "Implement"}}); err != nil {
		t.Fatal(err)
	}
	feedback := ""
	reviewCalls := 0
	err = e.RunVerified(context.Background(), run.ID, func(value string) Worker {
		feedback = value
		return workerFunc(func(context.Context, Task) error { return nil })
	}, func(context.Context) ([]GateResult, error) {
		return []GateResult{{Name: "test", Passed: true}}, nil
	}, func(context.Context) (ReviewResult, error) {
		reviewCalls++
		findings := make([]string, 12)
		for index := range findings {
			findings[index] = strings.Repeat("x", 301)
		}
		return ReviewResult{Verdict: ReviewIndeterminate, Findings: findings}, nil
	}, VerifiedOptions{MaxFixRetries: 1, Developer: "claude:developer", Reviewer: "codex:reviewer", ReviewedChange: func() (string, error) { return "diff", nil }})
	if err == nil || !strings.Contains(err.Error(), "indeterminate") {
		t.Fatalf("indeterminate review error = %v", err)
	}
	events, err := e.Events(context.Background(), run.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == "review.finished" {
			var payload struct {
				Verdict  string   `json:"verdict"`
				Findings []string `json:"findings"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Verdict != "indeterminate" || len(payload.Findings) != 10 || len([]rune(payload.Findings[0])) != 300 {
				t.Fatalf("bounded review payload = %#v", payload)
			}
		}
	}
	if feedback == "" || !strings.HasPrefix(feedback, "Independent review indeterminate: ") || reviewCalls != 2 {
		t.Fatal("developer factory did not receive reviewer feedback")
	}
}

func TestVerifiedRunBlocksAfterBoundedGateFailure(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	run, err := e.CreateRun(ctx, t.TempDir(), "goal")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(ctx, run.ID, []Task{{ID: "implementation", Name: "Implement"}}); err != nil {
		t.Fatal(err)
	}
	gateCalls := 0
	err = e.RunVerified(ctx, run.ID, func(string) Worker { return workerFunc(func(context.Context, Task) error { return nil }) }, func(context.Context) ([]GateResult, error) {
		gateCalls++
		return []GateResult{{Name: "test", Passed: false, ExitCode: 1}}, errors.New("test failed")
	}, func(context.Context) (ReviewResult, error) { return ReviewResult{Verdict: ReviewApprove}, nil }, VerifiedOptions{MaxFixRetries: 1, Developer: "codex:developer", Reviewer: "claude:reviewer", ReviewedChange: func() (string, error) { return "sha256:diff", nil }})
	if err == nil || gateCalls != 2 {
		t.Fatalf("error=%v gate calls=%d", err, gateCalls)
	}
	saved, err := e.GetRun(ctx, run.ID)
	if err != nil || saved.Status != Blocked || !strings.Contains(saved.Reason, "quality gates failed") {
		t.Fatalf("run = %#v, err=%v", saved, err)
	}
}

func TestParseReviewRequiresOneStructuredVerdict(t *testing.T) {
	parsed, err := ParseReview(`{"verdict":"changes_requested","findings":["add test"]}`)
	if err != nil || parsed.Verdict != ReviewChangesRequested || len(parsed.Findings) != 1 {
		t.Fatalf("parsed = %#v, err=%v", parsed, err)
	}
	parsed, err = ParseReview("```json\n{\"verdict\":\"approve\",\"findings\":[]}\n```")
	if err != nil || parsed.Verdict != ReviewApprove {
		t.Fatalf("fenced parsed = %#v, err=%v", parsed, err)
	}
	for _, invalid := range []string{`{"verdict":"approve"} trailing`, `{"verdict":"later"}`, `{"verdict":"changes_requested"}`, "approve"} {
		if _, err = ParseReview(invalid); err == nil {
			t.Errorf("accepted invalid review %q", invalid)
		}
	}
}

func TestAgentEventsStoreOnlyRedactedMetadata(t *testing.T) {
	e := testEngine(t)
	run, err := e.CreateRun(context.Background(), t.TempDir(), "goal")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.RecordAgentEventDetail(context.Background(), run.ID, "reviewer", "claude", "launch.retry", "authentication failed --api-key top-secret"); err != nil {
		t.Fatal(err)
	}
	if err = e.RecordAgentMetrics(context.Background(), run.ID, AgentMetrics{Role: "reviewer", Agent: "claude", ExitCode: 1, EventCount: 4, OutputBytes: 128, DurationMS: 12, CostUSD: 0.01, CostKnown: true, InputTokens: 20, OutputTokens: 4, FailureReason: "network/auth error --api-key top-secret"}); err != nil {
		t.Fatal(err)
	}
	if err = e.RecordChangedFiles(context.Background(), run.ID, []string{"hello.txt"}); err != nil {
		t.Fatal(err)
	}
	if err = e.RecordAgentEvent(context.Background(), run.ID, "reviewer", "claude", "secret prompt text"); err == nil {
		t.Fatal("unbounded event type was accepted")
	}
	events, err := e.Events(context.Background(), run.ID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(string(event.Payload), "secret prompt text") || strings.Contains(string(event.Payload), "top-secret") {
			t.Fatal("agent content was persisted in the event log")
		}
		if event.Type == "agent.event" && !strings.Contains(string(event.Payload), "network/auth error") && !strings.Contains(string(event.Payload), "authentication failed") {
			t.Fatalf("agent launch event detail missing: %s", event.Payload)
		}
		if event.Type == "agent.finished" && !strings.Contains(string(event.Payload), "failure_reason") {
			t.Fatalf("agent failure reason missing: %s", event.Payload)
		}
	}
}

func TestGateHelper(t *testing.T) {
	if os.Getenv("GASA_GATE_HELPER") == "" {
		return
	}
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "fail" {
		os.Exit(7)
	}
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "sleep" {
		time.Sleep(time.Second)
		return
	}
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "large" {
		_, _ = os.Stdout.Write(make([]byte, int(defaultGateOutput+1)))
		return
	}
	_, _ = os.Stdout.WriteString("safe output")
}

func TestRunGateBoundsTimeAndOutput(t *testing.T) {
	t.Setenv("GASA_GATE_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{executable, "-test.run=^TestGateHelper$"}
	timed, err := runGate(context.Background(), t.TempDir(), GateCommand{Name: "test", Argv: append(argv, "sleep"), Timeout: 20 * time.Millisecond})
	if err == nil || timed.Passed || timed.Reason != "timed out" {
		t.Fatalf("timed gate result=%#v err=%v", timed, err)
	}
	large, err := runGate(context.Background(), t.TempDir(), GateCommand{Name: "test", Argv: append(argv, "large")})
	if err == nil || large.Passed || large.Reason != "output limit exceeded" {
		t.Fatalf("large gate result=%#v err=%v", large, err)
	}
}

func TestRunGatesExecutesArgvAndRecordsEvents(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	project := t.TempDir()
	t.Setenv("GASA_GATE_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{executable, "-test.run=^TestGateHelper$"}
	run, err := e.CreateRun(ctx, project, "passing gate")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(ctx, run.ID, []Task{{ID: "a", Name: "A"}}); err != nil {
		t.Fatal(err)
	}
	err = e.RunVerified(ctx, run.ID, func(string) Worker { return workerFunc(func(context.Context, Task) error { return nil }) }, func(ctx context.Context) ([]GateResult, error) {
		return e.RunGates(ctx, run.ID, project, []GateCommand{{Name: "test", Argv: argv}})
	}, func(context.Context) (ReviewResult, error) { return ReviewResult{Verdict: ReviewApprove}, nil }, VerifiedOptions{Developer: "claude:developer", Reviewer: "codex:reviewer", ReviewedChange: func() (string, error) { return "diff", nil }})
	if err != nil {
		t.Fatal(err)
	}
	events, err := e.Events(ctx, run.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 1 {
		t.Fatal("gate events missing")
	}
	foundGateEvent := false
	for _, event := range events {
		if event.Type == "gate.finished" {
			foundGateEvent = true
		}
	}
	if !foundGateEvent {
		t.Fatal("gate result event missing")
	}
	failed, err := e.CreateRun(ctx, project, "failing gate")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.AddTasks(ctx, failed.ID, []Task{{ID: "b", Name: "B"}}); err != nil {
		t.Fatal(err)
	}
	err = e.RunVerified(ctx, failed.ID, func(string) Worker { return workerFunc(func(context.Context, Task) error { return nil }) }, func(ctx context.Context) ([]GateResult, error) {
		return e.RunGates(ctx, failed.ID, project, []GateCommand{{Name: "test", Argv: append(argv, "fail")}})
	}, func(context.Context) (ReviewResult, error) { return ReviewResult{Verdict: ReviewApprove}, nil }, VerifiedOptions{Developer: "claude:developer", Reviewer: "codex:reviewer", ReviewedChange: func() (string, error) { return "diff", nil }})
	if err == nil {
		t.Fatal("failing gate was accepted")
	}
}
