// Package engine implements durable orchestration primitives for local runs.
package engine

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/storage"
)

type RunStatus string
type TaskStatus string
type DispatchStatus string

const (
	TrustPending RunStatus = "TRUST_PENDING"
	Planning     RunStatus = "PLANNING"
	Running      RunStatus = "RUNNING"
	Review       RunStatus = "REVIEW"
	Fixing       RunStatus = "FIXING"
	Gates        RunStatus = "GATES"
	Done         RunStatus = "DONE"
	Blocked      RunStatus = "BLOCKED"
	Cancelled    RunStatus = "CANCELLED"

	Queued        TaskStatus = "QUEUED"
	Ready         TaskStatus = "READY"
	Executing     TaskStatus = "RUNNING"
	Failed        TaskStatus = "FAILED"
	TaskBlocked   TaskStatus = "BLOCKED"
	TaskDone      TaskStatus = "DONE"
	TaskCancelled TaskStatus = "CANCELLED"

	DispatchCreated   DispatchStatus = "CREATED"
	DispatchRunning   DispatchStatus = "RUNNING"
	DispatchSucceeded DispatchStatus = "SUCCEEDED"
	DispatchFailed    DispatchStatus = "FAILED"
	DispatchTimedOut  DispatchStatus = "TIMED_OUT"
	DispatchCancelled DispatchStatus = "CANCELLED"
)

type Run struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Goal      string    `json:"goal"`
	Status    RunStatus `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Task struct {
	ID           string        `json:"id"`
	RunID        string        `json:"run_id"`
	Name         string        `json:"name"`
	Status       TaskStatus    `json:"status"`
	Dependencies []string      `json:"dependencies,omitempty"`
	Retries      int           `json:"retries"`
	Timeout      time.Duration `json:"timeout,omitempty"`
}
type Dispatch struct {
	ID      string         `json:"id"`
	TaskID  string         `json:"task_id"`
	Attempt int            `json:"attempt"`
	Worker  string         `json:"worker"`
	Status  DispatchStatus `json:"status"`
	Reason  string         `json:"reason,omitempty"`
}
type Event struct {
	ID         int64           `json:"id"`
	RunID      string          `json:"run_id"`
	TaskID     string          `json:"task_id,omitempty"`
	DispatchID string          `json:"dispatch_id,omitempty"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}
