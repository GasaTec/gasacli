// Package orchestration connects the durable engine to project gates and
// isolated developer/reviewer CLI sessions.
package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/adapters"
	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/prompts"
)

type Options struct {
	Core          *engine.Engine
	Policy        *permissions.Engine
	Approver      permissions.Approver
	Home          string
	Project       string
	Goal          string
	Developer     string
	Reviewer      string
	MaxFixRetries int
	Scheduler     engine.SchedulerOptions
	OnEvent       func(role string, event adapters.Event)
}

// CreateRun stores a run and its initial implementation task.
func CreateRun(ctx context.Context, core *engine.Engine, project, goal string) (engine.Run, error) {
	if core == nil {
		return engine.Run{}, errors.New("orchestration: engine is required")
	}
	run, err := core.CreateRun(ctx, project, goal)
	if err != nil {
		return engine.Run{}, err
	}
	if err = core.AddTasks(ctx, run.ID, []engine.Task{{ID: "implementation-" + run.ID, RunID: run.ID, Name: "Implement requested goal", Timeout: 30 * time.Minute}}); err != nil {
		return engine.Run{}, err
	}
	return run, nil
}

// Run executes implementation, configured project gates, and an independent
// read-only review. Review findings are sent back to a fresh developer session
// for a bounded number of fix cycles.
func Run(ctx context.Context, runID string, options Options) error {
	if options.Core == nil || options.Policy == nil || options.Home == "" || options.Project == "" || options.Goal == "" {
		return errors.New("orchestration: engine, policy, home, project, and goal are required")
	}
	if options.Developer == "" || options.Reviewer == "" {
		return errors.New("orchestration: developer and reviewer agents are required")
	}
	gates, err := config.LoadProjectGates(options.Project)
	if err != nil {
		return err
	}
	commands := gateCommands(gates)
	if len(commands) == 0 {
		err = errors.New(i18n.T("goal.gates.none"))
		_ = options.Core.Transition(context.Background(), runID, engine.Blocked, err.Error())
		return err
	}
	language := effectiveLanguage()
	developerPrompt, err := prompts.Effective(options.Home, options.Project, "developer", language)
	if err != nil {
		return fmt.Errorf("load developer prompt: %w", err)
	}
	reviewerPrompt, err := prompts.Effective(options.Home, options.Project, "tech-lead", language)
	if err != nil {
		return fmt.Errorf("load reviewer prompt: %w", err)
	}
	developerID := options.Developer + ":developer:" + runID
	reviewerID := options.Reviewer + ":reviewer:" + runID
	var reviewedDigest string
	newExecutor := func(role, agent string) (*adapters.Executor, *error) {
		recordErr := new(error)
		return &adapters.Executor{Policy: options.Policy, Approver: options.Approver, OnEvent: func(event adapters.Event) {
			if *recordErr == nil {
				detail := ""
				if event.Type == "launch.retry" {
					var payload struct {
						Reason string `json:"reason"`
					}
					_ = json.Unmarshal(event.Data, &payload)
					detail = payload.Reason
				}
				*recordErr = options.Core.RecordAgentEventDetail(context.Background(), runID, role, agent, event.Type, detail)
			}
			if options.OnEvent != nil {
				options.OnEvent(role, event)
			}
		}}, recordErr
	}
	return options.Core.RunVerified(ctx, runID, func(feedback string) engine.Worker {
		prompt := strings.TrimSpace(developerPrompt.Body + "\n\nGoal:\n" + options.Goal + "\n\n" + feedback)
		executor, recordErr := newExecutor("developer", options.Developer)
		return developerWorker{executor: executor, agent: options.Developer, project: options.Project, prompt: prompt, core: options.Core, runID: runID, eventErr: recordErr}
	}, func(gateCtx context.Context) ([]engine.GateResult, error) {
		gateRequest := permissions.Request{Action: "run_tests", Project: options.Project, Target: "gasacli/config.yaml", RunID: runID}
		decision := options.Policy.Decide(gateCtx, gateRequest)
		if decision.Effect == permissions.Ask {
			if options.Approver == nil {
				return nil, errors.New("orchestration: configured gates require approval")
			}
			var approvalErr error
			decision, approvalErr = options.Approver.Ask(gateCtx, gateRequest)
			if approvalErr != nil {
				return nil, approvalErr
			}
		}
		if decision.Effect != permissions.Allow {
			return nil, fmt.Errorf("orchestration: run_tests denied: %s", decision.Reason)
		}
		return options.Core.RunGates(gateCtx, runID, options.Project, commands)
	}, func(reviewCtx context.Context) (engine.ReviewResult, error) {
		before, err := snapshot(options.Project)
		if err != nil {
			return engine.ReviewResult{}, err
		}
		prompt := strings.Join([]string{
			reviewerPrompt.Body,
			"\nReview the current project changes for the goal below. Inspect the working tree, including untracked files. This is a separate read-only session; do not edit files. You have no developer conversation history. Return exactly one JSON object with schema {\"verdict\":\"approve\"|\"changes_requested\",\"findings\":[\"actionable finding\"]}. Use approve with an empty findings array only when the change meets the goal and has no blocking defects.",
			"Goal:\n" + options.Goal,
			"Reviewed change snapshot SHA-256: " + before,
		}, "\n\n")
		executor, eventErr := newExecutor("reviewer", options.Reviewer)
		result, err := executor.Execute(reviewCtx, adapters.Request{Agent: options.Reviewer, Project: options.Project, Prompt: prompt, RunID: runID, Timeout: 10 * time.Minute, ReadOnly: true})
		metricsErr := options.Core.RecordAgentMetrics(context.Background(), runID, agentMetrics("reviewer", options.Reviewer, result))
		if *eventErr != nil {
			return engine.ReviewResult{}, *eventErr
		}
		if metricsErr != nil {
			return engine.ReviewResult{}, metricsErr
		}
		if err != nil {
			return engine.ReviewResult{}, err
		}
		if result.TextTruncated {
			return engine.ReviewResult{}, errors.New("reviewer response exceeded the structured output limit")
		}
		review, err := engine.ParseReview(result.Text)
		if err != nil {
			return engine.ReviewResult{}, err
		}
		after, err := snapshot(options.Project)
		if err != nil {
			return engine.ReviewResult{}, err
		}
		if before != after {
			return engine.ReviewResult{}, errors.New("read-only reviewer changed the project")
		}
		reviewedDigest = "sha256:" + after
		return review, nil
	}, engine.VerifiedOptions{Scheduler: options.Scheduler, MaxFixRetries: options.MaxFixRetries, Developer: developerID, Reviewer: reviewerID, ReviewedChange: func() (string, error) {
		if reviewedDigest == "" {
			return "", errors.New("no reviewed project snapshot")
		}
		return reviewedDigest, nil
	}})
}

