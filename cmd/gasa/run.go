package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/adapters"
	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/storage"
	"github.com/GasaTec/gasacli/internal/trust"
	"golang.org/x/term"
)

func runOrchestration(ctx context.Context, args []string) error {
	if len(args) == 0 || strings.TrimSpace(strings.Join(args, " ")) == "" {
		return errors.New(`usage: gasa run "<goal>"`)
	}
	goal := strings.Join(args, " ")
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
	results := discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{})
	agent := selectAgent(results)
	if agent == "" {
		return errors.New("no supported coding CLI is installed")
	}
	policy := permissions.New(home, trust.NewStore(home))
	approver := interactiveApprover{in: os.Stdin, out: os.Stderr}
	return runGoal(ctx, core, policy, approver, project, goal, agent)
}

func runGoal(ctx context.Context, core *engine.Engine, policy *permissions.Engine, approver permissions.Approver, project, goal, agent string) error {
	run, err := core.CreateRun(ctx, project, goal)
	if err != nil {
		return err
	}
	task := engine.Task{ID: "task-1", RunID: run.ID, Name: "Implement requested goal", Timeout: 30 * time.Minute}
	if err = core.AddTasks(ctx, run.ID, []engine.Task{task}); err != nil {
		return err
	}
	fmt.Printf("Run %s — plan: implement the goal in one task.\n", run.ID)
	worker := runWorker{executor: &adapters.Executor{Policy: policy, Approver: approver}, project: project, agent: agent, goal: goal}
	err = core.RunDAG(ctx, run.ID, worker, engine.SchedulerOptions{Workers: 1, MaxRetries: 1})
	if err != nil {
		return fmt.Errorf("run %s blocked: %w", run.ID, err)
	}
	// This command intentionally leaves the run blocked until deterministic gates
	// and an independent review of the resulting diff are available.
	if err = core.Transition(ctx, run.ID, engine.Blocked, "independent review and configured quality gates are required before completion"); err != nil {
		return err
	}
	return fmt.Errorf("run %s is BLOCKED: independent review and configured quality gates are required", run.ID)
}

type runWorker struct {
	executor             *adapters.Executor
	project, agent, goal string
}

func (w runWorker) Run(ctx context.Context, task engine.Task) error {
	result, err := w.executor.Execute(ctx, adapters.Request{Agent: w.agent, Project: w.project, Prompt: w.goal, Profile: "balanced", Timeout: task.Timeout})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("agent exited with %d", result.ExitCode)
	}
	return nil
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
		fmt.Printf("%s  %-12s  %s\n", run.ID, run.Status, run.Goal)
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
		fmt.Printf("Run %s is already DONE.\n", run.ID)
		return nil
	}
	agent := selectAgent(discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{}))
	if agent == "" {
		return errors.New("no supported coding CLI is installed")
	}
	policy := permissions.New(home, trust.NewStore(home))
	worker := runWorker{executor: &adapters.Executor{Policy: policy, Approver: interactiveApprover{in: os.Stdin, out: os.Stderr}}, project: project, agent: agent, goal: run.Goal}
	if err = core.RunDAG(ctx, run.ID, worker, engine.SchedulerOptions{Workers: 1, MaxRetries: 1}); err != nil {
		return err
	}
	if err = core.Transition(ctx, run.ID, engine.Blocked, "independent review and configured quality gates are required before completion"); err != nil {
		return err
	}
	return fmt.Errorf("run %s resumed through execution and is BLOCKED pending independent review and configured quality gates", run.ID)
}

func selectAgent(results []discovery.Result) string {
	for _, id := range []string{"codex", "claude"} {
		for _, result := range results {
			if result.ID == id && (result.Status == discovery.Ready || result.Status == discovery.Installed) {
				return id
			}
		}
	}
	return ""
}

type interactiveApprover struct {
	in  *os.File
	out *os.File
}

func (a interactiveApprover) Ask(ctx context.Context, request permissions.Request) (permissions.Decision, error) {
	decision := permissions.Decision{Effect: permissions.Deny, Reason: "approval declined or unavailable", Rule: request.Action}
	if !term.IsTerminal(int(a.in.Fd())) {
		return decision, nil
	}
	fmt.Fprintf(a.out, "Allow %s for %s? [y/N] ", request.Action, permissions.RedactTarget(request.Project))
	line, err := bufio.NewReader(a.in).ReadString('\n')
	if err != nil && ctx.Err() != nil {
		return decision, ctx.Err()
	}
	if strings.EqualFold(strings.TrimSpace(line), "y") {
		decision.Effect = permissions.Allow
		decision.Reason = "approved for this run"
	}
	return decision, nil
}
