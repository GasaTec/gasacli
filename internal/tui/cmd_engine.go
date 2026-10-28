package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/adapters"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/orchestration"
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
	run, err := orchestration.CreateRun(ctx, core, project, goal)
	if err != nil {
		closeStore()
		return "", err
	}
	reviewer := orchestration.SelectReviewer(discoveryResults(cc.DiscoveryResults()), agent)
	profile, _ := permissions.New(cc.ConfigHome(), trust.NewStore(cc.ConfigHome())).Profile()
	cc.Emit(i18n.T("goal.plan", run.ID, goal, agent, reviewer, profile))
	managerParent := context.Background()
	if sessionContext, ok := cc.(*sessionCommandContext); ok && sessionContext.session != nil && sessionContext.session.Context != nil {
		managerParent = sessionContext.session.Context
	}
	managerCtx, cancel := context.WithCancel(managerParent)
	activeRuns.Store(run.ID, activeRun{cancel: cancel, project: project})
	go func() {
		defer closeStore()
		defer activeRuns.Delete(run.ID)
		defer cancel()
		runAsync(managerCtx, cc, core, run.ID, project, goal, agent, reviewer)
	}()
	return i18n.T("goal.started", run.ID, run.ID), nil
}

func runAsync(ctx context.Context, cc CommandContext, core *engine.Engine, runID, project, goal, agent, reviewer string) {
	started := time.Now()
	stopEvents := make(chan struct{})
	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		var after int64
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopEvents:
				emitRunEvents(core, runID, &after, cc)
				return
			case <-ticker.C:
				emitRunEvents(core, runID, &after, cc)
			}
		}
	}()
	policy := permissions.New(cc.ConfigHome(), trust.NewStore(cc.ConfigHome()))
	runApprover := engineCommandApprover{cc: cc, agent: agent, approvals: &sync.Map{}}
	err := orchestration.Run(ctx, runID, orchestration.Options{
		Core: core, Policy: policy, Approver: runApprover, Home: cc.ConfigHome(), Project: project,
		Goal: goal, Developer: agent, Reviewer: reviewer, MaxFixRetries: 2, Scheduler: engine.SchedulerOptions{Workers: 3, MaxRetries: 2},
		OnEvent: func(role string, event adapters.Event) {
			cc.Emit(i18n.T("orchestration.event", runID, role, event.Type))
		},
	})
	close(stopEvents)
	<-eventsDone
	if err != nil {
		cc.Emit(i18n.T("goal.blocked", runID, statusAfter(core, runID), err))
		return
	}
	cc.Emit(i18n.T("goal.event", runID, fmt.Sprintf("elapsed %.1fs", time.Since(started).Seconds())))
	cc.Emit(i18n.T("goal.done", runID))
}

func emitRunEvents(core *engine.Engine, runID string, after *int64, cc CommandContext) {
	events, err := core.Events(context.Background(), runID, *after, 100)
	if err != nil {
		return
	}
	for _, event := range events {
		*after = event.ID
		if event.Type == "agent.event" {
			continue
		}
		var data map[string]any
		_ = json.Unmarshal(event.Payload, &data)
		var detail string
		switch event.Type {
		case "agent.finished":
			duration := number(data["duration_ms"]) / 1000
			if data["cost_known"] == true {
				detail = fmt.Sprintf("%s: %s, cost $%.4f, %.1fs", data["role"], data["agent"], data["cost_usd"], duration)
			} else {
				detail = fmt.Sprintf("%s: %s, cost unknown, %.1fs", data["role"], data["agent"], duration)
			}
		default:
			detail = event.Type + " " + strings.TrimSpace(string(event.Payload))
		}
		cc.Emit(i18n.T("goal.event", runID, detail))
	}
}

func number(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0
	}
}

