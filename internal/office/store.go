package office

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/GasaTec/gasacli/internal/engine" // Existing task projection schema.
	"github.com/GasaTec/gasacli/internal/storage"
)

func init() {
	if err := storage.RegisterMigration(storage.Migration{Version: 80, Name: "local_office", SQL: `
CREATE TABLE office_state (project_id TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE office_events (id INTEGER PRIMARY KEY AUTOINCREMENT, project_id TEXT NOT NULL, type TEXT NOT NULL, origin TEXT NOT NULL, payload TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE INDEX office_events_project_cursor ON office_events(project_id,id);
CREATE TABLE office_receipts (project_id TEXT NOT NULL, operation TEXT NOT NULL, key TEXT NOT NULL, hash TEXT NOT NULL, response TEXT NOT NULL, PRIMARY KEY(project_id,operation,key));
CREATE TABLE office_task_bindings (project_id TEXT NOT NULL, intent_id TEXT NOT NULL, task_id TEXT NOT NULL UNIQUE, agent_id TEXT NOT NULL, PRIMARY KEY(project_id,intent_id,task_id));
CREATE TABLE office_outbox (project_id TEXT NOT NULL, intent_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending', PRIMARY KEY(project_id,intent_id));
`}); err != nil {
		panic(err)
	}
}

type Store struct{ store *storage.Store }

func New(s *storage.Store) *Store { return &Store{store: s} }

func empty(project string) Snapshot {
	return Snapshot{ProjectID: project, Agents: []Agent{}, Teams: []Team{}, Messages: []Message{}, Intents: []Intent{}, Tasks: []Task{}, Settings: Settings{Quality: "balanced", Autonomy: "accepted_plan"}}
}

func validProject(project string) error {
	if strings.TrimSpace(project) == "" || len(project) > 4096 || strings.ContainsAny(project, "\x00\r\n") {
		return fmt.Errorf("%w: project is required", ErrValidation)
	}
	return nil
}

func load(ctx context.Context, tx *sql.Tx, project string) (Snapshot, error) {
	s := empty(project)
	var data string
	err := tx.QueryRowContext(ctx, `SELECT data FROM office_state WHERE project_id=?`, project).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal([]byte(data), &s)
	return s, err
}

func save(ctx context.Context, tx *sql.Tx, s Snapshot) error {
	// Runtime projections and cursor are queried atomically, never copied into
	// the office document as an independent task source of truth.
	s.Tasks = []Task{}
	s.Metrics = Metrics{}
	s.Cursor = 0
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO office_state(project_id,data) VALUES(?,?) ON CONFLICT(project_id) DO UPDATE SET data=excluded.data`, s.ProjectID, string(b))
	return err
}

func event(ctx context.Context, tx *sql.Tx, project, kind string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO office_events(project_id,type,origin,payload,created_at) VALUES(?,?,?,?,?)`, project, kind, "local.office", string(b), now())
	return err
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func newID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

func (s *Store) tx(ctx context.Context, project string, fn func(*sql.Tx) error) error {
	if err := validProject(project); err != nil {
		return err
	}
	if s == nil || s.store == nil {
		return fmt.Errorf("%w: storage unavailable", ErrValidation)
	}
	return s.store.Tx(ctx, fn)
}

func replay(ctx context.Context, tx *sql.Tx, project, operation, key string, input any, out any) (bool, error) {
	if strings.TrimSpace(key) == "" || len(key) > 200 {
		return false, fmt.Errorf("%w: idempotency key required (up to 200 bytes)", ErrValidation)
	}
	b, err := json.Marshal(input)
	if err != nil {
		return false, err
	}
	h := sha256.Sum256(b)
	var hash, response string
	err = tx.QueryRowContext(ctx, `SELECT hash,response FROM office_receipts WHERE project_id=? AND operation=? AND key=?`, project, operation, key).Scan(&hash, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if hash != hex.EncodeToString(h[:]) {
		return false, fmt.Errorf("%w: idempotency key has a different payload", ErrConflict)
	}
	return true, json.Unmarshal([]byte(response), out)
}

func receipt(ctx context.Context, tx *sql.Tx, project, operation, key string, input, out any) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	h := sha256.Sum256(b)
	response, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO office_receipts(project_id,operation,key,hash,response) VALUES(?,?,?,?,?)`, project, operation, key, hex.EncodeToString(h[:]), string(response))
	return err
}

func (s *Store) Snapshot(ctx context.Context, project string) (Snapshot, error) {
	var out Snapshot
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		var err error
		out, err = load(ctx, tx, project)
		if err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM office_events WHERE project_id=?`, project).Scan(&out.Cursor); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT t.id,t.run_id,t.name,t.status,r.status,b.agent_id FROM office_task_bindings b JOIN tasks t ON t.id=b.task_id JOIN runs r ON r.id=t.run_id WHERE b.project_id=? ORDER BY r.created_at,t.id`, project)
		if err != nil {
			return err
		}
		defer rows.Close()
		out.Tasks = []Task{}
		out.Metrics = Metrics{Requests: len(out.Intents)}
		for rows.Next() {
			var task Task
			var runStatus string
			if err := rows.Scan(&task.ID, &task.RunID, &task.Title, &task.Status, &runStatus, &task.AgentID); err != nil {
				return err
			}
			out.Tasks = append(out.Tasks, task)
			out.Metrics.Tasks++
			switch task.Status {
			case "DONE":
				if runStatus == "DONE" {
					out.Metrics.Verified++
				}
				out.Metrics.Closed++
			case "FAILED":
				out.Metrics.Closed++
			case "BLOCKED":
				out.Metrics.Blocked++
			case "CANCELLED":
				out.Metrics.Cancelled++
				out.Metrics.Closed++
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if out.Metrics.Closed > 0 {
			rate := float64(out.Metrics.Verified) / float64(out.Metrics.Closed)
			out.Metrics.SuccessRate = &rate
		}
		return nil
	})
	return out, err
}

// Events returns bounded project-local replay in increasing durable cursor order.
// Consumers request again with the last cursor until a page has fewer than 500.
func (s *Store) Events(ctx context.Context, project string, after int64) ([]Event, error) {
	out := []Event{}
	if after < 0 {
		return nil, fmt.Errorf("%w: negative cursor", ErrValidation)
	}
	err := s.tx(ctx, project, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id,project_id,type,origin,payload,created_at FROM office_events WHERE project_id=? AND id>? ORDER BY id LIMIT 500`, project, after)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e Event
			var payload string
			if err := rows.Scan(&e.Cursor, &e.ProjectID, &e.Type, &e.Origin, &payload, &e.CreatedAt); err != nil {
				return err
			}
			e.Payload = json.RawMessage(payload)
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

func projectHasWork(ctx context.Context, tx *sql.Tx, project string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM office_task_bindings b JOIN tasks t ON t.id=b.task_id WHERE b.project_id=? AND (t.status NOT IN ('DONE','CANCELLED','FAILED','BLOCKED') OR EXISTS (SELECT 1 FROM dispatches d WHERE d.task_id=t.id AND d.status NOT IN ('SUCCEEDED','FAILED','TIMED_OUT','CANCELLED')))`, project).Scan(&n)
	return n > 0, err
}
