package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/integrations/host"
	"github.com/GasaTec/gasacli/internal/office"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
)

func (a *officeAPI) toolSnapshot(ctx context.Context, refresh bool) ([]host.Tool, time.Time) {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()
	if a.tools == nil || refresh || time.Since(a.checkedAt) > time.Minute {
		probe, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		a.tools = host.Discover(probe, discovery.Deps{})
		a.checkedAt = time.Now().UTC()
	}
	return append([]host.Tool(nil), a.tools...), a.checkedAt
}

func (a *officeAPI) installService() (*host.Installer, error) {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()
	if a.installer != nil {
		return a.installer, nil
	}
	value, err := host.New(a.home)
	if err == nil {
		a.installer = value
	}
	return value, err
}

func (a *officeAPI) validateChoices(w http.ResponseWriter, r *http.Request, in office.AgentInput, previous *office.AgentInput) bool {
	if in.Status == "draft" || in.Status == "paused" || in.Status == "archived" {
		return true
	}
	if previous != nil && previous.Status == "active" && (in.Status == "active" || in.Status == "") && reflect.DeepEqual(previous.Primary, in.Primary) && reflect.DeepEqual(previous.Fallbacks, in.Fallbacks) {
		return true
	}
	tools, _ := a.toolSnapshot(r.Context(), false)
	choices := append([]office.ExecutionChoice{in.Primary}, in.Fallbacks...)
	for _, choice := range choices {
		var found *host.Tool
		for i := range tools {
			if tools[i].ID == choice.ToolID {
				found = &tools[i]
				break
			}
		}
		if found == nil || !found.Selectable || found.Kind != "agent" {
			officeFailure(w, 409, "tool_not_ready", "Escolha uma IA pronta e compatível ou salve como rascunho.")
			return false
		}
		if (choice.Model != "" && !found.Capabilities["modelSelection"]) || (choice.Effort != "" && !found.Capabilities["effortSelection"]) {
			officeFailure(w, 409, "model_unverified", "Esta combinação ainda não comprova seleção de modelo ou esforço. Use o padrão da ferramenta.")
			return false
		}
	}
	return true
}

