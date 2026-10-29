package tui

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/adapters"
	"github.com/GasaTec/gasacli/internal/config"
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
	goal, requestedAgent, requestedReviewer, err := parseGoalArgs(args)
	if err != nil {
		return "", err
	}
	if goal == "" {
		return "", errors.New("usage: /goal [--agent <id>] <text>")
	}
	results := discoveryResults(cc.DiscoveryResults())
	cfg, err := config.LoadFrom(cc.ConfigHome())
	if err != nil {
		return "", err
	}
	preferredAgent := requestedAgent
	if preferredAgent == "" {
		preferredAgent = cfg.Agents.Developer
	}
	agent := orchestration.SelectDeveloper(results, preferredAgent)
	if agent == "" {
		return "", unavailableRunAgentError("developer", preferredAgent, results)
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
	baselineFiles, _ := projectFileState(project)
	run, err := orchestration.CreateRun(ctx, core, project, goal)
	if err != nil {
		closeStore()
		return "", err
	}
	reviewer := orchestration.SelectReviewer(results, agent)
	preferredReviewer := requestedReviewer
	if preferredReviewer == "" {
		preferredReviewer = cfg.Agents.Reviewer
	}
	if preferredReviewer != "" {
		if !orchestration.AgentAvailable(results, preferredReviewer) {
			closeStore()
			return "", unavailableRunAgentError("reviewer", preferredReviewer, results)
		}
		reviewer = preferredReviewer
	}
	profile, _ := permissions.New(cc.ConfigHome(), trust.NewStore(cc.ConfigHome())).Profile()
	cc.Emit(i18n.T("goal.plan", shortRunID(run.ID), goal, agent, reviewer, profile))
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
		runAsyncWithFiles(managerCtx, cc, core, run.ID, project, goal, agent, reviewer, baselineFiles)
	}()
	return i18n.T("goal.started", shortRunID(run.ID), shortRunID(run.ID)), nil
}

func parseGoalArgs(args []string) (goal, agent, reviewer string, err error) {
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent", "--reviewer":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", "", "", fmt.Errorf("%s requires an agent id", args[i])
			}
			option := args[i]
			i++
			if option == "--agent" {
				agent = args[i]
			} else {
				reviewer = args[i]
			}
		default:
			remaining = append(remaining, args[i])
		}
	}
	return strings.TrimSpace(strings.Join(remaining, " ")), agent, reviewer, nil
}

func unavailableRunAgentError(role, agent string, results []discovery.Result) error {
	if agent == "" {
		return errors.New("no ready or installed supported coding CLI is available")
	}
	return fmt.Errorf("configured %s agent %q is not available (discovery status: %s)", role, agent, orchestration.AgentStatus(results, agent))
}

func runAsync(ctx context.Context, cc CommandContext, core *engine.Engine, runID, project, goal, agent, reviewer string) {
	runAsyncWithFiles(ctx, cc, core, runID, project, goal, agent, reviewer, nil)
}

func runAsyncWithFiles(ctx context.Context, cc CommandContext, core *engine.Engine, runID, project, goal, agent, reviewer string, baseline map[string][32]byte) {
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
		OnAttempt: func(role, agent string, event adapters.AttemptEvent) {
			if current, ok := cc.(*sessionCommandContext); ok && current.attempt != nil {
				current.attempt(runID, role, agent, event)
			}
		},
		OnReviewProgress: func(elapsed time.Duration) {
			minutes := int(elapsed.Minutes())
			if minutes < 1 {
				minutes = 1
			}
			cc.Emit(i18n.T("goal.event", shortRunID(runID), i18n.T("goal.progress.review_elapsed", minutes)))
		},
	})
	if baseline != nil {
		if current, snapshotErr := projectFileState(project); snapshotErr == nil {
			_ = core.RecordChangedFiles(context.Background(), runID, changedProjectFiles(baseline, current))
		}
	}
	close(stopEvents)
	<-eventsDone
	if err != nil {
		cc.Emit(i18n.T("goal.blocked", shortRunID(runID), statusAfter(core, runID), permissions.RedactTarget(err.Error())))
	} else {
		if run, getErr := core.GetRun(context.Background(), runID); getErr == nil && run.Status == engine.Done {
			cc.Emit(i18n.T("goal.event", shortRunID(runID), runCompletionLine(core, runID, run)))
		}
	}
	cc.Emit(formatRunSummary(core, runID, baseline == nil))
}

