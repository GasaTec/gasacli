package office

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PendingIntents is a durable coordinator inbox. Reading it does not acknowledge
// or authorize a request. Decisions must match the current revision.
func (s *Store) PendingIntents(ctx context.Context, project string) ([]Intent, error) {
	out := []Intent{}
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		state, err := load(ctx, tx, project)
		if err != nil {
			return err
		}
		for _, intent := range state.Intents {
			if intent.Status == "AWAITING_COORDINATOR" || intent.Status == "DEFERRED" {
				out = append(out, intent)
			}
		}
		return nil
	})
	return out, err
}

// RecordDecision persists a coordinator conclusion but does not dispatch a
// worker, bypass human approval, or assert enforcement by a provider.
func (s *Store) RecordDecision(ctx context.Context, project, intentID string, revision int, decision Decision) (Intent, error) {
	var out Intent
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		return recordDecision(ctx, tx, project, intentID, revision, decision, &out)
	})
	return out, err
}

func recordDecision(ctx context.Context, tx *sql.Tx, project, intentID string, revision int, decision Decision, result *Intent) error {
	var out Intent
	state, err := load(ctx, tx, project)
	if err != nil {
		return err
	}
	if state.Settings.Paused {
		return ErrBusy
	}
	if decision.CoordinatorID == "" || state.Settings.CoordinatorID != decision.CoordinatorID {
		return fmt.Errorf("%w: coordinator authority changed", ErrConflict)
	}
	idx := agentIndex(state, decision.CoordinatorID)
	if idx < 0 || state.Agents[idx].Status != "active" {
		return fmt.Errorf("%w: coordinator unavailable", ErrConflict)
	}
	decision.CoordinatorRevision = state.Agents[idx].Revision
	if decision.Reason == "" || len(decision.Reason) > 16384 {
		return fmt.Errorf("%w: decision needs a bounded reason", ErrValidation)
	}
	switch decision.Outcome {
	case "AUTHORIZED":
		idx := agentIndex(state, decision.AgentID)
		if idx < 0 || state.Agents[idx].Status != "active" {
			return fmt.Errorf("%w: execution agent unavailable", ErrValidation)
		}
		decision.AgentRevision = state.Agents[idx].Revision
		deadline, err := time.Parse(time.RFC3339Nano, decision.ExpiresAt)
		if err != nil || !deadline.After(time.Now()) {
			return fmt.Errorf("%w: authorization needs a future expiry", ErrValidation)
		}
		allowed := map[string]bool{}
		for _, c := range state.Agents[idx].AllowedCapabilities {
			allowed[c] = true
		}
		for _, c := range decision.Capabilities {
			if !allowed[c] {
				return fmt.Errorf("%w: decision exceeds agent capabilities", ErrValidation)
			}
		}
	case "DEFERRED", "DENIED":
	default:
		return fmt.Errorf("%w: invalid coordinator outcome", ErrValidation)
	}
	for i, intent := range state.Intents {
		if intent.ID != intentID {
			continue
		}
		if intent.Revision != revision {
			return ErrConflict
		}
		if intent.Status != "AWAITING_COORDINATOR" && intent.Status != "DEFERRED" {
			return ErrConflict
		}
		intent.Status = decision.Outcome
		intent.Revision++
		intent.Decision = &decision
		state.Intents[i] = intent
		out = intent
		*result = out
		if _, err := tx.ExecContext(ctx, `UPDATE office_outbox SET status=? WHERE project_id=? AND intent_id=?`, decision.Outcome, project, intentID); err != nil {
			return err
		}
		if err := save(ctx, tx, state); err != nil {
			return err
		}
		return event(ctx, tx, project, "office.coordination.decided", out)
	}
	return ErrNotFound
}

// BindTask associates an existing engine task with a previously authorized
// office intent. projectPath must come from the trusted project registry, never
// from an arbitrary browser payload. It is not a dispatch API: the integration
// must separately enforce policy, approval, scope reservation and host support.
func (s *Store) BindTask(ctx context.Context, project, projectPath, intentID string, revision int, taskID string) (Task, error) {
	var out Task
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		state, err := load(ctx, tx, project)
		if err != nil {
			return err
		}
		if state.Settings.Paused {
			return ErrBusy
		}
		var intent *Intent
		for i := range state.Intents {
			if state.Intents[i].ID == intentID {
				intent = &state.Intents[i]
				break
			}
		}
		if intent == nil {
			return ErrNotFound
		}
		if intent.Revision != revision || intent.Status != "AUTHORIZED" || intent.Decision == nil {
			return ErrConflict
		}
		coordinatorIndex := agentIndex(state, intent.Decision.CoordinatorID)
		agentIdx := agentIndex(state, intent.Decision.AgentID)
		if state.Settings.CoordinatorID != intent.Decision.CoordinatorID || coordinatorIndex < 0 || agentIdx < 0 || state.Agents[coordinatorIndex].Revision != intent.Decision.CoordinatorRevision || state.Agents[agentIdx].Revision != intent.Decision.AgentRevision || state.Agents[agentIdx].Status != "active" {
			return fmt.Errorf("%w: authorized configuration has changed", ErrConflict)
		}
		deadline, err := time.Parse(time.RFC3339Nano, intent.Decision.ExpiresAt)
		if err != nil || !deadline.After(time.Now()) {
			return fmt.Errorf("%w: authorization expired", ErrConflict)
		}
		if projectPath == "" {
			return fmt.Errorf("%w: canonical project path required", ErrValidation)
		}
		if err := tx.QueryRowContext(ctx, `SELECT t.id,t.run_id,t.name,t.status FROM tasks t JOIN runs r ON r.id=t.run_id WHERE t.id=? AND r.project=?`, taskID, projectPath).Scan(&out.ID, &out.RunID, &out.Title, &out.Status); err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return err
		}
		out.AgentID = intent.Decision.AgentID
		var boundProject, boundIntent, boundAgent string
		err = tx.QueryRowContext(ctx, `SELECT project_id,intent_id,agent_id FROM office_task_bindings WHERE task_id=?`, taskID).Scan(&boundProject, &boundIntent, &boundAgent)
		if err == nil {
			if boundProject == project && boundIntent == intentID && boundAgent == out.AgentID {
				return nil
			}
			return fmt.Errorf("%w: engine task is already associated", ErrConflict)
		}
		if err != sql.ErrNoRows {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO office_task_bindings(project_id,intent_id,task_id,agent_id) VALUES(?,?,?,?)`, project, intentID, taskID, out.AgentID); err != nil {
			return err
		}
		return event(ctx, tx, project, "office.task.bound", out)
	})
	return out, err
}