func (a *officeAPI) serveTools(w http.ResponseWriter, r *http.Request) bool {
	path := strings.TrimPrefix(r.URL.Path, "/api/office/")
	isTools := path == "tools" || path == "tool-discovery" || path == "tool-install-plans" || path == "tool-install-jobs" || strings.HasPrefix(path, "tool-install-jobs/")
	if !isTools {
		return false
	}
	if (path == "tools" && r.Method == http.MethodGet) || (path == "tool-discovery" && r.Method == http.MethodPost) {
		tools, checked := a.toolSnapshot(r.Context(), path == "tool-discovery")
		jsonResponse(w, map[string]any{"tools": tools, "checkedAt": checked, "hostId": host.HostID()})
		return true
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		officeFailure(w, 405, "method_not_allowed", "Método não permitido.")
		return true
	}
	var in struct {
		ProjectID      string `json:"projectId"`
		ToolID         string `json:"toolId"`
		PlanID         string `json:"planId"`
		PlanHash       string `json:"planHash"`
		Confirmation   bool   `json:"confirmation"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if r.Method == http.MethodPost {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
			officeFailure(w, 400, "invalid_request", "Plano de instalação inválido.")
			return true
		}
	} else {
		in.ProjectID = r.URL.Query().Get("projectId")
	}
	project, err := a.project(in.ProjectID)
	if err != nil {
		officeFailure(w, 403, "project_untrusted", "Selecione um projeto confiável antes de gerenciar instalações.")
		return true
	}
	installer, err := a.installService()
	if err != nil {
		officeFailure(w, 503, "installer_unavailable", "O serviço de instalação não está disponível.")
		return true
	}
	switch {
	case path == "tool-install-plans" && r.Method == http.MethodPost:
		plan, e := installer.Plan(r.Context(), in.ToolID)
		if e != nil {
			officeFailure(w, 409, "install_unavailable", "Não há método de instalação verificado para esta ferramenta neste host.")
			return true
		}
		if e = a.bindInstallPlan(r.Context(), plan.ID, in.ProjectID); e != nil {
			officeFailure(w, 503, "plan_scope_unavailable", "Não foi possível registrar o escopo do plano.")
			return true
		}
		jsonResponse(w, plan)
	case path == "tool-install-jobs" && r.Method == http.MethodPost:
		plan, e := installer.GetPlan(in.PlanID)
		if e != nil || !in.Confirmation || in.PlanHash == "" || in.PlanHash != plan.Hash {
			officeFailure(w, 400, "plan_confirmation_required", "Confirme o plano exato apresentado na tela.")
			return true
		}
		if a.checkInstallScope(r.Context(), plan.ID, in.ProjectID) != nil {
			officeFailure(w, 403, "plan_project_mismatch", "Este plano não pertence ao escritório selecionado.")
			return true
		}
		if !a.approveInstall(w, r, project.Path, plan) {
			return true
		}
		job, e := installer.Start(r.Context(), in.PlanID, in.PlanHash, in.IdempotencyKey)
		if e != nil {
			officeFailure(w, 409, "install_not_started", "A instalação não foi iniciada. Atualize o plano ou concilie o job anterior antes de tentar novamente.")
			return true
		}
		jsonResponse(w, job)
	case strings.HasPrefix(path, "tool-install-jobs/"):
		id := strings.TrimPrefix(path, "tool-install-jobs/")
		cancel := strings.HasSuffix(id, "/cancel")
		reconcile := strings.HasSuffix(id, "/reconcile")
		id = strings.TrimSuffix(id, "/cancel")
		id = strings.TrimSuffix(id, "/reconcile")
		if !officeID(id) {
			officeFailure(w, 404, "not_found", "Instalação não encontrada.")
			return true
		}
		existing, lookupErr := installer.Get(id)
		if lookupErr != nil || a.checkInstallScope(r.Context(), existing.PlanID, in.ProjectID) != nil {
			officeFailure(w, 404, "not_found", "Instalação não encontrada neste escritório.")
			return true
		}
		var job host.Job
		if cancel && r.Method == http.MethodPost {
			job, err = installer.Cancel(id)
		} else if reconcile && r.Method == http.MethodPost {
			job, err = installer.Reconcile(r.Context(), id)
		} else if !cancel && !reconcile && r.Method == http.MethodGet {
			job, err = installer.Get(id)
		} else {
			officeFailure(w, 405, "method_not_allowed", "Método não permitido.")
			return true
		}
		if err != nil {
			officeFailure(w, 409, "install_unknown", "Não foi possível confirmar o estado ou controlar esta instalação com segurança.")
			return true
		}
		jsonResponse(w, job)
	default:
		officeFailure(w, 405, "method_not_allowed", "Método não permitido.")
	}
	return true
}

func (a *officeAPI) approveInstall(w http.ResponseWriter, r *http.Request, project string, plan host.Plan) bool {
	policy := permissions.New(a.home, trust.NewStore(a.home))
	for _, action := range []permissions.Permission{"install_dependencies", "network_access"} {
		decision := policy.Decide(r.Context(), permissions.Request{Action: action, Project: project, Target: "Instalar " + plan.ToolID + " no host " + plan.HostID})
		if decision.Effect == permissions.Deny {
			officeFailure(w, 403, "install_denied", "A política não permite esta instalação.")
			return false
		}
	}
	// The exact-plan confirmation is human consent, independent of coordinator authority.
	for _, action := range []permissions.Permission{"install_dependencies", "network_access"} {
		event := permissions.ApprovalEvent{Permission: action, Decision: permissions.Allow, Reason: "Explicit office installation plan confirmation"}
		event.Actor.Kind = "human"
		event.Actor.ID = "local-user"
		event.Scope.Kind = "project"
		event.Scope.ID = project
		event.Operation.Summary = "Install " + plan.ToolID
		event.Operation.Digest = plan.Hash
		expires := plan.ExpiresAt
		event.ExpiresAt = &expires
		if err := policy.RecordApproval(event); err != nil {
			officeFailure(w, 503, "approval_not_recorded", "Não foi possível registrar o consentimento; a instalação não será iniciada.")
			return false
		}
	}
	return true
}
