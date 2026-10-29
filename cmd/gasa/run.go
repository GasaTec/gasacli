package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/orchestration"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/storage"
	"github.com/GasaTec/gasacli/internal/trust"
	"golang.org/x/term"
)

func runOrchestration(ctx context.Context, args []string) error {
	if len(args) == 0 || strings.TrimSpace(strings.Join(args, " ")) == "" {
		return errors.New(`usage: gasa run [--agent <id>] [--reviewer <id>] "<goal>"`)
	}
	goal, requestedAgent, requestedReviewer, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	if goal == "" {
		return errors.New(`usage: gasa run [--agent <id>] [--reviewer <id>] "<goal>"`)
	}
	project, err := os.Getwd()
	if err != nil {
		return err
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	trusted, err := trust.NewStore(home).IsTrusted(project)
	if err != nil {
		return err
	}
	if !trusted {
		return errors.New("gasa run requires a trusted project directory")
	}
	project, err = trust.Canonical(project)
	if err != nil {
		return err
	}
	store, err := storage.Open(filepath.Join(home, "gasacli.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	core, err := engine.New(store)
	if err != nil {
		return err
	}
	cfg, err := config.LoadFrom(home)
	if err != nil {
		return err
	}
	results := discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{})
	preferredAgent := requestedAgent
	if preferredAgent == "" {
		preferredAgent = cfg.Agents.Developer
	}
	agent := orchestration.SelectDeveloper(results, preferredAgent)
	if agent == "" {
		return unavailableAgentError("developer", preferredAgent, results)
	}
	preferredReviewer := requestedReviewer
	if preferredReviewer == "" {
		preferredReviewer = cfg.Agents.Reviewer
	}
	reviewer := orchestration.SelectReviewer(results, agent)
	if preferredReviewer != "" {
		if !orchestration.AgentAvailable(results, preferredReviewer) {
			return unavailableAgentError("reviewer", preferredReviewer, results)
		}
		reviewer = preferredReviewer
	}
	policy := permissions.New(home, trust.NewStore(home))
	approver := interactiveApprover{in: os.Stdin, out: os.Stderr, policy: policy, approvals: &sync.Map{}}
	return runGoal(ctx, core, policy, approver, home, project, goal, agent, reviewer)
}

func parseRunArgs(args []string) (goal, agent, reviewer string, err error) {
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent", "--reviewer":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", "", "", fmt.Errorf("%s requires an agent id", args[i])
			}
			i++
			if args[i-1] == "--agent" {
				agent = args[i]
			} else {
				reviewer = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "--") {
				return "", "", "", fmt.Errorf("unknown gasa run option %q", args[i])
			}
			remaining = append(remaining, args[i])
		}
	}
	return strings.TrimSpace(strings.Join(remaining, " ")), agent, reviewer, nil
}

func unavailableAgentError(role, agent string, results []discovery.Result) error {
	if agent == "" {
		return errors.New("no ready or installed supported coding CLI is available")
	}
	status := orchestration.AgentStatus(results, agent)
	return fmt.Errorf("configured %s agent %q is not available (discovery status: %s)", role, agent, status)
}

func runGoal(ctx context.Context, core *engine.Engine, policy *permissions.Engine, approver permissions.Approver, home, project, goal, agent, reviewer string) error {
	run, err := orchestration.CreateRun(ctx, core, project, goal)
	if err != nil {
		return err
	}
	profile, profileErr := policy.Profile()
	if profileErr != nil {
		return profileErr
	}
	fmt.Println(i18n.T("goal.plan", shortRunID(run.ID), goal, agent, reviewer, profile))
	return orchestration.Run(ctx, run.ID, orchestration.Options{Core: core, Policy: policy, Approver: approver, Home: home, Project: project, Goal: goal, Developer: agent, Reviewer: reviewer, MaxFixRetries: 2, Scheduler: engine.SchedulerOptions{Workers: 1, MaxRetries: 2}})
}