func shortRunID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func emitRunEvents(core *engine.Engine, runID string, after *int64, cc CommandContext) {
	events, err := core.Events(context.Background(), runID, *after, 100)
	if err != nil {
		return
	}
	for _, event := range events {
		*after = event.ID
		if current, ok := cc.(*sessionCommandContext); ok && current.onAgentMetrics != nil && event.Type == "agent.finished" {
			var metrics engine.AgentMetrics
			if json.Unmarshal(event.Payload, &metrics) == nil {
				current.onAgentMetrics(metrics)
			}
		}
		if detail := formatRunEvent(event); detail != "" {
			cc.Emit(i18n.T("goal.event", shortRunID(runID), detail))
		}
	}
}

func formatRunEvent(event engine.Event) string {
	var payload map[string]json.RawMessage
	if json.Unmarshal(event.Payload, &payload) != nil {
		return ""
	}
	stringValue := func(key string) string {
		var value string
		_ = json.Unmarshal(payload[key], &value)
		return value
	}
	switch event.Type {
	case "run.state":
		from, to, reason := stringValue("from"), stringValue("to"), stringValue("reason")
		switch {
		case from == string(engine.Review) && to == string(engine.Gates):
			return i18n.T("goal.progress.review_approved")
		case to == string(engine.Fixing):
			return i18n.T("goal.progress.fixing", permissions.RedactTarget(reason))
		case to == string(engine.Blocked):
			return i18n.T("goal.progress.blocked", permissions.RedactTarget(reason))
		case to == string(engine.Done):
			return ""
		default:
			return i18n.T("goal.progress.state", from, to)
		}
	case "gate.finished":
		var data struct {
			Name   string `json:"name"`
			Passed bool   `json:"passed"`
			Result struct {
				Name     string        `json:"name"`
				Passed   bool          `json:"passed"`
				Duration time.Duration `json:"duration"`
				Reason   string        `json:"reason"`
			} `json:"result"`
		}
		if json.Unmarshal(event.Payload, &data) != nil {
			return ""
		}
		name, passed, duration := data.Result.Name, data.Result.Passed, data.Result.Duration
		if name == "" {
			name, passed = data.Name, data.Passed
		}
		outcome := i18n.T("goal.progress.gate_failed")
		if passed {
			outcome = i18n.T("goal.progress.gate_passed")
		}
		return i18n.T("goal.progress.gate", name, outcome, formatSeconds(duration.Seconds()))
	case "review.finished":
		verdict := stringValue("verdict")
		label := i18n.T("goal.progress.review.error")
		if verdict == "approve" {
			label = i18n.T("goal.progress.review.approved")
		} else if verdict == "changes_requested" {
			label = i18n.T("goal.progress.review.changes")
		} else if verdict == "indeterminate" {
			label = i18n.T("goal.progress.review.indeterminate")
		} else if verdict == "review_expired" {
			label = i18n.T("goal.progress.review.expired")
		}
		line := i18n.T("goal.progress.review", label, shortAgentRole(stringValue("reviewer")))
		var findings []string
		_ = json.Unmarshal(payload["findings"], &findings)
		for _, finding := range findings {
			line += "\n- " + permissions.RedactTarget(finding)
		}
		return line
	case "agent.finished":
		var metrics engine.AgentMetrics
		if json.Unmarshal(event.Payload, &metrics) != nil {
			return ""
		}
		role := i18n.T("goal.progress.developer")
		if metrics.Role == "reviewer" {
			role = i18n.T("goal.progress.reviewer")
		}
		cost := i18n.T("goal.progress.cost_unknown")
		if metrics.CostKnown {
			cost = i18n.T("goal.progress.cost", formatMoney(metrics.CostUSD))
		}
		line := i18n.T("goal.progress.agent", role, metrics.Agent, formatSeconds(float64(metrics.DurationMS)/1000), cost)
		if metrics.InputTokensKnown {
			line += i18n.T("goal.progress.input_tokens", metrics.InputTokens)
		}
		if metrics.OutputTokensKnown {
			line += i18n.T("goal.progress.output_tokens", metrics.OutputTokens)
		}
		if metrics.FailureReason != "" {
			line += ": " + permissions.RedactTarget(metrics.FailureReason)
		}
		return line
	case "agent.event":
		if stringValue("event_type") == "launch.retry" {
			var detail string
			_ = json.Unmarshal(payload["detail"], &detail)
			return i18n.T("goal.progress.retry", stringValue("agent"), permissions.RedactTarget(detail))
		}
	case "dispatch.started":
		var data struct {
			Attempt int `json:"attempt"`
		}
		_ = json.Unmarshal(event.Payload, &data)
		return i18n.T("goal.progress.dispatch_started", fmt.Sprint(data.Attempt))
	case "dispatch.finished":
		if stringValue("status") == string(engine.DispatchSucceeded) {
			return i18n.T("goal.progress.dispatch_done")
		}
		return i18n.T("goal.progress.dispatch_failed", permissions.RedactTarget(stringValue("reason")))
	}
	return ""
}