type Checkpoint struct {
	RunID     string          `json:"run_id"`
	TaskID    string          `json:"task_id,omitempty"`
	Name      string          `json:"name"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
type Worker interface {
	Run(context.Context, Task) error
}

type Engine struct{ store *storage.Store }

func New(store *storage.Store) (*Engine, error) {
	if store == nil {
		return nil, errors.New("engine: storage is required")
	}
	if err := store.Migrate(context.Background()); err != nil {
		return nil, err
	}
	return &Engine{store: store}, nil
}

func (e *Engine) CreateRun(ctx context.Context, project, goal string) (Run, error) {
	if project == "" || goal == "" {
		return Run{}, errors.New("engine: project and goal are required")
	}
	now, id := time.Now().UTC(), newID()
	run := Run{ID: id, Project: project, Goal: goal, Status: TrustPending, CreatedAt: now, UpdatedAt: now}
	if err := e.store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO runs(id,project,goal,status,created_at,updated_at) VALUES(?,?,?,?,?,?)`, run.ID, run.Project, run.Goal, run.Status, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("engine: create run: %w", err)
		}
		payload, _ := json.Marshal(map[string]string{"status": string(run.Status)})
		_, err := tx.ExecContext(ctx, `INSERT INTO events(run_id,type,payload,created_at) VALUES(?,?,?,?)`, run.ID, "run.created", string(payload), now.Format(time.RFC3339Nano))
		return err
	}); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (e *Engine) AddTasks(ctx context.Context, runID string, tasks []Task) error {
	if len(tasks) == 0 {
		return errors.New("engine: at least one task is required")
	}
	seen := map[string]bool{}
	for _, t := range tasks {
		if t.ID == "" || t.Name == "" || seen[t.ID] || t.Timeout < 0 {
			return fmt.Errorf("engine: invalid or duplicate task %q", t.ID)
		}
		seen[t.ID] = true
	}
	for _, t := range tasks {
		for _, dep := range t.Dependencies {
			if !seen[dep] {
				return fmt.Errorf("engine: dependency %q is not in task plan", dep)
			}
		}
	}
	if hasCycle(tasks) {
		return errors.New("engine: task dependencies contain a cycle")
	}
	return e.store.Tx(ctx, func(tx *sql.Tx) error {
		for _, t := range tasks {
			deps, _ := json.Marshal(t.Dependencies)
			timeout := t.Timeout.Milliseconds()
			if _, err := tx.ExecContext(ctx, `INSERT INTO tasks(id,run_id,name,status,dependencies,timeout_ms,updated_at) VALUES(?,?,?,?,?,?,?)`, t.ID, runID, t.Name, Queued, deps, timeout, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `UPDATE runs SET status=?,updated_at=? WHERE id=? AND status IN (?,?)`, Planning, time.Now().UTC().Format(time.RFC3339Nano), runID, TrustPending, Planning)
		return err
	})
}

// CancelRun records cancellation after the caller has canceled the scheduler context.
func (e *Engine) CancelRun(ctx context.Context, id string) error {
	run, err := e.GetRun(ctx, id)
	if err != nil {
		return err
	}
	if run.Status == Done || run.Status == Cancelled {
		return fmt.Errorf("engine: run %s is already %s", id, run.Status)
	}
	if err = e.Transition(ctx, id, Cancelled, "cancelled by user"); err != nil {
		return err
	}
	_, err = e.store.DB().ExecContext(ctx, `UPDATE tasks SET status=?,updated_at=? WHERE run_id=? AND status IN (?,?)`, TaskCancelled, time.Now().UTC().Format(time.RFC3339Nano), id, Queued, Executing)
	return err
}

func (e *Engine) GetRun(ctx context.Context, id string) (Run, error) {
	var r Run
	var created, updated string
	err := e.store.DB().QueryRowContext(ctx, `SELECT id,project,goal,status,reason,created_at,updated_at FROM runs WHERE id=?`, id).Scan(&r.ID, &r.Project, &r.Goal, &r.Status, &r.Reason, &created, &updated)
	if err != nil {
		return Run{}, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return r, nil
}
func (e *Engine) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := e.store.DB().QueryContext(ctx, `SELECT id,project,goal,status,reason,created_at,updated_at FROM runs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		var c, u string
		if err := rows.Scan(&r.ID, &r.Project, &r.Goal, &r.Status, &r.Reason, &c, &u); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
		r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, u)
		out = append(out, r)
	}
	return out, rows.Err()
}
func (e *Engine) Tasks(ctx context.Context, runID string) ([]Task, error) {
	rows, err := e.store.DB().QueryContext(ctx, `SELECT id,run_id,name,status,dependencies,retries,timeout_ms FROM tasks WHERE run_id=? ORDER BY rowid`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		var deps string
		var seconds int64
		if err := rows.Scan(&t.ID, &t.RunID, &t.Name, &t.Status, &deps, &t.Retries, &seconds); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(deps), &t.Dependencies)
		t.Timeout = time.Duration(seconds) * time.Millisecond
		out = append(out, t)
	}
	return out, rows.Err()
}
func (e *Engine) Events(ctx context.Context, runID string, after int64, limit int) ([]Event, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	rows, err := e.store.DB().QueryContext(ctx, `SELECT id,run_id,task_id,dispatch_id,type,payload,created_at FROM events WHERE run_id=? AND id>? ORDER BY id LIMIT ?`, runID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var v Event
		var payload, created string
		if err := rows.Scan(&v.ID, &v.RunID, &v.TaskID, &v.DispatchID, &v.Type, &payload, &created); err != nil {
			return nil, err
		}
		v.Payload = json.RawMessage(payload)
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, v)
	}
	return out, rows.Err()
}
func (e *Engine) Checkpoint(ctx context.Context, c Checkpoint) error {
	if c.Name == "" {
		return errors.New("engine: checkpoint name is required")
	}
	if len(c.Payload) == 0 {
		c.Payload = json.RawMessage(`{}`)
	}
	now := time.Now().UTC()
	_, err := e.store.DB().ExecContext(ctx, `INSERT INTO checkpoints(run_id,task_id,name,payload,created_at) VALUES(?,?,?,?,?) ON CONFLICT(run_id,task_id,name) DO UPDATE SET payload=excluded.payload,created_at=excluded.created_at`, c.RunID, c.TaskID, c.Name, string(c.Payload), now.Format(time.RFC3339Nano))
	return err
}
func (e *Engine) LastCheckpoint(ctx context.Context, runID, taskID string) (Checkpoint, error) {
	var c Checkpoint
	var p, created string
	err := e.store.DB().QueryRowContext(ctx, `SELECT run_id,task_id,name,payload,created_at FROM checkpoints WHERE run_id=? AND task_id=? ORDER BY created_at DESC LIMIT 1`, runID, taskID).Scan(&c.RunID, &c.TaskID, &c.Name, &p, &created)
	if err != nil {
		return Checkpoint{}, err
	}
	c.Payload = json.RawMessage(p)
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return c, nil
}

func (e *Engine) Transition(ctx context.Context, id string, to RunStatus, reason string) error {
	allowed := map[RunStatus]map[RunStatus]bool{TrustPending: {Planning: true, Blocked: true, Cancelled: true}, Planning: {Running: true, Blocked: true, Cancelled: true}, Running: {Review: true, Fixing: true, Gates: true, Blocked: true, Cancelled: true}, Review: {Fixing: true, Gates: true, Blocked: true, Cancelled: true}, Fixing: {Running: true, Gates: true, Blocked: true, Cancelled: true}, Gates: {Review: true, Fixing: true, Blocked: true, Cancelled: true}, Blocked: {Planning: true, Running: true, Cancelled: true}}
	r, err := e.GetRun(ctx, id)
	if err != nil {
		return err
	}
	if !allowed[r.Status][to] {
		return fmt.Errorf("engine: invalid run transition %s -> %s", r.Status, to)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := e.store.DB().ExecContext(ctx, `UPDATE runs SET status=?,reason=?,updated_at=? WHERE id=? AND status=?`, to, reason, now, id, r.Status)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("engine: run changed concurrently")
	}
	return e.addEvent(ctx, id, "", "", "run.state", map[string]string{"from": string(r.Status), "to": string(to), "reason": reason})
}

type CompletionEvidence struct {
	ChecksPassed   bool
	ReviewApproved bool
	Developer      string
	Reviewer       string
	Commit         string
}

// CompleteRun is the only transition to DONE and requires current check and review evidence.
func (e *Engine) CompleteRun(ctx context.Context, id string, evidence CompletionEvidence) error {
	if !evidence.ChecksPassed || !evidence.ReviewApproved || strings.TrimSpace(evidence.Commit) == "" {
		return errors.New("engine: DONE requires passing checks, an approved review, and a reviewed commit")
	}
	if strings.TrimSpace(evidence.Developer) == "" || strings.TrimSpace(evidence.Reviewer) == "" || evidence.Developer == evidence.Reviewer {
		return errors.New("engine: review must come from a distinct developer and reviewer")
	}
	run, err := e.GetRun(ctx, id)
	if err != nil {
		return err
	}
	if run.Status != Gates {
		return fmt.Errorf("engine: DONE requires GATES state, got %s", run.Status)
	}
	if err := e.requireVerificationEvidence(ctx, id, evidence.Reviewer); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := e.store.DB().ExecContext(ctx, `UPDATE runs SET status=?,reason='',updated_at=? WHERE id=? AND status=?`, Done, now, id, Gates)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("engine: run changed concurrently")
	}
	return e.addEvent(ctx, id, "", "", "run.state", map[string]string{"from": string(Gates), "to": string(Done), "commit": evidence.Commit})
}

func (e *Engine) addEvent(ctx context.Context, runID, taskID, dispatchID, kind string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = e.store.DB().ExecContext(ctx, `INSERT INTO events(run_id,task_id,dispatch_id,type,payload,created_at) VALUES(?,?,?,?,?,?)`, runID, taskID, dispatchID, kind, string(data), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("run_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
func hasCycle(tasks []Task) bool {
	byID := map[string]Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		for _, d := range byID[id].Dependencies {
			if visit(d) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	for _, t := range tasks {
		if visit(t.ID) {
			return true
		}
	}
	return false
}
