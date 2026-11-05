package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/integrations/orca"
	"github.com/GasaTec/gasacli/internal/office"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/storage"
	"github.com/GasaTec/gasacli/internal/trust"
)

func (a *officeAPI) coordinate(ctx context.Context, service *officeRuntimeService, store *storage.Store, project, planID, planHash, key string, confirmation bool, path string) (officeRuntimeOperation, bool, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	var op officeRuntimeOperation
	if strings.TrimSpace(key) == "" || len(key) > 200 {
		return op, false, office.ErrValidation
	}
	requestHash, err := runtimeHash(map[string]any{"planId": planID, "planHash": planHash, "confirmation": confirmation})
	if err != nil {
		return op, false, err
	}
	// A receipt is immutable evidence of the original submit even after plan
	// expiry or a later provider outage. Conflicting payloads never replay it.
	replayed := false
	err = store.Tx(ctx, func(tx *sql.Tx) error {
		var hash, data string
		err := tx.QueryRowContext(ctx, `SELECT request_hash,data FROM office_runtime_operations WHERE project_id=? AND key=?`, project, key).Scan(&hash, &data)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if hash != requestHash {
			return office.ErrConflict
		}
		replayed = true
		return json.Unmarshal([]byte(data), &op)
	})
	if err != nil || replayed {
		return op, replayed, err
	}
	plan, err := runtimePlan(ctx, store, project, planID)
	if err != nil {
		return op, false, err
	}
	if !confirmation || plan.Hash != planHash || !plan.ExpiresAt.After(time.Now()) {
		return op, false, office.ErrConflict
	}
	if err := a.runtimePreflight(ctx, store, plan, path, false); err != nil {
		return op, false, err
	}
	id, err := runtimeID()
	if err != nil {
		return op, false, err
	}
	stamp := time.Now().UTC()
	op = officeRuntimeOperation{ID: id, ProjectID: project, MessageID: plan.MessageID, PlanID: plan.ID, Status: "unknown", Action: plan.Action, CreatedAt: stamp, UpdatedAt: stamp, Reason: "Handoff reservado; resultado exige recibo do runtime."}
	err = store.Tx(ctx, func(tx *sql.Tx) error {
		var pending int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM office_runtime_operations WHERE project_id=? AND (status='unknown' OR (message_id=? AND status='accepted'))`, project, plan.MessageID).Scan(&pending); err != nil {
			return err
		}
		if pending > 0 {
			return office.ErrConflict
		}
		var binding officeRuntimeBinding
		bindingErr := runtimeLoad(ctx, tx, "binding", project, "", &binding)
		if plan.Action == "launch" {
			if bindingErr == nil {
				return office.ErrConflict
			}
			if !errors.Is(bindingErr, sql.ErrNoRows) {
				return bindingErr
			}
		} else {
			if bindingErr != nil {
				return office.ErrConflict
			}
			if plan.CoordinatorExecutionHash == "" || binding.Terminal != plan.Terminal || binding.RuntimeID != plan.RuntimeID || binding.CoordinatorID != plan.CoordinatorID || binding.CoordinatorExecutionHash != plan.CoordinatorExecutionHash {
				return office.ErrConflict
			}
		}
		data, err := json.Marshal(op)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO office_runtime_operations(id,project_id,message_id,key,request_hash,status,data) VALUES(?,?,?,?,?,?,?)`, op.ID, project, plan.MessageID, key, requestHash, op.Status, string(data))
		return err
	})
	if err != nil {
		return op, false, err
	}
	// The browser owns observation, not the operation's cancellation context.
	// A crash between this durable fence and the final receipt remains unknown.
	go a.runCoordinator(service, plan, op, path)
	op.Status = "submitting"
	return op, false, nil
}

