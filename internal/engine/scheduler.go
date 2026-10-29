package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/permissions"
)

type SchedulerOptions struct {
	Workers    int
	MaxRetries int
	Now        func() time.Time
}

// ErrNoRetry marks a task failure that has already had its one launch retry.
var ErrNoRetry = errors.New("engine: task must not be retried")

// RunDAG schedules ready tasks in insertion order and retries failures up to
// MaxRetries. A run remains in GATES after execution; a separate quality gate
// and reviewer must approve it before it can become DONE.
func (e *Engine) RunDAG(ctx context.Context, runID string, worker Worker, options SchedulerOptions) error {
	if worker == nil {
		return errors.New("engine: worker is required")
	}
	if options.Workers < 1 {
		options.Workers = 3
	}
	if options.Workers > 32 {
		return errors.New("engine: worker limit cannot exceed 32")
	}
	if options.MaxRetries < 0 || options.MaxRetries > 10 {
		return errors.New("engine: retry limit must be between 0 and 10")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	run, err := e.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != Planning && run.Status != Running && run.Status != Blocked && run.Status != Fixing {
		return fmt.Errorf("engine: run %s cannot execute from %s", runID, run.Status)
	}
	if run.Status == Planning || run.Status == Blocked || run.Status == Fixing {
		if err := e.Transition(ctx, runID, Running, ""); err != nil {
			return err
		}
	}
	if _, err := e.store.DB().ExecContext(ctx, `UPDATE tasks SET status=? WHERE run_id=? AND status=?`, Queued, runID, Executing); err != nil {
		return err
	}
	if _, err := e.store.DB().ExecContext(ctx, `UPDATE tasks SET status=? WHERE run_id=? AND status=?`, Queued, runID, TaskBlocked); err != nil {
		return err
	}
	tasks, err := e.Tasks(ctx, runID)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return errors.New("engine: run has no tasks")
	}
	byID := make(map[string]Task, len(tasks))
	indexByID := make(map[string]int, len(tasks))
	for index, t := range tasks {
		if t.Status == Executing {
			t.Status = Queued
			tasks[index] = t
		}
		byID[t.ID] = t
		indexByID[t.ID] = index
	}
	for _, t := range tasks {
		for _, dep := range t.Dependencies {
			if _, ok := byID[dep]; !ok {
				return fmt.Errorf("engine: task %s has missing dependency %s", t.ID, dep)
			}
		}
	}
	if hasCycle(tasks) {
		return errors.New("engine: task dependencies contain a cycle")
	}
	ready := make(chan Task, options.Workers)
	results := make(chan taskResult, options.Workers)
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var pool sync.WaitGroup
	for i := 0; i < options.Workers; i++ {
		pool.Add(1)
		go func() {
			defer pool.Done()
			for task := range ready {
				results <- e.executeTask(workerCtx, runID, task, worker, options)
			}
		}()
	}
	active, complete := 0, 0
	for _, t := range tasks {
		if t.Status == TaskDone {
			complete++
		} else if t.Status == TaskBlocked || t.Status == TaskCancelled {
			close(ready)
			cancel()
			pool.Wait()
			return fmt.Errorf("engine: task %s is %s", t.ID, t.Status)
		}
	}
	defer func() {
		cancel()
		close(ready)
		pool.Wait()
		if ctx.Err() != nil {
			_ = e.CancelRun(context.Background(), runID)
		}
	}()
	dispatched := map[string]bool{}
	for complete < len(tasks) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		progress := false
		for _, t := range tasks {
			if t.Status != Queued || dispatched[t.ID] {
				continue
			}
			depsReady := true
			for _, d := range t.Dependencies {
				if byID[d].Status != TaskDone {
					depsReady = false
					break
				}
			}
			if depsReady && active < options.Workers {
				dispatched[t.ID] = true
				active++
				ready <- t
				progress = true
			}
		}
		if active == 0 && !progress {
			_ = e.Transition(context.Background(), runID, Blocked, "no runnable task; dependencies are incomplete")
			return errors.New("engine: DAG stalled")
		}
		if active == 0 {
			continue
		}
		result := <-results
		active--
		if result.err != nil {
			if errors.Is(result.err, context.Canceled) && ctx.Err() != nil {
				return ctx.Err()
			}
			if result.task.Retries < options.MaxRetries && !errors.Is(result.err, ErrNoRetry) {
				result.task.Retries++
				result.task.Status = Queued
				tasks[indexByID[result.task.ID]] = result.task
				byID[result.task.ID] = result.task
				dispatched[result.task.ID] = false
				if err := e.setTask(ctx, result.task, Queued); err != nil {
					return err
				}
				continue
			}
			_ = e.setTask(context.Background(), result.task, TaskBlocked)
			reason := permissions.RedactTarget(fmt.Sprintf("task %s failed after %d retries: %v", result.task.ID, result.task.Retries, result.err))
			_ = e.Transition(context.Background(), runID, Blocked, reason)
			return fmt.Errorf("engine: %s", reason)
		}
		result.task.Status = TaskDone
		if err := e.setTask(ctx, result.task, TaskDone); err != nil {
			return err
		}
		byID[result.task.ID] = result.task
		complete++
	}
	return e.Transition(ctx, runID, Gates, "")
}

type taskResult struct {
	task Task
	err  error
}

func (e *Engine) executeTask(parent context.Context, runID string, task Task, worker Worker, options SchedulerOptions) taskResult {
	ctx := parent
	cancel := func() {}
	if task.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, task.Timeout)
	}
	defer cancel()
	var previousAttempt int
	if err := e.store.DB().QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt),0) FROM dispatches WHERE task_id=?`, task.ID).Scan(&previousAttempt); err != nil {
		return taskResult{task: task, err: err}
	}
	attempt := previousAttempt + 1
	id := newID()
	started := options.Now().UTC().Format(time.RFC3339Nano)
	_, err := e.store.DB().ExecContext(ctx, `INSERT INTO dispatches(id,task_id,attempt,status,started_at) VALUES(?,?,?,?,?)`, id, task.ID, attempt, DispatchRunning, started)
	if err != nil {
		return taskResult{task: task, err: err}
	}
	if err = e.setTask(ctx, task, Executing); err != nil {
		return taskResult{task: task, err: err}
	}
	_ = e.addEvent(ctx, runID, task.ID, id, "dispatch.started", map[string]any{"attempt": attempt})
	err = worker.Run(ctx, task)
	status := DispatchSucceeded
	if err != nil {
		status = DispatchFailed
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = DispatchTimedOut
			err = ctx.Err()
		} else if errors.Is(ctx.Err(), context.Canceled) {
			status = DispatchCancelled
			err = ctx.Err()
		}
	}
	finished := options.Now().UTC().Format(time.RFC3339Nano)
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	_, persistErr := e.store.DB().ExecContext(context.Background(), `UPDATE dispatches SET status=?,finished_at=?,reason=? WHERE id=?`, status, finished, reason, id)
	if persistErr != nil {
		return taskResult{task: task, err: persistErr}
	}
	_ = e.addEvent(context.Background(), runID, task.ID, id, "dispatch.finished", map[string]string{"status": string(status), "reason": reason})
	return taskResult{task: task, err: err}
}

func (e *Engine) setTask(ctx context.Context, task Task, status TaskStatus) error {
	err := e.store.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET status=?,retries=?,updated_at=? WHERE id=?`, status, task.Retries, time.Now().UTC().Format(time.RFC3339Nano), task.ID)
		return err
	})
	if err != nil {
		return err
	}
	return e.addEvent(ctx, task.RunID, task.ID, "", "task.state", map[string]string{"status": string(status)})
}
