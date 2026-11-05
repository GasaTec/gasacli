package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/integrations/orca"
	"github.com/GasaTec/gasacli/internal/office"
	"github.com/GasaTec/gasacli/internal/storage"
)

func (a *officeAPI) runtimeService(project projectView) (*officeRuntimeService, error) {
	a.runtimeMu.Lock()
	defer a.runtimeMu.Unlock()
	if a.runtimes == nil {
		a.runtimes = map[string]*officeRuntimeService{}
	}
	if service := a.runtimes[project.Path]; service != nil {
		return service, nil
	}
	bridge, err := orca.New(project.Path)
	if err != nil {
		return nil, err
	}
	service := &officeRuntimeService{bridge: bridge}
	a.runtimes[project.Path] = service
	return service, nil
}

func (a *officeAPI) serveRuntime(w http.ResponseWriter, r *http.Request) bool {
	path := strings.TrimPrefix(r.URL.Path, "/api/office/")
	if path != "runtime" && path != "integrations" && path != "coordinate-plan" && path != "coordinate" {
		return false
	}
	var in struct {
		ProjectID      string `json:"projectId"`
		MessageID      string `json:"messageId"`
		PlanID         string `json:"planId"`
		PlanHash       string `json:"planHash"`
		Confirmation   bool   `json:"confirmation"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if (path == "runtime" || path == "integrations") && r.Method == http.MethodGet {
		in.ProjectID = r.URL.Query().Get("projectId")
	} else if (path == "coordinate-plan" || path == "coordinate") && r.Method == http.MethodPost {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
			officeFailure(w, 400, "invalid_request", "Pedido de coordenação inválido.")
			return true
		}
		if query := r.URL.Query().Get("projectId"); query != "" && query != in.ProjectID {
			officeFailure(w, 400, "project_mismatch", "O projeto informado não corresponde à operação.")
			return true
		}
	} else {
		officeFailure(w, 405, "method_not_allowed", "Método não permitido.")
		return true
	}
	project, err := a.project(in.ProjectID)
	if err != nil {
		officeFailure(w, 403, "project_untrusted", "Selecione um projeto confiável.")
		return true
	}
	store, err := storage.Open(filepath.Join(a.home, "gasacli.db"))
	if err != nil {
		officeResult(w, nil, err)
		return true
	}
	defer store.Close()
	service, err := a.runtimeService(project)
	if err != nil {
		if r.Method == http.MethodGet {
			ops, readErr := runtimeOperations(r.Context(), store, in.ProjectID)
			if readErr != nil {
				officeResult(w, nil, readErr)
				return true
			}
			jsonResponse(w, map[string]any{"capability": orca.Capability{Reason: "Orca CLI indisponível neste host.", Capabilities: []string{}}, "operations": ops, "binding": nil, "workspaces": []any{}, "plugins": []any{}, "reason": "Orca CLI indisponível neste host."})
			return true
		}
		officeFailure(w, 409, "runtime_unavailable", "O runtime Orca precisa estar disponível antes da coordenação.")
		return true
	}
	switch path {
	case "runtime", "integrations":
		if !service.mu.TryLock() {
			binding, e := runtimeBinding(r.Context(), store, in.ProjectID)
			if e != nil {
				officeResult(w, nil, e)
				return true
			}
			ops, e := runtimeOperations(r.Context(), store, in.ProjectID)
			if e != nil {
				officeResult(w, nil, e)
				return true
			}
			jsonResponse(w, map[string]any{"capability": service.observedCapability(), "binding": binding, "operations": ops, "reason": "Operação em andamento; observação do host será atualizada após o recibo."})
			return true
		}
		defer service.mu.Unlock()
		capability, probeErr := service.probe(r.Context())
		binding, err := runtimeBinding(r.Context(), store, in.ProjectID)
		if err != nil {
			officeResult(w, nil, err)
			return true
		}
		ops, err := runtimeOperations(r.Context(), store, in.ProjectID)
		if err != nil {
			officeResult(w, nil, err)
			return true
		}
		result := map[string]any{"capability": capability, "binding": binding, "operations": ops, "checkedAt": time.Now().UTC(), "reason": capability.Reason}
		if probeErr != nil {
			result["reason"] = "Não foi possível verificar o runtime Orca."
		}
		if path == "integrations" && probeErr == nil {
			workspaces, e := service.bridge.Workspaces(r.Context())
			if e == nil {
				selector, resolveErr := orca.ResolveWorktree(workspaces, project.Path)
				if resolveErr == nil {
					result["workspaces"] = []map[string]string{{"selector": selector, "projectId": in.ProjectID}}
				} else {
					result["workspaces"] = []any{}
				}
			} else {
				result["workspaces"] = []any{}
			}
			plugins, e := service.bridge.Plugins(r.Context())
			if e == nil {
				result["plugins"] = officeSafeJSON(plugins.Result)
			} else {
				result["plugins"] = []any{}
			}
		}
		if path == "runtime" && probeErr == nil && binding != nil && binding.RuntimeID == capability.RuntimeID {
			// Raw PTY output can contain private reasoning and provider internals.
			// Secret redaction alone does not make it a public conversation. Only
			// bound structured worker transcripts may populate public chat later.
			result["conversationStatus"] = "awaiting_structured_transcript_binding"
		}
		jsonResponse(w, result)
	case "coordinate-plan":
		plan, err := a.coordinatorPlan(r.Context(), service, store, in.ProjectID, in.MessageID, project.Path)
		officeResult(w, plan, err)
	case "coordinate":
		op, replayed, err := a.coordinate(r.Context(), service, store, in.ProjectID, in.PlanID, in.PlanHash, in.IdempotencyKey, in.Confirmation, project.Path)
		if err != nil {
			officeResult(w, nil, err)
			return true
		}
		if !replayed {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusAccepted)
		}
		jsonResponse(w, op)
	}
	return true
}

func (a *officeAPI) coordinatorPlan(ctx context.Context, service *officeRuntimeService, store *storage.Store, project, messageID, path string) (officeCoordinatorPlan, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	var plan officeCoordinatorPlan
	state, err := office.New(store).Snapshot(ctx, project)
	if err != nil {
		return plan, err
	}
	if state.Settings.Paused {
		return plan, office.ErrBusy
	}
	var agent *office.Agent
	for i := range state.Agents {
		if state.Agents[i].ID == state.Settings.CoordinatorID && state.Agents[i].Status == "active" {
			agent = &state.Agents[i]
			break
		}
	}
	if agent == nil {
		return plan, office.ErrValidation
	}
	var message *office.Message
	for i := range state.Messages {
		if state.Messages[i].ID == messageID {
			message = &state.Messages[i]
			break
		}
	}
	if message == nil {
		return plan, office.ErrNotFound
	}
	capability, err := service.probe(ctx)
	if err != nil || !capability.Compatible {
		return plan, office.ErrBusy
	}
	workspaces, err := service.bridge.Workspaces(ctx)
	if err != nil {
		return plan, err
	}
	selector, err := orca.ResolveWorktree(workspaces, path)
	if err != nil {
		return plan, err
	}
	binding, err := runtimeBinding(ctx, store, project)
	if err != nil {
		return plan, err
	}
	action, terminal := "launch", ""
	executionHash, err := coordinatorExecutionHash(*agent)
	if err != nil {
		return plan, err
	}
	if binding != nil {
		if !binding.matchesCoordinator(capability.RuntimeID, *agent, executionHash) {
			return plan, office.ErrConflict
		}
		action = "send"
		terminal = binding.Terminal
	}
	if (action == "launch" && !capability.CanLaunchCoordinator) || (action == "send" && !capability.CanSubmitCoordinator) {
		return plan, office.ErrBusy
	}
	id, err := runtimeID()
	if err != nil {
		return plan, err
	}
	prompt, err := runtimePrompt(project, path, agent, message, map[string]any{"agents": state.Agents, "teams": state.Teams, "settings": state.Settings, "tasks": state.Tasks})
	if err != nil {
		return plan, err
	}
	if len(prompt) > 128*1024 {
		return plan, office.ErrValidation
	}
	plan = officeCoordinatorPlan{ID: id, ProjectID: project, MessageID: messageID, CoordinatorID: agent.ID, CoordinatorRevision: agent.Revision, SettingsRevision: state.Settings.Revision, RuntimeID: capability.RuntimeID, Worktree: selector, Action: action, Prompt: prompt, ExpiresAt: time.Now().UTC().Add(10 * time.Minute), ToolID: agent.Primary.ToolID, Model: agent.Primary.Model, Effort: agent.Primary.Effort, Terminal: terminal}
	plan.CoordinatorExecutionHash = executionHash
	plan.OfficeConfigHash, err = runtimeHash(map[string]any{"agents": state.Agents, "teams": state.Teams, "settings": state.Settings})
	if err != nil {
		return plan, err
	}
	plan.Hash, err = runtimeHash(plan)
	if err != nil {
		return plan, err
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return plan, err
	}
	_, err = store.DB().ExecContext(ctx, `INSERT INTO office_runtime_plans(id,project_id,data) VALUES(?,?,?)`, plan.ID, project, string(data))
	return plan, err
}

func runtimePlan(ctx context.Context, store *storage.Store, project, id string) (officeCoordinatorPlan, error) {
	var plan officeCoordinatorPlan
	err := store.Tx(ctx, func(tx *sql.Tx) error { return runtimeLoad(ctx, tx, "plan", project, id, &plan) })
	if errors.Is(err, sql.ErrNoRows) {
		return plan, office.ErrNotFound
	}
	return plan, err
}
