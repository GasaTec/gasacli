package office

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/storage"
)

var ErrCoordinatorSession = errors.New("office: coordinator session unavailable or unauthorized")

func init() {
	if err := storage.RegisterMigration(storage.Migration{Version: 83, Name: "office_coordinator_sessions", SQL: `
CREATE TABLE office_coordinator_sessions(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,secret_hash BLOB NOT NULL,data TEXT NOT NULL,revoked INTEGER NOT NULL DEFAULT 0);
CREATE INDEX office_coordinator_sessions_project ON office_coordinator_sessions(project_id);
`}); err != nil {
		panic(err)
	}
}

// CoordinatorSession binds a local credential to one configuration. RuntimeID
// is a fence supplied by a trusted host integration, not proof of Orca authority.
// Minting this credential never grants an Orca caller identity or dispatches work.
type CoordinatorSession struct {
	ID                  string    `json:"id"`
	ProjectID           string    `json:"projectId"`
	CoordinatorID       string    `json:"coordinatorId"`
	CoordinatorRevision int       `json:"coordinatorRevision"`
	ConfigHash          string    `json:"configHash"`
	RuntimeID           string    `json:"runtimeId"`
	ExpiresAt           time.Time `json:"expiresAt"`
	Resources           []string  `json:"resources"`
	Capabilities        []string  `json:"capabilities"`
}

// CoordinatorCredential is returned only at mint time. Callers must deliver it
// through a private local channel, never a browser route, prompt or argv.
type CoordinatorCredential struct {
	Session CoordinatorSession
	Secret  string `json:"-"`
}

type CoordinatorSessionRequest struct {
	ProjectID, CoordinatorID, RuntimeID string
	CoordinatorRevision                 int
	ExpiresAt                           time.Time
	Resources, Capabilities             []string
}

// CoordinatorAuthentication must originate from the owned coordinator process.
// Browser sessions and worker credentials are different roles and are rejected.
type CoordinatorAuthentication struct {
	SessionID, Secret, Role, ProjectID, RuntimeID string
}

type CoordinatorDecisionRequest struct {
	MessageID, IntentID, IdempotencyKey string
	IntentRevision                      int
	Decision                            Decision
}

