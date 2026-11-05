package office

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func coordinatorSessionFixture(t *testing.T) (*Store, CoordinatorCredential, CoordinatorAuthentication, Message) {
	t.Helper()
	s, _ := fixture(t)
	ctx := context.Background()
	a := sample()
	a.Role = "coordinator"
	a.AllowedCapabilities = []string{"read"}
	agent, err := s.CreateAgent(ctx, "project", "coordinator", a)
	if err != nil {
		t.Fatal(err)
	}
	message, err := s.SubmitMessage(ctx, "project", "message", MessageInput{Text: "Inspect main.go", RecipientKind: "coordinator"})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := s.MintCoordinatorSession(ctx, CoordinatorSessionRequest{ProjectID: "project", CoordinatorID: agent.ID, CoordinatorRevision: agent.Revision, RuntimeID: "runtime-1", ExpiresAt: time.Now().Add(time.Hour), Resources: []string{"main.go"}, Capabilities: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	auth := CoordinatorAuthentication{SessionID: credential.Session.ID, Secret: credential.Secret, Role: "coordinator", ProjectID: "project", RuntimeID: "runtime-1"}
	return s, credential, auth, message
}

func TestCoordinatorSessionHashOnlyAndRoleFencing(t *testing.T) {
	s, c, auth, _ := coordinatorSessionFixture(t)
	ctx := context.Background()
	var data string
	var hash []byte
	if err := s.store.DB().QueryRow(`SELECT secret_hash,data FROM office_coordinator_sessions WHERE id=?`, c.Session.ID).Scan(&hash, &data); err != nil {
		t.Fatal(err)
	}
	if len(hash) != 32 || strings.Contains(data, c.Secret) {
		t.Fatal("credential persisted outside hash")
	}
	if _, err := s.AuthenticateCoordinator(ctx, auth); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*CoordinatorAuthentication){func(a *CoordinatorAuthentication) { a.Role = "browser" }, func(a *CoordinatorAuthentication) { a.RuntimeID = "other-runtime" }, func(a *CoordinatorAuthentication) { a.ProjectID = "other-project" }, func(a *CoordinatorAuthentication) { a.Secret = strings.Repeat("0", 64) }} {
		invalid := auth
		change(&invalid)
		if _, err := s.AuthenticateCoordinator(ctx, invalid); !errors.Is(err, ErrCoordinatorSession) {
			t.Fatalf("unauthorized credential accepted: %v", err)
		}
	}
	if err := s.RevokeCoordinatorSession(ctx, auth.ProjectID, auth.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateCoordinator(ctx, auth); !errors.Is(err, ErrCoordinatorSession) {
		t.Fatal("revoked credential accepted")
	}
}

func TestCoordinatorSessionIngestionIsScopedAndReplayable(t *testing.T) {
	s, c, auth, message := coordinatorSessionFixture(t)
	ctx := context.Background()
	request := CoordinatorDecisionRequest{MessageID: message.ID, IntentID: message.IntentID, IntentRevision: 1, IdempotencyKey: "decision-1", Decision: Decision{Outcome: "AUTHORIZED", Reason: "Read requested file", AgentID: c.Session.CoordinatorID, Resources: []string{"main.go"}, Capabilities: []string{"read"}, ExpiresAt: time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339Nano)}}
	invalid := request
	invalid.Decision.Resources = []string{"other.go"}
	if _, err := s.IngestCoordinatorDecision(ctx, auth, invalid); !errors.Is(err, ErrValidation) {
		t.Fatalf("scope expanded: %v", err)
	}
	mismatch := request
	mismatch.MessageID = "another-message"
	if _, err := s.IngestCoordinatorDecision(ctx, auth, mismatch); !errors.Is(err, ErrNotFound) {
		t.Fatalf("message mismatch accepted: %v", err)
	}
	first, err := s.IngestCoordinatorDecision(ctx, auth, request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.IngestCoordinatorDecision(ctx, auth, request)
	if err != nil || again.Revision != first.Revision {
		t.Fatalf("replay failed: %v", err)
	}
	conflict := request
	conflict.Decision.Reason = "Changed"
	if _, err := s.IngestCoordinatorDecision(ctx, auth, conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("payload conflict accepted: %v", err)
	}
	state, err := s.Snapshot(ctx, "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Tasks) != 0 || state.Intents[0].Status != "AUTHORIZED" {
		t.Fatal("decision dispatched or was not persisted")
	}
}

func TestCoordinatorSessionRejectsChangedConfigurationAndPausedOffice(t *testing.T) {
	s, _, auth, _ := coordinatorSessionFixture(t)
	ctx := context.Background()
	state, err := s.Snapshot(ctx, "project")
	if err != nil {
		t.Fatal(err)
	}
	input := state.Agents[0].AgentInput
	input.Name = "Updated coordinator"
	if _, err = s.UpdateAgent(ctx, "project", state.Agents[0].ID, state.Agents[0].Revision, input); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateCoordinator(ctx, auth); !errors.Is(err, ErrCoordinatorSession) {
		t.Fatal("changed configuration accepted")
	}
	s, _, auth, _ = coordinatorSessionFixture(t)
	if _, err = s.Stop(ctx, "project"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateCoordinator(ctx, auth); !errors.Is(err, ErrCoordinatorSession) {
		t.Fatal("paused office accepted")
	}
}

func TestCoordinatorSessionRejectsExpiredCredential(t *testing.T) {
	s, c, auth, _ := coordinatorSessionFixture(t)
	c.Session.ExpiresAt = time.Now().Add(-time.Minute)
	// Set the durable expiry directly to represent the passage of time without sleeps.
	raw, err := json.Marshal(c.Session)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.DB().Exec(`UPDATE office_coordinator_sessions SET data=? WHERE id=?`, raw, c.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateCoordinator(context.Background(), auth); !errors.Is(err, ErrCoordinatorSession) {
		t.Fatal("expired credential accepted")
	}
}

func TestCoordinatorSessionDecisionAndReceiptRollbackTogether(t *testing.T) {
	s, c, auth, message := coordinatorSessionFixture(t)
	ctx := context.Background()
	_, err := s.store.DB().Exec(`CREATE TRIGGER reject_coordinator_event BEFORE INSERT ON office_events WHEN NEW.type='office.coordination.decided' BEGIN SELECT RAISE(ABORT,'event unavailable'); END`)
	if err != nil {
		t.Fatal(err)
	}
	request := CoordinatorDecisionRequest{MessageID: message.ID, IntentID: message.IntentID, IntentRevision: 1, IdempotencyKey: "rollback", Decision: Decision{Outcome: "AUTHORIZED", Reason: "Inspect file", AgentID: c.Session.CoordinatorID, Resources: []string{"main.go"}, Capabilities: []string{"read"}, ExpiresAt: time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339Nano)}}
	if _, err = s.IngestCoordinatorDecision(ctx, auth, request); err == nil {
		t.Fatal("event failure was ignored")
	}
	state, err := s.Snapshot(ctx, "project")
	if err != nil {
		t.Fatal(err)
	}
	if state.Intents[0].Status != "AWAITING_COORDINATOR" || state.Intents[0].Revision != 1 {
		t.Fatal("failed decision was partially committed")
	}
	var count int
	if err = s.store.DB().QueryRow(`SELECT COUNT(*) FROM office_receipts WHERE operation=?`, "coordinator.decision."+auth.SessionID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed decision retained receipt: count=%d err=%v", count, err)
	}
}
