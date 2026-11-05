package office

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

func normalizeAgent(in AgentInput) (AgentInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Role = strings.TrimSpace(in.Role)
	if in.Status == "" {
		in.Status = "active"
	}
	if in.FallbackMode == "" {
		in.FallbackMode = "confirm"
	}
	if in.Fallbacks == nil {
		in.Fallbacks = []ExecutionChoice{}
	}
	if in.TeamIDs == nil {
		in.TeamIDs = []string{}
	}
	if in.AllowedCapabilities == nil {
		in.AllowedCapabilities = []string{}
	}
	if in.Avatar.Accessories == nil {
		in.Avatar.Accessories = []string{}
	}
	bad := func(message string) (AgentInput, error) { return in, fmt.Errorf("%w: %s", ErrValidation, message) }
	if !utf8.ValidString(in.Name) || utf8.RuneCountInString(in.Name) > 60 || utf8.RuneCountInString(in.WhenToCall) > 300 || len(in.Instructions) > 16384 {
		return bad("field length limit exceeded")
	}
	if in.Status != "draft" && in.Status != "active" && in.Status != "paused" && in.Status != "archived" {
		return bad("unknown agent status")
	}
	if in.Status != "draft" && (in.Name == "" || in.Role == "" || in.Primary.ToolID == "") {
		return bad("name, role and primary tool required")
	}
	if in.Avatar.FamilyID != "" {
		switch in.Avatar.FamilyID {
		case "fox", "cat", "owl", "capybara", "robot":
		default:
			return bad("unsupported avatar family")
		}
	} else if in.Status != "draft" {
		return bad("avatar family required")
	}
	if len(in.Avatar.Color) > 64 || len(in.Avatar.Eyes) > 64 || len(in.Avatar.Accessories) > 8 {
		return bad("invalid avatar options")
	}
	if in.FallbackMode != "confirm" && in.FallbackMode != "automatic_safe" {
		return bad("invalid fallback mode")
	}
	choices := map[ExecutionChoice]bool{in.Primary: true}
	for _, choice := range in.Fallbacks {
		if choice.ToolID == "" || choices[choice] {
			return bad("duplicate or empty execution choice")
		}
		choices[choice] = true
	}
	if len(in.Fallbacks) > 10 || len(in.TeamIDs) > 100 || len(in.AllowedCapabilities) > 50 {
		return bad("too many configuration entries")
	}
	for _, list := range [][]string{in.TeamIDs, in.AllowedCapabilities, in.Avatar.Accessories} {
		seen := map[string]bool{}
		for _, v := range list {
			if strings.TrimSpace(v) == "" || len(v) > 100 || seen[v] {
				return bad("empty or duplicate option")
			}
			seen[v] = true
		}
	}
	return in, nil
}

func agentIndex(state Snapshot, id string) int {
	for i, a := range state.Agents {
		if a.ID == id {
			return i
		}
	}
	return -1
}

