package office

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"
)

func (s *Store) SaveTeam(ctx context.Context, project string, input Team) (Team, error) {
	return s.saveTeam(ctx, project, "", input)
}

// SaveTeamWithKey creates a team and receipt atomically. Repeating the same
// request after a lost response returns the original team without new members
// or events. Existing teams continue to use revision-controlled SaveTeam.
func (s *Store) SaveTeamWithKey(ctx context.Context, project, key string, input Team) (Team, error) {
	if input.ID != "" || input.Revision != 0 || strings.TrimSpace(key) == "" {
		return Team{}, fmt.Errorf("%w: keyed team creation needs a key and no existing identity", ErrValidation)
	}
	return s.saveTeam(ctx, project, key, input)
}

func (s *Store) saveTeam(ctx context.Context, project, key string, input Team) (Team, error) {
	var out Team
	request := input
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		if key != "" {
			ok, err := replay(ctx, tx, project, "team.create", key, request, &out)
			if err != nil || ok {
				return err
			}
		}
		state, err := load(ctx, tx, project)
		if err != nil {
			return err
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || utf8.RuneCountInString(input.Name) > 60 || utf8.RuneCountInString(input.Purpose) > 300 || len(input.MemberIDs) > 3 {
			return fmt.Errorf("%w: team needs a name and up to three members", ErrValidation)
		}
		seen := map[string]bool{}
		for _, id := range input.MemberIDs {
			if seen[id] || agentIndex(state, id) < 0 {
				return fmt.Errorf("%w: duplicate or unknown team member", ErrValidation)
			}
			seen[id] = true
		}
		if input.LeaderID != "" && !seen[input.LeaderID] {
			return fmt.Errorf("%w: team leader must be a member", ErrValidation)
		}
		idx := -1
		for i, t := range state.Teams {
			if t.ID == input.ID {
				idx = i
				break
			}
		}
		if input.ID == "" {
			if input.Revision != 0 {
				return ErrConflict
			}
			input.ID, err = newID("team-")
			if err != nil {
				return err
			}
			input.Revision = 1
		} else {
			if idx < 0 {
				return ErrNotFound
			}
			if state.Teams[idx].Revision != input.Revision {
				return ErrConflict
			}
			busy, err := projectHasWork(ctx, tx, project)
			if err != nil {
				return err
			}
			if busy {
				return ErrBusy
			}
			input.Revision++
		}
		if input.MemberIDs == nil {
			input.MemberIDs = []string{}
		}
		for i, a := range state.Agents {
			ids := []string{}
			had := false
			for _, id := range a.TeamIDs {
				if id == input.ID {
					had = true
				} else {
					ids = append(ids, id)
				}
			}
			if seen[a.ID] {
				ids = append(ids, input.ID)
			}
			if had != seen[a.ID] {
				state.Agents[i].TeamIDs = ids
				state.Agents[i].Revision++
			}
		}
		if idx < 0 {
			state.Teams = append(state.Teams, input)
		} else {
			state.Teams[idx] = input
		}
		out = input
		if err := save(ctx, tx, state); err != nil {
			return err
		}
		if err := event(ctx, tx, project, "office.team.saved", out); err != nil {
			return err
		}
		if key != "" {
			return receipt(ctx, tx, project, "team.create", key, request, out)
		}
		return nil
	})
	return out, err
}

func (s *Store) SaveSettings(ctx context.Context, project string, input Settings) (Settings, error) {
	if input.Autonomy == "" {
		input.Autonomy = "accepted_plan"
	}
	if input.Autonomy != "ask_all" && input.Autonomy != "accepted_plan" && input.Autonomy != "scoped" {
		return Settings{}, fmt.Errorf("%w: invalid office autonomy", ErrValidation)
	}
	var out Settings
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		state, err := load(ctx, tx, project)
		if err != nil {
			return err
		}
		if state.Settings.Revision != input.Revision {
			return ErrConflict
		}
		if input.Quality != "low" && input.Quality != "balanced" && input.Quality != "high" {
			return fmt.Errorf("%w: invalid visual quality", ErrValidation)
		}
		if input.CoordinatorID != "" {
			idx := agentIndex(state, input.CoordinatorID)
			if idx < 0 || state.Agents[idx].Status != "active" {
				return fmt.Errorf("%w: coordinator must be active", ErrValidation)
			}
		} else if state.Settings.CoordinatorID != "" && !input.Paused {
			return fmt.Errorf("%w: pause office before clearing coordinator", ErrConflict)
		}
		if input.CoordinatorID != state.Settings.CoordinatorID || (!input.Paused && state.Settings.Paused) {
			busy, err := projectHasWork(ctx, tx, project)
			if err != nil {
				return err
			}
			if busy {
				return ErrBusy
			}
		}
		input.Revision++
		out = input
		state.Settings = out
		if err := save(ctx, tx, state); err != nil {
			return err
		}
		return event(ctx, tx, project, "office.settings.updated", out)
	})
	return out, err
}