func shortAgentRole(value string) string {
	if index := strings.Index(value, ":"); index >= 0 {
		return value[:index]
	}
	return value
}

func formatSeconds(value float64) string {
	formatted := fmt.Sprintf("%.1f s", value)
	if i18n.Language() == "pt-BR" {
		formatted = strings.ReplaceAll(formatted, ".", ",")
	}
	return formatted
}

func formatMoney(value float64) string {
	formatted := fmt.Sprintf("%.2f", value)
	if i18n.Language() == "pt-BR" {
		formatted = strings.ReplaceAll(formatted, ".", ",")
	}
	return formatted
}

func formatRunSummary(core *engine.Engine, runID string, resumed bool) string {
	run, err := core.GetRun(context.Background(), runID)
	if err != nil {
		return ""
	}
	events, eventsErr := core.Events(context.Background(), runID, 0, 1000)
	gateResults := map[string]engine.GateResult{}
	files := []string(nil)
	verdict := ""
	var reviewerFindings []string
	cost, costKnown := completeAgentCost(events)
	if eventsErr != nil {
		costKnown = false
	}
	for _, event := range events {
		if event.Type == "gate.cycle.started" {
			gateResults = map[string]engine.GateResult{}
		}
		if event.Type == "gate.finished" {
			var data struct {
				Result engine.GateResult `json:"result"`
			}
			if json.Unmarshal(event.Payload, &data) == nil && data.Result.Name != "" {
				gateResults[data.Result.Name] = data.Result
			}
		}
		if event.Type == "review.finished" {
			var data struct {
				Verdict  string   `json:"verdict"`
				Findings []string `json:"findings"`
			}
			_ = json.Unmarshal(event.Payload, &data)
			verdict = data.Verdict
			reviewerFindings = append(reviewerFindings, data.Findings...)
		}

		if event.Type == "run.files_changed" {
			var data struct {
				Files []string `json:"files"`
			}
			_ = json.Unmarshal(event.Payload, &data)
			files = data.Files
		}
	}
	gateNames := make([]string, 0, len(gateResults))
	for name := range gateResults {
		gateNames = append(gateNames, name)
	}
	sort.Strings(gateNames)
	var gates []string
	for _, name := range gateNames {
		result := i18n.T("goal.progress.gate_failed")
		if gateResults[name].Passed {
			result = i18n.T("goal.progress.gate_passed")
		}
		gates = append(gates, name+": "+result)
	}
	gateText := strings.Join(gates, ", ")
	if gateText == "" {
		gateText = i18n.T("goal.summary.gates_none")
	}
	reviewer := i18n.T("goal.summary.reviewer_unknown")
	if verdict != "" {
		label := "error"
		switch verdict {
		case "approve":
			label = "approved"
		case "changes_requested":
			label = "changes"
		case "indeterminate":
			label = "indeterminate"
		case "review_expired":
			label = "expired"
		}
		reviewer = i18n.T("goal.progress.review." + label)
	}
	fileText := strings.Join(files, ", ")
	if len(files) > 20 {
		fileText = strings.Join(files[:20], ", ") + fmt.Sprintf(" (+%d)", len(files)-20)
	}
	if fileText == "" {
		fileText = i18n.T("goal.summary.files_none")
		if resumed {
			fileText = i18n.T("goal.summary.files_unknown")
		}
	}
	costText := i18n.T("goal.summary.cost_unknown")
	if costKnown {
		costText = "$" + formatMoney(cost)
		if i18n.Language() == "pt-BR" {
			costText = "US$ " + formatMoney(cost)
		}
		costText += i18n.T("goal.summary.reported")
	}
	duration := run.UpdatedAt.Sub(run.CreatedAt).Seconds()
	if duration < 0 {
		duration = 0
	}
	findingText := strings.Join(reviewerFindings, "; ")
	if findingText == "" {
		findingText = i18n.T("goal.summary.reviewer_findings_none")
	}
	return i18n.T("goal.summary", shortRunID(runID), run.Status, formatSeconds(duration), costText, gateText, reviewer, findingText, fileText, shortRunID(runID))
}