func runRuns(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: gasa runs")
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	store, err := storage.Open(filepath.Join(home, "gasacli.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	core, err := engine.New(store)
	if err != nil {
		return err
	}
	runs, err := core.ListRuns(context.Background(), 100)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Println("No orchestration runs.")
		return nil
	}
	for _, run := range runs {
		fmt.Printf("%s  %-12s  %s\n", shortRunID(run.ID), run.Status, run.Goal)
	}
	return nil
}

func runResume(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: gasa resume <run-id>")
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	store, err := storage.Open(filepath.Join(home, "gasacli.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	core, err := engine.New(store)
	if err != nil {
		return err
	}
	run, err := core.GetRun(ctx, args[0])
	if err != nil {
		return err
	}
	project, err := os.Getwd()
	if err != nil {
		return err
	}
	project, err = trust.Canonical(project)
	if err != nil {
		return err
	}
	if project != run.Project {
		return errors.New("resume must run from the original project directory")
	}
	trusted, err := trust.NewStore(home).IsTrusted(project)
	if err != nil {
		return err
	}
	if !trusted {
		return errors.New("resume requires a trusted project directory")
	}
	if run.Status == engine.Done {
		fmt.Printf("Run %s is already DONE.\n", shortRunID(run.ID))
		return nil
	}
	results := discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{})
	cfg, err := config.LoadFrom(home)
	if err != nil {
		return err
	}
	agent := orchestration.SelectDeveloper(results, cfg.Agents.Developer)
	if agent == "" {
		return unavailableAgentError("developer", cfg.Agents.Developer, results)
	}
	reviewer := orchestration.SelectReviewer(results, agent)
	if cfg.Agents.Reviewer != "" {
		if !orchestration.AgentAvailable(results, cfg.Agents.Reviewer) {
			return unavailableAgentError("reviewer", cfg.Agents.Reviewer, results)
		}
		reviewer = cfg.Agents.Reviewer
	}
	policy := permissions.New(home, trust.NewStore(home))
	return orchestration.Run(ctx, run.ID, orchestration.Options{Core: core, Policy: policy, Approver: interactiveApprover{in: os.Stdin, out: os.Stderr, policy: policy, approvals: &sync.Map{}}, Home: home, Project: project, Goal: run.Goal, Developer: agent, Reviewer: reviewer, MaxFixRetries: 2, Scheduler: engine.SchedulerOptions{Workers: 1, MaxRetries: 2}})
}

func shortRunID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

type interactiveApprover struct {
	in        *os.File
	out       *os.File
	policy    *permissions.Engine
	approvals *sync.Map
}

func (a interactiveApprover) Ask(ctx context.Context, request permissions.Request) (permissions.Decision, error) {
	decision := permissions.Decision{Effect: permissions.Deny, Reason: i18n.T("goal.cli.no_tty", request.Action, permissions.RedactTarget(request.Project)), Rule: request.Action}
	cacheKey := request.RunID + "\x00" + string(request.Action) + "\x00" + request.Target
	if a.approvals != nil {
		if _, ok := a.approvals.Load(cacheKey); ok {
			return permissions.Decision{Effect: permissions.Allow, Reason: i18n.T("goal.approval.run"), Rule: request.Action, Scope: permissions.Scope("run")}, nil
		}
	}
	if !term.IsTerminal(int(a.in.Fd())) {
		if a.policy != nil {
			_ = a.record(request, decision)
		}
		return decision, nil
	}
	fmt.Fprint(a.out, i18n.T("goal.cli.approval", request.Action, permissions.RedactTarget(request.Project)))
	line, err := bufio.NewReader(a.in).ReadString('\n')
	if err != nil && ctx.Err() != nil {
		return decision, ctx.Err()
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "o" || answer == "u" || answer == "y" {
		decision.Effect = permissions.Allow
		decision.Reason = i18n.T("goal.approval.once")
	} else if answer == "r" || answer == "e" {
		decision.Effect = permissions.Allow
		decision.Reason = i18n.T("goal.approval.run")
		decision.Scope = permissions.Scope("run")
		if a.approvals != nil {
			a.approvals.Store(cacheKey, struct{}{})
		}
	}
	if a.policy != nil {
		if err := a.record(request, decision); err != nil {
			return permissions.Decision{Effect: permissions.Deny, Reason: "approval log unavailable", Rule: request.Action}, err
		}
	}
	return decision, nil
}

func (a interactiveApprover) record(request permissions.Request, decision permissions.Decision) error {
	if a.policy == nil {
		return nil
	}
	event := permissions.ApprovalEvent{Decision: decision.Effect, Permission: request.Action, PolicyRevision: "current", Reason: decision.Reason}
	event.Actor.Kind = "user"
	event.Requester.AgentID, event.Requester.RunID = request.Agent, request.RunID
	event.Scope.Kind, event.Scope.ID = string(decision.Scope), request.Project
	if decision.Scope == permissions.Scope("run") {
		event.Scope.Kind, event.Scope.ID = "run", request.RunID
	}
	event.Operation.Summary = permissions.RedactTarget(string(request.Action) + " " + request.Target)
	return a.policy.RecordApproval(event)
}