func statusCommand(ctx context.Context, cc CommandContext, _ []string) (string, error) {
	project, err := trust.Canonical(cc.ProjectPath())
	if err != nil {
		return "", err
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
	for _, run := range runs {
		if runBelongsToProject(run, project) {
			return fmt.Sprintf("Run %s: %s — %s", run.ID, run.Status, run.Goal), nil
		}
	}
	return "No orchestration run for this project.", nil
}

func resumeCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if !cc.Trusted() {
		return "", errors.New("trust this project before resuming a run")
	}
	project, err := trust.Canonical(cc.ProjectPath())
	if err != nil {
		return "", err
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
			if runBelongsToProject(candidate, project) && (candidate.Status == engine.Blocked || candidate.Status == engine.Running || candidate.Status == engine.Planning || candidate.Status == engine.Fixing || candidate.Status == engine.Gates || candidate.Status == engine.Review) {
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
	if !runBelongsToProject(run, project) {
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
	managerParent := context.Background()
	if sessionContext, ok := cc.(*sessionCommandContext); ok && sessionContext.session != nil && sessionContext.session.Context != nil {
		managerParent = sessionContext.session.Context
	}
	managerCtx, cancel := context.WithCancel(managerParent)
	activeRuns.Store(run.ID, activeRun{cancel: cancel, project: project})
	go func() {
		defer closeStore()
		defer activeRuns.Delete(run.ID)
		defer cancel()
		runAsync(managerCtx, cc, core, run.ID, project, run.Goal, agent, orchestration.SelectReviewer(discoveryResults(cc.DiscoveryResults()), agent))
	}()
	return fmt.Sprintf("Resuming run %s.", run.ID), nil
}

func cancelCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if len(args) > 1 {
		return "", errors.New("usage: /cancel [run-id]")
	}
	project, err := trust.Canonical(cc.ProjectPath())
	if err != nil {
		return "", err
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
			if runBelongsToProject(run, project) && (run.Status == engine.Running || run.Status == engine.Planning || run.Status == engine.TrustPending || run.Status == engine.Fixing || run.Status == engine.Gates || run.Status == engine.Review) {
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
	if !runBelongsToProject(run, project) {
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
	project, err := trust.Canonical(cc.ProjectPath())
	if err != nil {
		return "", err
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
		if runBelongsToProject(run, project) {
			lines = append(lines, fmt.Sprintf("%s  %-12s  %s", run.ID, run.Status, run.Goal))
		}
	}
	if len(lines) == 0 {
		return "No orchestration runs.", nil
	}
	return strings.Join(lines, "\n"), nil
}

func runBelongsToProject(run engine.Run, project string) bool {
	canonical, err := trust.Canonical(run.Project)
	return err == nil && canonical == project
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
	cc        CommandContext
	agent     string
	approvals *sync.Map
}

func (a engineCommandApprover) Ask(ctx context.Context, request permissions.Request) (permissions.Decision, error) {
	exactCommand := request.Command
	if exactCommand == "" {
		exactCommand = string(request.Action) + " " + request.Target
	}
	permissionRequest := PermissionRequest{Agent: a.agent, Action: string(request.Action), Target: request.Target, ExactCommand: exactCommand, RunID: request.RunID}
	cacheKey := request.RunID + "\x00" + string(request.Action) + "\x00" + request.Target
	if a.approvals != nil {
		if _, ok := a.approvals.Load(cacheKey); ok {
			return permissions.Decision{Effect: permissions.Allow, Reason: "approved for this run", Rule: request.Action, Scope: permissions.Scope("run")}, nil
		}
	}
	var decision permissions.Decision
	var err error
	if scoped, ok := a.cc.(interface {
		AskScoped(context.Context, PermissionRequest) (permissions.Decision, error)
	}); ok {
		decision, err = scoped.AskScoped(ctx, permissionRequest)
		if err == nil {
			if emitter, ok := a.cc.(interface{ Emit(string) }); ok {
				emitter.Emit(i18n.T("goal.event", request.RunID, string(request.Action)+": "+string(decision.Effect)+" ("+string(decision.Scope)+")"))
			}
		}
		if err == nil && decision.Scope == permissions.Scope("run") && a.approvals != nil {
			a.approvals.Store(cacheKey, struct{}{})
		}
		return decision, err
	}
	var approved bool
	if contextual, ok := a.cc.(interface {
		AskContext(context.Context, PermissionRequest) (bool, error)
	}); ok {
		approved, err = contextual.AskContext(ctx, permissionRequest)
	} else {
		approved, err = a.cc.Ask(permissionRequest)
	}
	if err != nil {
		return permissions.Decision{Effect: permissions.Deny, Reason: "approval failed", Rule: request.Action}, err
	}
	effect := permissions.Deny
	reason := "user denied operation"
	if approved {
		effect = permissions.Allow
		reason = "user approved operation"
	}
	decision = permissions.Decision{Effect: effect, Reason: reason, Rule: request.Action, Scope: permissions.ProjectScope}
	return decision, nil
}

func discoveryResults(value any) []discovery.Result {
	results, _ := value.([]discovery.Result)
	return results
}
