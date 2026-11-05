package office

import "testing"

func fallbackFixture() FallbackRequest {
	return FallbackRequest{Agent: Agent{AgentInput: AgentInput{Status: "active", Primary: ExecutionChoice{ToolID: "claude"}, Fallbacks: []ExecutionChoice{{ToolID: "codex"}}, FallbackMode: "automatic_safe", AllowedCapabilities: []string{"read_project"}}}, Backend: "orca", Previous: AttemptEvidence{State: "never_started", Failure: "unavailable"}, Available: map[string]ToolAvailability{"codex": {Ready: true, Backend: "orca", ProviderFamily: "openai", Capabilities: map[string]bool{"read_project": true}}}, RequiredCapabilities: []string{"read_project"}, CoordinatorAcknowledged: true}
}

func TestFallbackNeverDuplicatesUnconfirmedAttempt(t *testing.T) {
	for _, state := range []string{"running", "unknown", ""} {
		in := fallbackFixture()
		in.Previous.State = state
		if got := SelectFallback(in); got.Status != "unknown" || got.Choice != nil {
			t.Fatalf("unsafe fallback for %q: %#v", state, got)
		}
	}
}

func TestFallbackRequiresCoordinationAndConsentForSettledEffects(t *testing.T) {
	in := fallbackFixture()
	in.CoordinatorAcknowledged = false
	if got := SelectFallback(in); got.Status != "awaiting_coordination" {
		t.Fatal(got)
	}
	in.CoordinatorAcknowledged = true
	in.Previous = AttemptEvidence{State: "settled", Failure: "provider_failed", EffectsReconciled: true}
	if got := SelectFallback(in); got.Status != "needs_approval" {
		t.Fatal(got)
	}
	in.HumanApproved = true
	if got := SelectFallback(in); got.Status != "ready" || got.Choice.ToolID != "codex" {
		t.Fatal(got)
	}
}

func TestFallbackDoesNotBypassBackendScopeReviewOrStop(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*FallbackRequest)
	}{
		{"backend", func(in *FallbackRequest) { v := in.Available["codex"]; v.Backend = "native"; in.Available["codex"] = v }},
		{"capability", func(in *FallbackRequest) { in.RequiredCapabilities = []string{"write_project"} }},
		{"review", func(in *FallbackRequest) { in.ForbiddenProviderFamilies = []string{"openai"} }},
		{"stop", func(in *FallbackRequest) { in.OfficePaused = true }},
		{"denial", func(in *FallbackRequest) { in.Previous.Failure = "permission_denied" }},
		{"quality", func(in *FallbackRequest) { in.Previous.Failure = "quality_failed" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := fallbackFixture()
			test.modify(&in)
			if got := SelectFallback(in); got.Status == "ready" {
				t.Fatalf("unsafe authorization: %#v", got)
			}
		})
	}
}

func TestFallbackNoCycleAndNoPrincipalMutation(t *testing.T) {
	in := fallbackFixture()
	in.Used = []ExecutionChoice{{ToolID: "codex"}}
	if got := SelectFallback(in); got.Status != "exhausted" {
		t.Fatal(got)
	}
	if in.Agent.Primary.ToolID != "claude" {
		t.Fatal("principal changed")
	}
}
