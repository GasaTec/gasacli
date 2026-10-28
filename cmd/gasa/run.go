package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	return runGoal(ctx, core, policy, approver, home, project, goal, agent, orchestration.SelectReviewer(results, agent))
}

func runGoal(ctx context.Context, core *engine.Engine, policy *permissions.Engine, approver permissions.Approver, home, project, goal, agent, reviewer string) error {
	run, err := orchestration.CreateRun(ctx, core, project, goal)
	if err != nil {
		return err
	}
	fmt.Println(i18n.T("orchestration.cli.plan", run.ID))
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
	results := discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{})
	agent := selectAgent(results)
	if agent == "" {
		return errors.New("no supported coding CLI is installed")
	}
	policy := permissions.New(home, trust.NewStore(home))
	return orchestration.Run(ctx, run.ID, orchestration.Options{Core: core, Policy: policy, Approver: interactiveApprover{in: os.Stdin, out: os.Stderr}, Home: home, Project: project, Goal: run.Goal, Developer: agent, Reviewer: orchestration.SelectReviewer(results, agent), MaxFixRetries: 2, Scheduler: engine.SchedulerOptions{Workers: 1, MaxRetries: 2}})
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