func runCompletionLine(core *engine.Engine, runID string, run engine.Run) string {
	duration := run.UpdatedAt.Sub(run.CreatedAt).Seconds()
	if duration < 0 {
		duration = 0
	}
	cost := i18n.T("goal.summary.cost_unknown")
	events, err := core.Events(context.Background(), runID, 0, 1000)
	total, known := completeAgentCost(events)
	known = known && err == nil
	if known {
		cost = "$" + formatMoney(total)
		if i18n.Language() == "pt-BR" {
			cost = "US$ " + formatMoney(total)
		}
		cost += i18n.T("goal.summary.reported")
	}
	return i18n.T("goal.summary.completed", formatSeconds(duration), cost)
}

func projectFileState(project string) (map[string][32]byte, error) {
	files := make(map[string][32]byte)
	err := filepath.WalkDir(project, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == project {
			return nil
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "vendor") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(project, path)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		var digest [32]byte
		copy(digest[:], hash.Sum(nil))
		files[filepath.ToSlash(relative)] = digest
		return nil
	})
	return files, err
}

func changedProjectFiles(before, after map[string][32]byte) []string {
	changes := make([]string, 0)
	for name, digest := range after {
		if prior, ok := before[name]; !ok || prior != digest {
			changes = append(changes, name)
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			changes = append(changes, name)
		}
	}
	sort.Strings(changes)
	return changes
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
			return fmt.Sprintf("Run %s: %s — %s", shortRunID(run.ID), run.Status, run.Goal), nil
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
		runs, listErr := core.ListRuns(ctx, 100)
		if listErr != nil {
			closeStore()
			return "", listErr
		}
		run, err = findProjectRun(runs, project, args[0])
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
		return fmt.Sprintf("Run %s is already DONE.", shortRunID(run.ID)), nil
	}
	cfg, err := config.LoadFrom(cc.ConfigHome())
	if err != nil {
		closeStore()
		return "", err
	}
	results := discoveryResults(cc.DiscoveryResults())
	agent := orchestration.SelectDeveloper(results, cfg.Agents.Developer)
	if agent == "" {
		closeStore()
		return "", unavailableRunAgentError("developer", cfg.Agents.Developer, results)
	}
	reviewer := orchestration.SelectReviewer(results, agent)
	if cfg.Agents.Reviewer != "" {
		if !orchestration.AgentAvailable(results, cfg.Agents.Reviewer) {
			closeStore()
			return "", unavailableRunAgentError("reviewer", cfg.Agents.Reviewer, results)
		}
		reviewer = cfg.Agents.Reviewer
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
		runAsync(managerCtx, cc, core, run.ID, project, run.Goal, agent, reviewer)
	}()
	return fmt.Sprintf("Resuming run %s.", shortRunID(run.ID)), nil
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
		runs, err := core.ListRuns(ctx, 100)
		if err != nil {
			return "", err
		}
		selected, err := findProjectRun(runs, project, args[0])
		if err != nil {
			return "", err
		}
		id = selected.ID
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
	if len(args) == 0 {
		confirmer, ok := cc.(interface {
			Confirm(PermissionRequest) (bool, error)
		})
		if !ok {
			return "", errors.New("cancellation confirmation is unavailable")
		}
		confirmed, confirmErr := confirmer.Confirm(PermissionRequest{Agent: "user", Action: "cancel_run", Target: shortRunID(id), ExactCommand: "Cancel active orchestration run " + shortRunID(id)})
		if confirmErr != nil {
			return "", confirmErr
		}
		if !confirmed {
			return "Cancellation declined.", nil
		}
	}
	if value, ok := activeRuns.Load(id); ok {
		value.(activeRun).cancel()
	}
	if run.Status != engine.Cancelled && run.Status != engine.Done {
		if err = core.CancelRun(ctx, id); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("Cancellation requested for run %s.", shortRunID(id)), nil
}

func runsCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if len(args) > 1 {
		return "", errors.New("usage: /runs [run-id]")
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
	if len(args) == 1 {
		selected, findErr := findProjectRun(runs, project, args[0])
		if findErr != nil {
			return "", findErr
		}
		events, eventErr := core.Events(ctx, selected.ID, 0, 1000)
		if eventErr != nil {
			return "", eventErr
		}
		lines := []string{formatRunSummary(core, selected.ID, true)}
		for _, event := range events {
			if detail := formatRunEvent(event); detail != "" {
				lines = append(lines, i18n.T("goal.event", shortRunID(selected.ID), detail))
			}
		}
		return strings.Join(lines, "\n"), nil
	}
	lines := []string{}
	for _, run := range runs {
		if runBelongsToProject(run, project) {
			lines = append(lines, fmt.Sprintf("%s  %-12s  %s", shortRunID(run.ID), run.Status, run.Goal))
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

func findProjectRun(runs []engine.Run, project, prefix string) (engine.Run, error) {
	var selected *engine.Run
	for index := range runs {
		if runBelongsToProject(runs[index], project) && strings.HasPrefix(runs[index].ID, prefix) {
			if selected != nil {
				return engine.Run{}, errors.New("run id prefix is ambiguous")
			}
			selected = &runs[index]
		}
	}
	if selected == nil {
		return engine.Run{}, errors.New("run not found for this project")
	}
	return *selected, nil
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
				effect := i18n.T("goal.progress.denied")
				if decision.Effect == permissions.Allow {
					effect = i18n.T("goal.progress.allowed")
				}
				scopeKey := "profile_default"
				switch string(decision.Scope) {
				case "run":
					scopeKey = "run"
				case "project":
					scopeKey = "project"
				case "agent":
					scopeKey = "agent"
				case "user":
					scopeKey = "user"
				}
				scope := i18n.T("goal.progress.scope." + scopeKey)
				emitter.Emit(i18n.T("goal.event", shortRunID(request.RunID), i18n.T("goal.progress.approval", i18n.T("permission."+string(request.Action)), effect, scope)))
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

// completeAgentCost omits a total when any recorded agent cost is unknown or
// the event page may have been truncated.
func completeAgentCost(events []engine.Event) (float64, bool) {
	if len(events) >= 1000 {
		return 0, false
	}
	var total float64
	seen := false
	for _, event := range events {
		if event.Type != "agent.finished" {
			continue
		}
		var metrics engine.AgentMetrics
		if json.Unmarshal(event.Payload, &metrics) != nil || !metrics.CostKnown {
			return 0, false
		}
		seen = true
		total += metrics.CostUSD
	}
	return total, seen
}
