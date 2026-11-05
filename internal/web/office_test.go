package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/integrations/host"
	"github.com/GasaTec/gasacli/internal/office"
	"github.com/GasaTec/gasacli/internal/storage"
	"github.com/GasaTec/gasacli/internal/trust"
)

// These tests use recorders and disposable files only. They are written here for
// the authorized test phase; they do not start listeners or provider processes.
func officeFixture(t *testing.T) (*officeAPI, string, string) {
	t.Helper()
	home, project := t.TempDir(), t.TempDir()
	if err := trust.NewStore(home).Grant(project); err != nil {
		t.Fatal(err)
	}
	view := projectView{ID: "project-qa", Name: "QA", Path: project, Trusted: true}
	api := &officeAPI{home: home, projects: func() snapshot {
		return snapshot{projects: []projectView{view}, byID: map[string]projectView{view.ID: view}}
	}, tools: []host.Tool{}, checkedAt: time.Now()}
	return api, view.ID, project
}

func officeRequest(t *testing.T, a *officeAPI, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	w := httptest.NewRecorder()
	if !a.serve(w, r) {
		t.Fatal("office route was not handled")
	}
	return w
}

func TestOfficeDraftCreationDoesNotNeedProviderAndDeduplicates(t *testing.T) {
	a, project, _ := officeFixture(t)
	in := officeMutation{ProjectID: project, IdempotencyKey: "create-qa", AgentInput: office.AgentInput{Status: "draft", Name: "Capivara", Avatar: office.Avatar{FamilyID: "capybara"}}}
	first := officeRequest(t, a, http.MethodPost, "/api/office/agents", in)
	if first.Code != 200 {
		t.Fatalf("create: %d %s", first.Code, first.Body.String())
	}
	second := officeRequest(t, a, http.MethodPost, "/api/office/agents", in)
	if second.Code != 200 || second.Body.String() != first.Body.String() {
		t.Fatal("creation replay changed the response")
	}
	response := officeRequest(t, a, http.MethodGet, "/api/office/snapshot?projectId="+project, nil)
	var state office.Snapshot
	if json.Unmarshal(response.Body.Bytes(), &state) != nil || len(state.Agents) != 1 || state.Agents[0].DeskID != "" || len(state.Tasks) != 0 {
		t.Fatal("draft created a desk/task or duplicated identity")
	}
}

func TestOfficeRevokedTrustBlocksStoredIdentityAccess(t *testing.T) {
	a, project, path := officeFixture(t)
	if err := trust.NewStore(a.home).Revoke(path); err != nil {
		t.Fatal(err)
	}
	w := officeRequest(t, a, http.MethodGet, "/api/office/snapshot?projectId="+project, nil)
	if w.Code != 403 {
		t.Fatalf("revoked project status=%d", w.Code)
	}
}

func TestOfficeActivationRevalidatesToolButExistingIdentitySurvivesOutage(t *testing.T) {
	a, _, _ := officeFixture(t)
	in := office.AgentInput{Status: "active", Primary: office.ExecutionChoice{ToolID: "codex"}}
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	draft := in
	draft.Status = "draft"
	if a.validateChoices(httptest.NewRecorder(), r, in, &draft) {
		t.Fatal("draft activation bypassed readiness")
	}
	active := in
	if !a.validateChoices(httptest.NewRecorder(), r, in, &active) {
		t.Fatal("unavailable existing identity could not be preserved")
	}
}

func TestOfficeCreateReceiptReplaysAfterToolBecomesUnavailable(t *testing.T) {
	a, project, _ := officeFixture(t)
	in := office.AgentInput{Status: "active", Name: "QA", Role: "developer", Avatar: office.Avatar{FamilyID: "owl"}, Primary: office.ExecutionChoice{ToolID: "codex"}}
	store, err := storage.Open(filepath.Join(a.home, "gasacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := office.New(store).CreateAgent(context.Background(), project, "accepted-before-outage", in)
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	w := officeRequest(t, a, http.MethodPost, "/api/office/agents", officeMutation{ProjectID: project, IdempotencyKey: "accepted-before-outage", AgentInput: in})
	var replay office.Agent
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &replay) != nil || replay.ID != created.ID {
		t.Fatalf("lost accepted receipt after provider outage: %d", w.Code)
	}
}

func TestOfficeClientCannotRecordCoordinatorDecision(t *testing.T) {
	a, project, _ := officeFixture(t)
	w := officeRequest(t, a, http.MethodPost, "/api/office/intents/intent-qa/decision", map[string]any{"projectId": project, "outcome": "AUTHORIZED"})
	if w.Code != 404 {
		t.Fatalf("browser coordinator impersonation route status=%d", w.Code)
	}
}

func TestOfficeInstallPlanCannotMoveBetweenProjects(t *testing.T) {
	a, project, _ := officeFixture(t)
	if err := a.bindInstallPlan(context.Background(), "plan-qa", project); err != nil {
		t.Fatal(err)
	}
	if err := a.checkInstallScope(context.Background(), "plan-qa", "another-trusted-project"); err == nil {
		t.Fatal("another project acquired installation consent")
	}
}