type developerWorker struct {
	executor *adapters.Executor
	agent    string
	project  string
	prompt   string
	core     *engine.Engine
	runID    string
	eventErr *error
}

func (w developerWorker) Run(ctx context.Context, _ engine.Task) error {
	result, err := w.executor.Execute(ctx, adapters.Request{Agent: w.agent, Project: w.project, Prompt: w.prompt, RunID: w.runID, Timeout: 30 * time.Minute})
	metricsErr := w.core.RecordAgentMetrics(context.Background(), w.runID, agentMetrics("developer", w.agent, result))
	if w.eventErr != nil && *w.eventErr != nil {
		return *w.eventErr
	}
	if metricsErr != nil {
		return metricsErr
	}
	if err != nil {
		if result.LaunchRetries > 0 {
			return fmt.Errorf("%w: %v", engine.ErrNoRetry, err)
		}
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("developer agent exited with code %d", result.ExitCode)
	}
	return nil
}

func agentMetrics(role, agent string, result adapters.Result) engine.AgentMetrics {
	return engine.AgentMetrics{Role: role, Agent: agent, ExitCode: result.ExitCode, EventCount: result.Events, OutputBytes: result.OutputBytes, DurationMS: result.Duration.Milliseconds(), CostUSD: result.CostUSD, CostKnown: result.CostKnown, InputTokens: result.InputTokens, OutputTokens: result.OutputTokens, FailureReason: result.FailureReason}
}

func gateCommands(gates config.ProjectGates) []engine.GateCommand {
	var result []engine.GateCommand
	for _, gate := range []struct {
		name string
		argv []string
	}{{"test", gates.Test}, {"build", gates.Build}, {"lint", gates.Lint}} {
		if len(gate.argv) > 0 {
			result = append(result, engine.GateCommand{Name: gate.name, Argv: append([]string(nil), gate.argv...)})
		}
	}
	return result
}

func effectiveLanguage() string {
	value := strings.TrimSpace(i18n.Language())
	if value == "en" || value == "pt-BR" {
		return value
	}
	return "pt-BR"
}

// SelectDeveloper prefers authenticated agents over installed agents whose
// authentication state is unknown. An explicit preference is returned only
// when it is currently usable.
func SelectDeveloper(results []discovery.Result, preferred string) string {
	if preferred != "" {
		if AgentAvailable(results, preferred) {
			return preferred
		}
		return ""
	}
	for _, status := range []discovery.Status{discovery.Ready, discovery.Installed} {
		for _, id := range []string{"codex", "claude"} {
			for _, result := range results {
				if result.ID == id && result.Status == status {
					return id
				}
			}
		}
	}
	return ""
}

func AgentStatus(results []discovery.Result, agent string) discovery.Status {
	for _, result := range results {
		if result.ID == agent {
			return result.Status
		}
	}
	return discovery.Missing
}

func AgentAvailable(results []discovery.Result, agent string) bool {
	if agent != "claude" && agent != "codex" {
		return false
	}
	status := AgentStatus(results, agent)
	return status == discovery.Ready || status == discovery.Installed
}

// SelectReviewer prefers a different ready agent, then a different installed
// agent, and finally the developer as an isolated fresh session.
func SelectReviewer(results []discovery.Result, developer string) string {
	for _, status := range []discovery.Status{discovery.Ready, discovery.Installed} {
		for _, result := range results {
			if result.ID != developer && AgentAvailable(results, result.ID) && result.Status == status {
				return result.ID
			}
		}
	}
	return developer
}
