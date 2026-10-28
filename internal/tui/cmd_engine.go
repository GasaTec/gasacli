package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/adapters"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/storage"
	"github.com/GasaTec/gasacli/internal/trust"
)

type activeRun struct {
	cancel  context.CancelFunc
	project string
}

var activeRuns sync.Map

func goalCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if !cc.Trusted() {
		return "", errors.New("trust this project before starting an orchestrated run")
	}
	goal := strings.TrimSpace(strings.Join(args, " "))
	if goal == "" {
		return "", errors.New("usage: /goal <text>")
	}
	agent := selectRunAgent(cc.DiscoveryResults())
	if agent == "" {
		return "", errors.New("no supported coding CLI is installed")
	}
	core, closeStore, err := openCommandEngine(cc)
	if err != nil {
		return "", err
	}
	project, err := trust.Canonical(cc.ProjectPath())
	if err != nil {
		closeStore()
		return "", err
	}
	run, err := core.CreateRun(ctx, project, goal)
	if err != nil {
		closeStore()
		return "", err
	}
	if err = core.AddTasks(ctx, run.ID, []engine.Task{{ID: "task-1", RunID: run.ID, Name: "Implement requested goal", Timeout: 30 * time.Minute}}); err != nil {
		closeStore()
		return "", err
	}
	cc.Emit(fmt.Sprintf("Plan for run %s: implement the goal in one task. Agent: %s.", run.ID, agent))
	managerCtx, cancel := context.WithCancel(ctx)
	activeRuns.Store(run.ID, activeRun{cancel: cancel, project: project})
	go func() {
		defer closeStore()
		defer activeRuns.Delete(run.ID)
		defer cancel()
		runAsync(managerCtx, cc, core, run.ID, project, goal, agent)
	}()
	return fmt.Sprintf("Run %s started. Use /status, /runs, /cancel %s, or /resume %s.", run.ID, run.ID, run.ID), nil
}

func runAsync(ctx context.Context, cc CommandContext, core *engine.Engine, runID, project, goal, agent string) {
	policy := permissions.New(cc.ConfigHome(), trust.NewStore(cc.ConfigHome()))
	executor := &adapters.Executor{Policy: policy, Approver: engineCommandApprover{cc: cc, agent: agent}, OnEvent: func(event adapters.Event) { cc.Emit(fmt.Sprintf("Run %s: agent event %s", runID, event.Type)) }}
	worker := adapterWorker{executor: executor, project: project, goal: goal, agent: agent}
	if err := core.RunDAG(ctx, runID, worker, engine.SchedulerOptions{Workers: 3, MaxRetries: 2}); err != nil {
		cc.Emit(fmt.Sprintf("Run %s is %s: %s", runID, statusAfter(core, runID), err))
		return
	}
	_ = core.Transition(context.Background(), runID, engine.Blocked, "independent review and configured quality gates are required before completion")
	cc.Emit(fmt.Sprintf("Run %s is BLOCKED pending configured checks and independent review.", runID))
}

func statusCommand(ctx context.Context, cc CommandContext, _ []string) (string, error) {
	core, closeStore, err := openCommandEngine(cc)
	if err != nil {
		return "", err
	}
	defer closeStore()
	runs, err := core.ListRuns(ctx, 100)
	if err != nil {
		return "", err
	}
	for _, run := range runs {
		if run.Project == cc.ProjectPath() {
			return fmt.Sprintf("Run %s: %s — %s", run.ID, run.Status, run.Goal), nil
		}
	}
	return "No orchestration run for this project.", nil
}

func resumeCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if !cc.Trusted() {
		return "", errors.New("trust this project before resuming a run")
	}
	core, closeStore, err := openCommandEngine(cc)
	if err != nil {
		return "", err
	}
	var run engine.Run
	if len(args) > 1 {
		return "", errors.New("usage: /resume [run-id]")
	}
	if len(args) == 1 {
		run, err = core.GetRun(ctx, args[0])
	} else {
		runs, listErr := core.ListRuns(ctx, 100)
		if listErr != nil {
			closeStore()
			return "", listErr
		}
		for _, candidate := range runs {
			if candidate.Project == cc.ProjectPath() && (candidate.Status == engine.Blocked || candidate.Status == engine.Running || candidate.Status == engine.Planning) {
				run = candidate
				break
			}
		}
		if run.ID == "" {
			closeStore()
			return "", errors.New("no resumable run for this project")
		}
	}
	if err != nil {
		closeStore()
		return "", err
	}
	if run.Project != cc.ProjectPath() {
		closeStore()
		return "", errors.New("run belongs to another project")
	}
	if run.Status == engine.Done {
		closeStore()
		return fmt.Sprintf("Run %s is already DONE.", run.ID), nil
	}
	agent := selectRunAgent(cc.DiscoveryResults())
	if agent == "" {
		closeStore()
		return "", errors.New("no supported coding CLI is installed")
	}
	project, err := trust.Canonical(cc.ProjectPath())
	if err != nil {
		closeStore()
		return "", err
	}
	managerCtx, cancel := context.WithCancel(ctx)
	activeRuns.Store(run.ID, activeRun{cancel: cancel, project: project})
	go func() {
		defer closeStore()
		defer activeRuns.Delete(run.ID)
		defer cancel()
		runAsync(managerCtx, cc, core, run.ID, project, run.Goal, agent)
	}()
	return fmt.Sprintf("Resuming run %s.", run.ID), nil
}

func cancelCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if len(args) > 1 {
		return "", errors.New("usage: /cancel [run-id]")
	}
	core, closeStore, err := openCommandEngine(cc)
	if err != nil {
		return "", err
	}
	defer closeStore()
	id := ""
	if len(args) == 1 {
		id = args[0]
	} else {
		runs, err := core.ListRuns(ctx, 100)
		if err != nil {
			return "", err
		}
		for _, run := range runs {
			if run.Project == cc.ProjectPath() && (run.Status == engine.Running || run.Status == engine.Planning || run.Status == engine.TrustPending) {
				id = run.ID
				break
			}
		}
	}
	if id == "" {
		return "", errors.New("no active run for this project")
	}
	run, err := core.GetRun(ctx, id)
	if err != nil {
		return "", err
	}
	if run.Project != cc.ProjectPath() {
		return "", errors.New("run belongs to another project")
	}
	if value, ok := activeRuns.Load(id); ok {
		value.(activeRun).cancel()
	}
	if run.Status != engine.Cancelled && run.Status != engine.Done {
		if err = core.CancelRun(ctx, id); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("Cancellation requested for run %s.", id), nil
}

func runsCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if len(args) > 0 {
		return "", errors.New("usage: /runs")
	}
	core, closeStore, err := openCommandEngine(cc)
	if err != nil {
		return "", err
	}
	defer closeStore()
	runs, err := core.ListRuns(ctx, 100)
	if err != nil {
		return "", err
	}
	lines := []string{}
	for _, run := range runs {
		if run.Project == cc.ProjectPath() {
			lines = append(lines, fmt.Sprintf("%s  %-12s  %s", run.ID, run.Status, run.Goal))
		}
	}
	if len(lines) == 0 {
		return "No orchestration runs.", nil
	}
	return strings.Join(lines, "\n"), nil
}

func openCommandEngine(cc CommandContext) (*engine.Engine, func(), error) {
	if cc.ConfigHome() == "" {
		return nil, func() {}, errors.New("configuration home unavailable")
	}
	store, err := storage.Open(filepath.Join(cc.ConfigHome(), "gasacli.db"))
	if err != nil {
		return nil, func() {}, err
	}
	core, err := engine.New(store)
	if err != nil {
		_ = store.Close()
		return nil, func() {}, err
	}
	return core, func() { _ = store.Close() }, nil
}

func selectRunAgent(value any) string {
	results, ok := value.([]discovery.Result)
	if !ok {
		return ""
	}
	for _, id := range []string{"codex", "claude"} {
		for _, result := range results {
			if result.ID == id && (result.Status == discovery.Ready || result.Status == discovery.Installed) {
				return id
			}
		}
	}
	return ""
}
func statusAfter(core *engine.Engine, id string) engine.RunStatus {
	run, err := core.GetRun(context.Background(), id)
	if err != nil {
		return engine.Blocked
	}
	return run.Status
}

type engineCommandApprover struct {
	cc    CommandContext
	agent string
}

func (a engineCommandApprover) Ask(ctx context.Context, request permissions.Request) (permissions.Decision, error) {
	approved, err := a.cc.Ask(PermissionRequest{Agent: a.agent, Action: string(request.Action), Target: request.Target, ExactCommand: "coding agent execution"})
	if err != nil {
		return permissions.Decision{Effect: permissions.Deny, Reason: "approval failed", Rule: request.Action}, err
	}
	effect := permissions.Deny
	reason := "user denied operation"
	if approved {
		effect = permissions.Allow
		reason = "user approved operation"
	}
	return permissions.Decision{Effect: effect, Reason: reason, Rule: request.Action, Scope: permissions.ProjectScope}, nil
}

type adapterWorker struct {
	executor             *adapters.Executor
	project, goal, agent string
}

func (w adapterWorker) Run(ctx context.Context, task engine.Task) error {
	result, err := w.executor.Execute(ctx, adapters.Request{Agent: w.agent, Project: w.project, Prompt: w.goal, Timeout: task.Timeout})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("agent exited with code %d", result.ExitCode)
	}
	return nil
}