func validateTeams(state Snapshot, in AgentInput, agentID string) error {
	for _, id := range in.TeamIDs {
		found := false
		for _, t := range state.Teams {
			if t.ID == id && !t.Archived {
				if len(t.MemberIDs) >= 3 && !slices.Contains(t.MemberIDs, agentID) {
					return fmt.Errorf("%w: team %s already has three members", ErrValidation, id)
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: team %s unavailable", ErrValidation, id)
		}
	}
	return nil
}

func allocateDesk(state Snapshot) string {
	used := map[string]bool{}
	for _, a := range state.Agents {
		if a.DeskID != "" {
			used[a.DeskID] = true
		}
	}
	for n := 1; ; n++ {
		id := fmt.Sprintf("desk-%03d", n)
		if !used[id] {
			return id
		}
	}
}

func chooseCoordinator(state *Snapshot, a Agent) error {
	if a.Status == "active" && a.Role == "coordinator" {
		if state.Settings.CoordinatorID != "" && state.Settings.CoordinatorID != a.ID {
			return fmt.Errorf("%w: select a replacement coordinator explicitly", ErrConflict)
		}
		if state.Settings.CoordinatorID != a.ID {
			state.Settings.CoordinatorID = a.ID
			state.Settings.Revision++
		}
	}
	return nil
}

func (s *Store) CreateAgent(ctx context.Context, project, key string, input AgentInput) (Agent, error) {
	var out Agent
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		ok, err := replay(ctx, tx, project, "agent.create", key, input, &out)
		if err != nil || ok {
			return err
		}
		in, err := normalizeAgent(input)
		if err != nil {
			return err
		}
		state, err := load(ctx, tx, project)
		if err != nil {
			return err
		}
		if err := validateTeams(state, in, ""); err != nil {
			return err
		}
		id, err := newID("agent-")
		if err != nil {
			return err
		}
		out = Agent{AgentInput: in, ID: id, Revision: 1}
		if in.Status == "active" || in.Status == "paused" {
			out.DeskID = allocateDesk(state)
		}
		if err := chooseCoordinator(&state, out); err != nil {
			return err
		}
		state.Agents = append(state.Agents, out)
		syncMembership(&state)
		if err := save(ctx, tx, state); err != nil {
			return err
		}
		if err := event(ctx, tx, project, "office.agent.created", out); err != nil {
			return err
		}
		return receipt(ctx, tx, project, "agent.create", key, input, out)
	})
	return out, err
}

// ReplayAgent checks an existing create receipt before readiness validation.
// A committed identity remains replayable if its provider later goes offline.
// A missing receipt does not authorize creation or skip normal validation.
func (s *Store) ReplayAgent(ctx context.Context, project, key string, input AgentInput) (Agent, bool, error) {
	var out Agent
	var found bool
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		var err error
		found, err = replay(ctx, tx, project, "agent.create", key, input, &out)
		return err
	})
	return out, found, err
}

func (s *Store) UpdateAgent(ctx context.Context, project, id string, revision int, input AgentInput) (Agent, error) {
	var out Agent
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		in, err := normalizeAgent(input)
		if err != nil {
			return err
		}
		state, err := load(ctx, tx, project)
		if err != nil {
			return err
		}
		idx := agentIndex(state, id)
		if idx < 0 {
			return ErrNotFound
		}
		old := state.Agents[idx]
		if old.Revision != revision {
			return ErrConflict
		}
		if err := validateTeams(state, in, id); err != nil {
			return err
		}
		// Without a per-dispatch permission revocation adapter, do not pretend a
		// new policy is enforced against running work. Keep changes pending.
		busy, err := projectHasWork(ctx, tx, project)
		if err != nil {
			return err
		}
		if busy {
			return ErrBusy
		}
		if state.Settings.CoordinatorID == id && (in.Status != "active" || in.Role != "coordinator") {
			if !state.Settings.Paused {
				return fmt.Errorf("%w: pause office before removing coordinator", ErrConflict)
			}
			state.Settings.CoordinatorID = ""
			state.Settings.Revision++
		}
		out = Agent{AgentInput: in, ID: id, Revision: revision + 1, DeskID: old.DeskID}
		if in.Status == "draft" || in.Status == "archived" {
			out.DeskID = ""
		} else if out.DeskID == "" {
			out.DeskID = allocateDesk(state)
		}
		if err := chooseCoordinator(&state, out); err != nil {
			return err
		}
		state.Agents[idx] = out
		syncMembership(&state)
		if err := save(ctx, tx, state); err != nil {
			return err
		}
		return event(ctx, tx, project, "office.agent.updated", out)
	})
	return out, err
}

func syncMembership(state *Snapshot) {
	for i := range state.Teams {
		previous := state.Teams[i].MemberIDs
		state.Teams[i].MemberIDs = []string{}
		for _, a := range state.Agents {
			for _, id := range a.TeamIDs {
				if id == state.Teams[i].ID {
					state.Teams[i].MemberIDs = append(state.Teams[i].MemberIDs, a.ID)
				}
			}
		}
		if !slices.Equal(previous, state.Teams[i].MemberIDs) {
			state.Teams[i].Revision++
			if !slices.Contains(state.Teams[i].MemberIDs, state.Teams[i].LeaderID) {
				state.Teams[i].LeaderID = ""
			}
		}
	}
}
