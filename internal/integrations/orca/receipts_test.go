package orca

import (
	"encoding/json"
	"testing"
)

func TestLaunchHandoffDoesNotClaimTurnStarted(t *testing.T) {
	receipt := Receipt{OK: true, Result: json.RawMessage(`{"operationId":"launch-one","outcome":{"kind":"terminal","handle":"term-one"},"prompt":{"delivery":"submit","outcome":"handed-to-terminal"}}`)}
	handoff, err := ParseCoordinatorReceipt(receipt, "launch")
	if err != nil {
		t.Fatal(err)
	}
	if !handoff.Accepted || handoff.DeliveryStatus != "handed-to-terminal" || handoff.OperationID != "launch-one" || handoff.Terminal != "term-one" {
		t.Fatal("incorrect public launch receipt", handoff)
	}
}
func TestCoordinatorSubmissionPreservesAcceptanceStage(t *testing.T) {
	receipt := Receipt{OK: true, Result: json.RawMessage(`{"send":{"handle":"term-one","accepted":true,"prompt":{"requestId":"prompt-one","provider":"codex","stages":["input_accepted"]}}}`)}
	handoff, err := ParseCoordinatorReceipt(receipt, "send")
	if err != nil {
		t.Fatal(err)
	}
	if !handoff.Accepted || handoff.DeliveryStatus != "input_accepted" {
		t.Fatal("input acceptance was promoted to a turn", handoff)
	}
	receipt.Unknown = true
	handoff, err = ParseCoordinatorReceipt(receipt, "send")
	if err == nil || handoff.Accepted || handoff.OperationID != "prompt-one" {
		t.Fatal("unknown outcome lost recovery reference or was accepted", handoff, err)
	}
}
func TestWorkspaceResolutionRejectsIncompleteScope(t *testing.T) {
	receipt := Receipt{OK: true, RuntimeID: "runtime-one", Result: json.RawMessage(`{"worktrees":[],"truncated":true,"hostScope":{"hostIds":["local"],"omittedHostIds":[]}}`)}
	if _, err := ResolveWorktree(receipt, t.TempDir()); err == nil {
		t.Fatal("truncated inventory used as proof")
	}
}