func (s *Store) SubmitMessage(ctx context.Context, project, key string, input MessageInput) (Message, error) {
	var out Message
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		ok, err := replay(ctx, tx, project, "message.submit", key, input, &out)
		if err != nil || ok {
			return err
		}
		in := input
		in.Text = strings.TrimSpace(in.Text)
		if in.Text == "" || len(in.Text) > 16384 {
			return fmt.Errorf("%w: message must contain 1 to 16384 bytes", ErrValidation)
		}
		if in.RecipientKind == "" {
			if in.RecipientID != "" && in.RecipientID != "team" && in.RecipientID != "coordinator" {
				in.RecipientKind = "agent"
			} else {
				in.RecipientKind = "coordinator"
				in.RecipientID = ""
			}
		}
		state, err := load(ctx, tx, project)
		if err != nil {
			return err
		}
		switch in.RecipientKind {
		case "agent":
			idx := agentIndex(state, in.RecipientID)
			if idx < 0 {
				return ErrNotFound
			}
			if state.Agents[idx].Status != "active" {
				return fmt.Errorf("%w: recipient unavailable", ErrValidation)
			}
		case "team":
			if in.RecipientID != "" {
				found := false
				for _, t := range state.Teams {
					if t.ID == in.RecipientID && !t.Archived {
						found = true
					}
				}
				if !found {
					return ErrNotFound
				}
			}
		case "coordinator":
			if in.RecipientID != "" && in.RecipientID != state.Settings.CoordinatorID {
				return ErrNotFound
			}
		default:
			return fmt.Errorf("%w: unknown recipient kind", ErrValidation)
		}
		messageID, err := newID("message-")
		if err != nil {
			return err
		}
		intentID, err := newID("intent-")
		if err != nil {
			return err
		}
		out = Message{MessageInput: in, ID: messageID, IntentID: intentID, SenderKind: "user", Status: "awaiting_coordination", CreatedAt: now()}
		intent := Intent{ID: intentID, Revision: 1, MessageID: messageID, Status: "AWAITING_COORDINATOR", CreatedAt: out.CreatedAt}
		state.Messages = append(state.Messages, out)
		state.Intents = append(state.Intents, intent)
		if _, err := tx.ExecContext(ctx, `INSERT INTO office_outbox(project_id,intent_id) VALUES(?,?)`, project, intentID); err != nil {
			return err
		}
		if err := save(ctx, tx, state); err != nil {
			return err
		}
		if err := event(ctx, tx, project, "office.message.persisted", out); err != nil {
			return err
		}
		if err := event(ctx, tx, project, "office.intent.recorded", intent); err != nil {
			return err
		}
		return receipt(ctx, tx, project, "message.submit", key, input, out)
	})
	return out, err
}

// Stop records a dispatch barrier and stop request. A paused office is not proof
// that an already accepted worker has stopped; task observations remain intact.
func (s *Store) Stop(ctx context.Context, project string) (Settings, error) {
	var out Settings
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		state, err := load(ctx, tx, project)
		if err != nil {
			return err
		}
		if state.Settings.Paused {
			out = state.Settings
			return nil
		}
		state.Settings.Paused = true
		state.Settings.Revision++
		out = state.Settings
		for i, intent := range state.Intents {
			if intent.Status == "AUTHORIZED" {
				state.Intents[i].Status = "CANCELLED"
				state.Intents[i].Revision++
				if _, err := tx.ExecContext(ctx, `UPDATE office_outbox SET status='CANCELLED' WHERE project_id=? AND intent_id=?`, project, intent.ID); err != nil {
					return err
				}
			}
		}
		if err := save(ctx, tx, state); err != nil {
			return err
		}
		return event(ctx, tx, project, "office.stop.requested", map[string]any{"settings": out, "workerStopConfirmed": false})
	})
	return out, err
}