func coordinatorConfigHash(state Snapshot) (string, error) {
	data, err := json.Marshal(struct {
		Agents   []Agent
		Teams    []Team
		Settings Settings
	}{state.Agents, state.Teams, state.Settings})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func boundedUnique(values []string) bool {
	if len(values) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, v := range values {
		if v == "" || len(v) > 4096 || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}

func validCoordinatorResource(value string) bool {
	// Exact project-relative resources, not shell patterns, URLs or traversal.
	return value != "" && !strings.ContainsAny(value, "\\\x00\r\n:*?[]") && !strings.HasPrefix(value, "/") && value != "." && value != ".." && path.Clean(value) == value && !strings.HasPrefix(value, "../")
}

func subset(values, allowed []string) bool {
	set := map[string]bool{}
	for _, v := range allowed {
		set[v] = true
	}
	for _, v := range values {
		if !set[v] {
			return false
		}
	}
	return true
}

// MintCoordinatorSession is an internal integration API. Its caller must first
// establish genuine runtime ownership and human consent. No HTTP route exposes it.
func (s *Store) MintCoordinatorSession(ctx context.Context, in CoordinatorSessionRequest) (CoordinatorCredential, error) {
	var out CoordinatorCredential
	if in.RuntimeID == "" || len(in.RuntimeID) > 512 || !in.ExpiresAt.After(time.Now()) || in.ExpiresAt.After(time.Now().Add(24*time.Hour)) || !boundedUnique(in.Resources) || !boundedUnique(in.Capabilities) {
		return out, ErrValidation
	}
	for _, r := range in.Resources {
		if !validCoordinatorResource(r) {
			return out, ErrValidation
		}
	}
	err := s.tx(ctx, in.ProjectID, func(tx *sql.Tx) error {
		state, err := load(ctx, tx, in.ProjectID)
		if err != nil {
			return err
		}
		idx := agentIndex(state, in.CoordinatorID)
		if state.Settings.Paused || state.Settings.CoordinatorID != in.CoordinatorID || idx < 0 || state.Agents[idx].Status != "active" || state.Agents[idx].Revision != in.CoordinatorRevision {
			return ErrCoordinatorSession
		}
		hash, err := coordinatorConfigHash(state)
		if err != nil {
			return err
		}
		id, err := newID("coordinator-")
		if err != nil {
			return err
		}
		var secret [32]byte
		if _, err = rand.Read(secret[:]); err != nil {
			return err
		}
		out = CoordinatorCredential{Session: CoordinatorSession{ID: id, ProjectID: in.ProjectID, CoordinatorID: in.CoordinatorID, CoordinatorRevision: in.CoordinatorRevision, ConfigHash: hash, RuntimeID: in.RuntimeID, ExpiresAt: in.ExpiresAt.UTC(), Resources: append([]string{}, in.Resources...), Capabilities: append([]string{}, in.Capabilities...)}, Secret: hex.EncodeToString(secret[:])}
		data, err := json.Marshal(out.Session)
		if err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(out.Secret))
		_, err = tx.ExecContext(ctx, `INSERT INTO office_coordinator_sessions(id,project_id,secret_hash,data) VALUES(?,?,?,?)`, id, in.ProjectID, digest[:], string(data))
		return err
	})
	if err != nil {
		return CoordinatorCredential{}, err
	}
	return out, nil
}

func authenticateCoordinator(ctx context.Context, tx *sql.Tx, in CoordinatorAuthentication) (CoordinatorSession, error) {
	var out CoordinatorSession
	if in.Role != "coordinator" || len(in.Secret) != 64 || in.RuntimeID == "" {
		return out, ErrCoordinatorSession
	}
	var hash []byte
	var data string
	var revoked int
	err := tx.QueryRowContext(ctx, `SELECT secret_hash,data,revoked FROM office_coordinator_sessions WHERE id=? AND project_id=?`, in.SessionID, in.ProjectID).Scan(&hash, &data, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrCoordinatorSession
	}
	if err != nil {
		return out, err
	}
	supplied := sha256.Sum256([]byte(in.Secret))
	if subtle.ConstantTimeCompare(supplied[:], hash) != 1 || revoked != 0 {
		return out, ErrCoordinatorSession
	}
	if err = json.Unmarshal([]byte(data), &out); err != nil {
		return out, err
	}
	if out.RuntimeID != in.RuntimeID || !out.ExpiresAt.After(time.Now()) {
		return CoordinatorSession{}, ErrCoordinatorSession
	}
	state, err := load(ctx, tx, in.ProjectID)
	if err != nil {
		return CoordinatorSession{}, err
	}
	idx := agentIndex(state, out.CoordinatorID)
	if state.Settings.Paused || state.Settings.CoordinatorID != out.CoordinatorID || idx < 0 || state.Agents[idx].Status != "active" || state.Agents[idx].Revision != out.CoordinatorRevision {
		return CoordinatorSession{}, ErrCoordinatorSession
	}
	current, err := coordinatorConfigHash(state)
	if err != nil {
		return CoordinatorSession{}, err
	}
	if current != out.ConfigHash {
		return CoordinatorSession{}, ErrCoordinatorSession
	}
	return out, nil
}

func (s *Store) AuthenticateCoordinator(ctx context.Context, in CoordinatorAuthentication) (CoordinatorSession, error) {
	var out CoordinatorSession
	err := s.tx(ctx, in.ProjectID, func(tx *sql.Tx) error { var err error; out, err = authenticateCoordinator(ctx, tx, in); return err })
	return out, err
}

// RevokeCoordinatorSession is an internal administrative operation and is
// idempotent. It does not claim to stop any external process.
func (s *Store) RevokeCoordinatorSession(ctx context.Context, project, id string) error {
	return s.tx(ctx, project, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE office_coordinator_sessions SET revoked=1 WHERE project_id=? AND id=?`, project, id)
		return err
	})
}

// IngestCoordinatorDecision authenticates, validates scope, records the existing
// office decision, and saves its replay receipt in one transaction. It does not
// reserve execution resources, satisfy human approvals, or launch a worker.
func (s *Store) IngestCoordinatorDecision(ctx context.Context, auth CoordinatorAuthentication, in CoordinatorDecisionRequest) (Intent, error) {
	var out Intent
	err := s.tx(ctx, auth.ProjectID, func(tx *sql.Tx) error {
		session, err := authenticateCoordinator(ctx, tx, auth)
		if err != nil {
			return err
		}
		operation := "coordinator.decision." + session.ID
		found, err := replay(ctx, tx, auth.ProjectID, operation, in.IdempotencyKey, in, &out)
		if err != nil || found {
			return err
		}
		if in.IntentRevision < 1 || in.MessageID == "" || in.IntentID == "" || !boundedUnique(in.Decision.Resources) || !boundedUnique(in.Decision.Capabilities) {
			return ErrValidation
		}
		state, err := load(ctx, tx, auth.ProjectID)
		if err != nil {
			return err
		}
		matched := false
		for _, message := range state.Messages {
			if message.ID == in.MessageID && message.IntentID == in.IntentID {
				matched = true
				break
			}
		}
		if !matched {
			return ErrNotFound
		}
		decision := in.Decision
		if (decision.CoordinatorID != "" && decision.CoordinatorID != session.CoordinatorID) || (decision.CoordinatorRevision != 0 && decision.CoordinatorRevision != session.CoordinatorRevision) {
			return ErrCoordinatorSession
		}
		if !subset(decision.Resources, session.Resources) || !subset(decision.Capabilities, session.Capabilities) {
			return fmt.Errorf("%w: decision exceeds session scope", ErrValidation)
		}
		if decision.Outcome == "AUTHORIZED" {
			expires, err := time.Parse(time.RFC3339Nano, decision.ExpiresAt)
			if err != nil || expires.After(session.ExpiresAt) || len(decision.Resources) == 0 {
				return ErrValidation
			}
			for _, r := range decision.Resources {
				if !validCoordinatorResource(r) {
					return ErrValidation
				}
			}
		}
		decision.CoordinatorID = session.CoordinatorID
		decision.CoordinatorRevision = session.CoordinatorRevision
		if err = recordDecision(ctx, tx, auth.ProjectID, in.IntentID, in.IntentRevision, decision, &out); err != nil {
			return err
		}
		return receipt(ctx, tx, auth.ProjectID, operation, in.IdempotencyKey, in, out)
	})
	return out, err
}
