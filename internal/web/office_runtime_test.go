package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/office"
	"github.com/GasaTec/gasacli/internal/storage"
)

func runtimeFixture(t *testing.T) *storage.Store {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestCoordinatorBindingSurvivesPresentationChangesOnly(t *testing.T) {
	agent := office.Agent{ID: "coordinator", Revision: 1, AgentInput: office.AgentInput{
		Name: "Lia", Status: "active", Role: "coordinator",
		Primary: office.ExecutionChoice{ToolID: "claude"},
		Avatar:  office.Avatar{FamilyID: "cat"},
	}}
	hash, err := coordinatorExecutionHash(agent)
	if err != nil {
		t.Fatal(err)
	}
	binding := officeRuntimeBinding{RuntimeID: "runtime", CoordinatorID: agent.ID, CoordinatorRevision: 1, CoordinatorExecutionHash: hash}
	appearance := agent
	appearance.Revision = 2
	appearance.Name = "Outro nome"
	appearance.Avatar = office.Avatar{FamilyID: "capybara", Color: "blue"}
	appearance.DeskID = "desk-2"
	changedHash, err := coordinatorExecutionHash(appearance)
	if err != nil {
		t.Fatal(err)
	}
	if !binding.matchesCoordinator("runtime", appearance, changedHash) {
		t.Fatal("presentation edit invalidated execution binding")
	}
	mutations := map[string]func(*office.Agent){
		"provider":        func(a *office.Agent) { a.Primary.ToolID = "codex" },
		"model":           func(a *office.Agent) { a.Primary.Model = "changed" },
		"effort":          func(a *office.Agent) { a.Primary.Effort = "changed" },
		"status":          func(a *office.Agent) { a.Status = "paused" },
		"role":            func(a *office.Agent) { a.Role = "developer" },
		"instructions":    func(a *office.Agent) { a.Instructions = "changed" },
		"routing":         func(a *office.Agent) { a.WhenToCall = "changed" },
		"fallback":        func(a *office.Agent) { a.Fallbacks = []office.ExecutionChoice{{ToolID: "codex"}} },
		"fallback policy": func(a *office.Agent) { a.FallbackMode = "automatic_safe" },
		"capabilities":    func(a *office.Agent) { a.AllowedCapabilities = []string{"edit"} },
		"teams":           func(a *office.Agent) { a.TeamIDs = []string{"team"} },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := agent
			mutate(&changed)
			digest, err := coordinatorExecutionHash(changed)
			if err != nil {
				t.Fatal(err)
			}
			if binding.matchesCoordinator("runtime", changed, digest) {
				t.Fatal("changed authority retained binding")
			}
		})
	}
	if binding.matchesCoordinator("other-runtime", agent, hash) {
		t.Fatal("runtime switch accepted")
	}
	other := agent
	other.ID = "another-coordinator"
	if binding.matchesCoordinator("runtime", other, hash) {
		t.Fatal("coordinator switch accepted")
	}
	binding.CoordinatorExecutionHash = ""
	if binding.matchesCoordinator("runtime", agent, hash) {
		t.Fatal("legacy binding guessed compatible")
	}
}

func TestCoordinatorReceiptReplayDoesNotContactUnavailableHost(t *testing.T) {
	db := runtimeFixture(t)
	ctx := context.Background()
	op := officeRuntimeOperation{ID: "operation-local", ProjectID: "p", MessageID: "message", PlanID: "expired-plan", Status: "accepted", Action: "launch", Terminal: "known-terminal", CreatedAt: time.Now().UTC()}
	hash, err := runtimeHash(map[string]any{"planId": op.PlanID, "planHash": "original-hash", "confirmation": true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO office_runtime_operations(id,project_id,message_id,key,request_hash,status,data) VALUES(?,?,?,?,?,?,?)`, op.ID, op.ProjectID, op.MessageID, "same-key", hash, op.Status, string(data)); err != nil {
		t.Fatal(err)
	}
	// There is deliberately no bridge or current plan. A receipt replay must
	// not probe, launch, submit another prompt, or depend on current readiness.
	a := &officeAPI{}
	got, replayed, err := a.coordinate(ctx, &officeRuntimeService{}, db, "p", op.PlanID, "original-hash", "same-key", true, "unused")
	if err != nil || !replayed || got.ID != op.ID || got.Terminal != op.Terminal {
		t.Fatalf("receipt replay: %+v replay=%v err=%v", got, replayed, err)
	}
	if _, _, err := a.coordinate(ctx, &officeRuntimeService{}, db, "p", op.PlanID, "changed-hash", "same-key", true, "unused"); !errors.Is(err, office.ErrConflict) {
		t.Fatalf("changed payload: %v", err)
	}
}

func TestRuntimePlansBindingsAndOperationsStayProjectScoped(t *testing.T) {
	db := runtimeFixture(t)
	ctx := context.Background()
	plan := officeCoordinatorPlan{ID: "plan", ProjectID: "p", MessageID: "private-message", Prompt: "private-scope"}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO office_runtime_plans(id,project_id,data) VALUES(?,?,?)`, plan.ID, plan.ProjectID, string(data)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimePlan(ctx, db, "other", plan.ID); !errors.Is(err, office.ErrNotFound) {
		t.Fatalf("cross-project plan: %v", err)
	}
	if err := db.Tx(ctx, func(tx *sql.Tx) error {
		var b officeRuntimeBinding
		return runtimeLoad(ctx, tx, "binding", "other", "", &b)
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unexpected binding: %v", err)
	}
	ops, err := runtimeOperations(ctx, db, "other")
	if err != nil || len(ops) != 0 {
		t.Fatalf("cross-project operations: %+v %v", ops, err)
	}
}
