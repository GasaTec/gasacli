package office

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/storage"
)

func fixture(t *testing.T) (*Store, *storage.Store) {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db), db
}
func sample() AgentInput {
	return AgentInput{Name: "Bia", Role: "development", Avatar: Avatar{FamilyID: "capybara", Color: "teal", Eyes: "round"}, Primary: ExecutionChoice{ToolID: "codex"}}
}

func TestCreationIsAtomicIdempotentAndProjectScoped(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	in := sample()
	a, err := s.CreateAgent(ctx, "project-a", "create-1", in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateAgent(ctx, "project-a", "create-1", in)
	if err != nil || again.ID != a.ID || again.DeskID != a.DeskID {
		t.Fatalf("replay=%+v err=%v", again, err)
	}
	in.Name = "different"
	if _, err := s.CreateAgent(ctx, "project-a", "create-1", in); !errors.Is(err, ErrConflict) {
		t.Fatalf("payload mismatch: %v", err)
	}
	aState, err := s.Snapshot(ctx, "project-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(aState.Agents) != 1 || aState.Agents[0].DeskID == "" {
		t.Fatalf("inconsistent snapshot: %+v", aState)
	}
	bState, err := s.Snapshot(ctx, "project-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(bState.Agents) != 0 || bState.Cursor != 0 {
		t.Fatal("cross-project data leaked")
	}
	events, err := s.Events(ctx, "project-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Cursor != aState.Cursor {
		t.Fatalf("events=%+v snapshot cursor=%d", events, aState.Cursor)
	}
}

func TestEventFailureRollsBackIdentityAndDeskAndReceipt(t *testing.T) {
	s, db := fixture(t)
	ctx := context.Background()
	if _, err := db.DB().Exec(`CREATE TRIGGER reject_office_events BEFORE INSERT ON office_events BEGIN SELECT RAISE(ABORT,'injected event failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgent(ctx, "p", "retry", sample()); err == nil {
		t.Fatal("expected injected failure")
	}
	state, err := s.Snapshot(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Agents) != 0 {
		t.Fatal("partial identity survived rollback")
	}
	if _, err := db.DB().Exec(`DROP TRIGGER reject_office_events`); err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateAgent(ctx, "p", "retry", sample())
	if err != nil {
		t.Fatal(err)
	}
	if a.DeskID != "desk-001" {
		t.Fatalf("orphaned reservation: %+v", a)
	}
}

func TestDraftActivationAndStaleRevision(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	a, err := s.CreateAgent(ctx, "p", "draft", AgentInput{Status: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if a.DeskID != "" {
		t.Fatal("draft allocated a desk")
	}
	active, err := s.UpdateAgent(ctx, "p", a.ID, a.Revision, sample())
	if err != nil {
		t.Fatal(err)
	}
	if active.DeskID == "" || active.Revision != 2 {
		t.Fatalf("activation: %+v", active)
	}
	if _, err := s.UpdateAgent(ctx, "p", a.ID, 1, sample()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	if _, err := s.UpdateAgent(ctx, "other", a.ID, 2, sample()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project edit: %v", err)
	}
}

func TestMessagePersistsIntentionWithoutWorkerOrSyntheticReply(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	a, err := s.CreateAgent(ctx, "p", "a", sample())
	if err != nil {
		t.Fatal(err)
	}
	in := MessageInput{RecipientKind: "agent", RecipientID: a.ID, Text: "Implement login"}
	m, err := s.SubmitMessage(ctx, "p", "msg", in)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.SubmitMessage(ctx, "p", "msg", in)
	if err != nil || replay.ID != m.ID {
		t.Fatalf("message replay: %+v %v", replay, err)
	}
	state, err := s.Snapshot(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 1 || state.Messages[0].SenderKind != "user" || state.Messages[0].Status != "awaiting_coordination" || len(state.Intents) != 1 || state.Intents[0].Status != "AWAITING_COORDINATOR" || len(state.Tasks) != 0 {
		t.Fatalf("unexpected execution or reply: %+v", state)
	}
	if state.Metrics.Requests != 1 || state.Metrics.SuccessRate != nil || state.Metrics.Tokens != nil {
		t.Fatalf("invented metrics: %+v", state.Metrics)
	}
	in.Text = "new payload"
	if _, err := s.SubmitMessage(ctx, "p", "msg", in); !errors.Is(err, ErrConflict) {
		t.Fatalf("message mismatch: %v", err)
	}
}

func TestStopIsBarrierWithoutClaimingWorkerStopped(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	settings, err := s.Stop(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Paused {
		t.Fatal("missing durable pause")
	}
	second, err := s.Stop(ctx, "p")
	if err != nil || second.Revision != settings.Revision {
		t.Fatal("repeated stop mutated revision")
	}
	events, err := s.Events(ctx, "p", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "office.stop.requested" {
		t.Fatalf("false stop claim: %+v", events)
	}
}

func TestCoordinatorCannotBeSilentlyRemoved(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	in := sample()
	in.Role = "coordinator"
	a, err := s.CreateAgent(ctx, "p", "coordinator", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgent(ctx, "p", "second", in); !errors.Is(err, ErrConflict) {
		t.Fatalf("second coordinator: %v", err)
	}
	in.Status = "archived"
	if _, err := s.UpdateAgent(ctx, "p", a.ID, a.Revision, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("removed operational coordinator: %v", err)
	}
	if _, err := s.Stop(ctx, "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateAgent(ctx, "p", a.ID, a.Revision, in); err != nil {
		t.Fatal(err)
	}
}

func TestTrustedDecisionBindsOnlyExistingProjectTask(t *testing.T) {
	s, db := fixture(t)
	ctx := context.Background()
	in := sample()
	in.Role = "coordinator"
	a, err := s.CreateAgent(ctx, "opaque-project", "coordinator", in)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.SubmitMessage(ctx, "opaque-project", "message", MessageInput{Text: "Read the project"})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := s.RecordDecision(ctx, "opaque-project", m.IntentID, 1, Decision{CoordinatorID: a.ID, Outcome: "AUTHORIZED", Reason: "Within reviewed plan", AgentID: a.ID, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	engineStore, err := engine.New(db)
	if err != nil {
		t.Fatal(err)
	}
	run, err := engineStore.CreateRun(ctx, "canonical-path", "Goal")
	if err != nil {
		t.Fatal(err)
	}
	if err := engineStore.AddTasks(ctx, run.ID, []engine.Task{{ID: "native-task", Name: "Read"}}); err != nil {
		t.Fatal(err)
	}
	before, err := s.Snapshot(ctx, "opaque-project")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Tasks) != 0 {
		t.Fatal("unbound native task leaked")
	}
	if _, err := s.BindTask(ctx, "opaque-project", "wrong-path", m.IntentID, intent.Revision, "native-task"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong project path: %v", err)
	}
	if _, err := s.BindTask(ctx, "opaque-project", "canonical-path", m.IntentID, intent.Revision, "native-task"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindTask(ctx, "opaque-project", "canonical-path", m.IntentID, intent.Revision, "native-task"); err != nil {
		t.Fatalf("binding replay: %v", err)
	}
	after, err := s.Snapshot(ctx, "opaque-project")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Tasks) != 1 || after.Tasks[0].AgentID != a.ID || after.Metrics.Tasks != 1 || after.Metrics.Verified != 0 {
		t.Fatalf("projection: %+v", after)
	}
	if _, err := s.UpdateAgent(ctx, "opaque-project", a.ID, a.Revision, in); !errors.Is(err, ErrBusy) {
		t.Fatalf("unsafe live config update: %v", err)
	}
}

func TestIdentityAndReceiptSurviveReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "office.sqlite")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(db).CreateAgent(ctx, "p", "durable", sample())
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	again, err := New(db).CreateAgent(ctx, "p", "durable", sample())
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != a.ID || again.DeskID != a.DeskID {
		t.Fatal("receipt or identity lost after restart")
	}
}

func TestTeamCreationReplaysWithoutDuplicateMembership(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	a, err := s.CreateAgent(ctx, "p", "agent", sample())
	if err != nil {
		t.Fatal(err)
	}
	in := Team{Name: "Review pair", MemberIDs: []string{a.ID}, LeaderID: a.ID}
	team, err := s.SaveTeamWithKey(ctx, "p", "team-create", in)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := s.SaveTeamWithKey(ctx, "p", "team-create", in)
	if err != nil || replayed.ID != team.ID || replayed.Revision != team.Revision {
		t.Fatalf("team replay: %+v %v", replayed, err)
	}
	state, err := s.Snapshot(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Teams) != 1 || len(state.Agents[0].TeamIDs) != 1 || state.Agents[0].Revision != 2 {
		t.Fatalf("duplicate team side effects: %+v", state)
	}
	in.Name = "Different"
	if _, err := s.SaveTeamWithKey(ctx, "p", "team-create", in); !errors.Is(err, ErrConflict) {
		t.Fatalf("team payload mismatch: %v", err)
	}
}