func (a *officeAPI) runtimePreflight(ctx context.Context, store *storage.Store, plan officeCoordinatorPlan, path string, audit bool) error {
	if !plan.ExpiresAt.After(time.Now()) {
		return office.ErrConflict
	}
	project, err := a.project(plan.ProjectID)
	if err != nil || project.Path != path {
		return office.ErrConflict
	}
	state, err := office.New(store).Snapshot(ctx, plan.ProjectID)
	if err != nil {
		return err
	}
	if state.Settings.Paused {
		return office.ErrBusy
	}
	if state.Settings.CoordinatorID != plan.CoordinatorID || state.Settings.Revision != plan.SettingsRevision {
		return office.ErrConflict
	}
	configHash, err := runtimeHash(map[string]any{"agents": state.Agents, "teams": state.Teams, "settings": state.Settings})
	if err != nil {
		return err
	}
	if configHash != plan.OfficeConfigHash {
		return office.ErrConflict
	}
	found := false
	for _, agent := range state.Agents {
		if agent.ID == plan.CoordinatorID && agent.Revision == plan.CoordinatorRevision && agent.Status == "active" && agent.Primary.ToolID == plan.ToolID && agent.Primary.Model == plan.Model && agent.Primary.Effort == plan.Effort {
			executionHash, err := coordinatorExecutionHash(agent)
			if err != nil {
				return err
			}
			if executionHash == "" || executionHash != plan.CoordinatorExecutionHash {
				return office.ErrConflict
			}
			found = true
		}
	}
	if !found {
		return office.ErrConflict
	}
	policy := permissions.New(a.home, trust.NewStore(a.home))
	for _, action := range []permissions.Permission{"agent_run", "execute_commands", "network_access"} {
		decision := policy.Decide(ctx, permissions.Request{Action: action, Agent: plan.ToolID, Project: path, Target: "Coordenar pedido persistido " + plan.MessageID})
		if decision.Effect == permissions.Deny {
			return office.ErrValidation
		}
		if audit {
			event := permissions.ApprovalEvent{Permission: action, Decision: permissions.Allow, Reason: "Explicit confirmation of exact coordinator handoff plan"}
			event.Actor.Kind = "human"
			event.Actor.ID = "local-user"
			event.Scope.Kind = "project"
			event.Scope.ID = path
			event.Requester.AgentID = plan.CoordinatorID
			event.Operation.Summary = "Coordinator " + plan.Action
			event.Operation.Digest = plan.Hash
			expiry := plan.ExpiresAt
			event.ExpiresAt = &expiry
			if err := policy.RecordApproval(event); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *officeAPI) runCoordinator(service *officeRuntimeService, plan officeCoordinatorPlan, op officeRuntimeOperation, path string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	store, err := storage.Open(filepath.Join(a.home, "gasacli.db"))
	if err != nil {
		return
	}
	defer store.Close()
	finish := func(receipt orca.Receipt, mutationErr error) {
		op.UpdatedAt = time.Now().UTC()
		handoff, parseErr := orca.ParseCoordinatorReceipt(receipt, plan.Action)
		op.Terminal = handoff.Terminal
		op.OperationID = handoff.OperationID
		op.DeliveryStatus = handoff.DeliveryStatus
		terminalMatches := plan.Action == "launch" || handoff.Terminal == plan.Terminal
		if mutationErr == nil && parseErr == nil && handoff.Accepted && !receipt.Unknown && receipt.RuntimeID == plan.RuntimeID && terminalMatches {
			op.Status = "accepted"
			op.Terminal = handoff.Terminal
			op.OperationID = handoff.OperationID
			op.DeliveryStatus = handoff.DeliveryStatus
			op.Reason = "Handoff observado; decisão do coordenador e execução continuam separadas."
		} else if !receipt.Process.Started && !receipt.Unknown {
			op.Status = "rejected"
			op.Reason = "Handoff não iniciado; configuração ou capacidade requer revisão."
		} else {
			op.Status = "unknown"
			op.Reason = "Resultado ambíguo. Concilie o recibo original antes de qualquer novo envio."
		}
		// Use a fresh bounded persistence context even if the provider timed out.
		persist, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer persistCancel()
		_ = store.Tx(persist, func(tx *sql.Tx) error {
			if op.Status == "accepted" && plan.Action == "launch" {
				binding := officeRuntimeBinding{RuntimeID: plan.RuntimeID, CoordinatorID: plan.CoordinatorID, CoordinatorRevision: plan.CoordinatorRevision, CoordinatorExecutionHash: plan.CoordinatorExecutionHash, Terminal: op.Terminal}
				data, err := json.Marshal(binding)
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(persist, `INSERT INTO office_runtime_bindings(project_id,data) VALUES(?,?)`, plan.ProjectID, string(data)); err != nil {
					return err
				}
			}
			if err := runtimeSaveOperation(persist, tx, op); err != nil {
				return err
			}
			raw, err := json.Marshal(receipt)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(persist, `UPDATE office_runtime_operations SET receipt=? WHERE project_id=? AND id=?`, string(raw), plan.ProjectID, op.ID)
			return err
		})
	}
	if err := a.runtimePreflight(ctx, store, plan, path, true); err != nil {
		finish(orca.Receipt{}, err)
		return
	}
	capability, err := service.probe(ctx)
	if err != nil || capability.RuntimeID != plan.RuntimeID {
		finish(orca.Receipt{}, office.ErrConflict)
		return
	}
	workspaces, err := service.bridge.Workspaces(ctx)
	if err != nil {
		finish(orca.Receipt{}, err)
		return
	}
	selector, err := orca.ResolveWorktree(workspaces, path)
	if err != nil || selector != plan.Worktree {
		finish(orca.Receipt{}, office.ErrConflict)
		return
	}
	var receipt orca.Receipt
	if plan.Action == "launch" {
		receipt, err = service.bridge.LaunchCoordinator(ctx, orca.LaunchRequest{AgentID: plan.ToolID, Worktree: plan.Worktree, Prompt: plan.Prompt, Model: plan.Model, Effort: plan.Effort})
	} else {
		if _, err = service.bridge.Terminal(ctx, plan.Terminal, ""); err != nil {
			finish(orca.Receipt{}, err)
			return
		}
		receipt, err = service.bridge.SendCoordinator(ctx, plan.Terminal, plan.Prompt, "")
	}
	finish(receipt, err)
}
