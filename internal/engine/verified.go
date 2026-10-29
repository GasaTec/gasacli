package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type ReviewVerdict string

const (
	ReviewApprove          ReviewVerdict = "approve"
	ReviewChangesRequested ReviewVerdict = "changes_requested"
	ReviewIndeterminate    ReviewVerdict = "indeterminate"
	ReviewExpired          ReviewVerdict = "review_expired"
)

const maxReviewFindings = 10
const maxReviewFindingLength = 300

type ReviewResult struct {
	Verdict  ReviewVerdict `json:"verdict"`
	Findings []string      `json:"findings,omitempty"`
}

// ParseReview accepts only a single structured verdict. Prose or unknown
// fields fail closed so an ambiguous reviewer response cannot approve a run.
func ParseReview(value string) (ReviewResult, error) {
	var result ReviewResult
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		lineEnd := strings.IndexByte(value, '\n')
		if lineEnd < 0 {
			return ReviewResult{}, errors.New("engine: reviewer verdict has an invalid code fence")
		}
		language := strings.TrimSpace(strings.TrimPrefix(value[:lineEnd], "```"))
		if language != "" && language != "json" {
			return ReviewResult{}, errors.New("engine: reviewer verdict code fence must contain JSON")
		}
		if !strings.HasSuffix(value, "```") {
			return ReviewResult{}, errors.New("engine: reviewer verdict has an unclosed code fence")
		}
		value = strings.TrimSpace(value[lineEnd+1 : len(value)-3])
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return ReviewResult{}, fmt.Errorf("engine: decode reviewer verdict: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return ReviewResult{}, fmt.Errorf("engine: decode trailing reviewer verdict: %w", err)
		}
		return ReviewResult{}, errors.New("engine: reviewer verdict must be one JSON object")
	}
	if result.Verdict != ReviewApprove && result.Verdict != ReviewChangesRequested {
		return ReviewResult{}, errors.New("engine: reviewer verdict must be approve or changes_requested")
	}
	for _, finding := range result.Findings {
		if strings.TrimSpace(finding) == "" || strings.IndexByte(finding, 0) >= 0 {
			return ReviewResult{}, errors.New("engine: reviewer findings must be non-empty text")
		}
	}
	if len(result.Findings) > maxReviewFindings {
		result.Findings = result.Findings[:maxReviewFindings]
	}
	for index, finding := range result.Findings {
		runes := []rune(finding)
		if len(runes) > maxReviewFindingLength {
			result.Findings[index] = string(runes[:maxReviewFindingLength])
		}
	}
	if result.Verdict == ReviewChangesRequested && len(result.Findings) == 0 {
		return ReviewResult{}, errors.New("engine: changes_requested verdict requires findings")
	}
	return result, nil
}

type DeveloperFactory func(feedback string) Worker
type GateCheck func(context.Context) ([]GateResult, error)
type ReviewCheck func(context.Context) (ReviewResult, error)

type VerifiedOptions struct {
	Scheduler      SchedulerOptions
	MaxFixRetries  int
	Developer      string
	Reviewer       string
	ReviewedChange func() (string, error)
}

// RunVerified coordinates implementation, deterministic gates, and an
// independent reviewer. Fix requests return to the developer with bounded
// retries; the run can reach DONE only after both checks and review approve.
func (e *Engine) RunVerified(ctx context.Context, runID string, developer DeveloperFactory, gates GateCheck, review ReviewCheck, options VerifiedOptions) error {
	if developer == nil || gates == nil || review == nil {
		return errors.New("engine: developer, gates, and reviewer are required")
	}
	if options.MaxFixRetries < 0 || options.MaxFixRetries > 5 {
		return errors.New("engine: fix retries must be between 0 and 5")
	}
	if strings.TrimSpace(options.Developer) == "" || strings.TrimSpace(options.Reviewer) == "" || options.Developer == options.Reviewer || options.ReviewedChange == nil {
		return errors.New("engine: developer and reviewer must be separate sessions")
	}
	feedback := ""
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			_ = e.CancelRun(context.Background(), runID)
			return ctx.Err()
		}
		run, err := e.GetRun(ctx, runID)
		if err != nil {
			return err
		}
		if run.Status != Gates && run.Status != Review {
			worker := developer(feedback)
			if err := e.RunDAG(ctx, runID, worker, options.Scheduler); err != nil {
				return err
			}
			run, err = e.GetRun(ctx, runID)
			if err != nil {
				return err
			}
		}
		cycleID := ""
		if run.Status != Review {
			cycleID = newID()
			if err := e.addEvent(context.Background(), runID, "", "", "gate.cycle.started", map[string]string{"cycle": cycleID}); err != nil {
				return err
			}
			gateResults, gateErr := gates(ctx)
			if persistErr := e.persistGateResults(runID, cycleID, gateResults, gateErr); persistErr != nil {
				return persistErr
			}
			if ctx.Err() != nil {
				_ = e.CancelRun(context.Background(), runID)
				return ctx.Err()
			}
			if gateErr == nil {
				for _, result := range gateResults {
					if !result.Passed {
						gateErr = fmt.Errorf("gate %s failed", result.Name)
						break
					}
				}
			}
			if gateErr != nil {
				if attempt >= options.MaxFixRetries {
					return e.blockAfterReviewFailure(runID, "quality gates failed after bounded fixes: "+gateErr.Error())
				}
				feedback = "Deterministic quality gates failed. Fix these failures and rerun them: " + gateErr.Error()
				if err := e.prepareFix(ctx, runID); err != nil {
					return err
				}
				continue
			}
			if err := e.Transition(ctx, runID, Review, "deterministic quality gates passed"); err != nil {
				return err
			}
		} else {
			cycleID, err = e.latestGateCycle(ctx, runID)
			if err != nil || cycleID == "" {
				return e.blockAfterReviewFailure(runID, "review state has no passing-gate cycle")
			}
		}
		result, reviewErr := review(ctx)
		if ctx.Err() != nil {
			_ = e.CancelRun(context.Background(), runID)
			return ctx.Err()
		}
		if reviewErr != nil {
			_ = e.addEvent(context.Background(), runID, "", "", "review.finished", map[string]any{"cycle": cycleID, "reviewer": options.Reviewer, "verdict": "error"})
			return e.blockAfterReviewFailure(runID, "independent review failed: "+reviewErr.Error())
		}
		if result.Verdict != ReviewApprove && result.Verdict != ReviewChangesRequested && result.Verdict != ReviewIndeterminate && result.Verdict != ReviewExpired {
			_ = e.addEvent(context.Background(), runID, "", "", "review.finished", map[string]any{"cycle": cycleID, "reviewer": options.Reviewer, "verdict": "invalid"})
			return e.blockAfterReviewFailure(runID, "independent reviewer returned an invalid verdict")
		}
		findings := boundReviewFindings(result.Findings)
		if err := e.addEvent(context.Background(), runID, "", "", "review.finished", map[string]any{"cycle": cycleID, "reviewer": options.Reviewer, "verdict": result.Verdict, "findings": findings, "finding_count": len(findings)}); err != nil {
			return err
		}
		if result.Verdict == ReviewApprove {
			if err := e.Transition(ctx, runID, Gates, "independent review approved"); err != nil {
				if ctx.Err() != nil {
					_ = e.CancelRun(context.Background(), runID)
				}
				return err
			}
			change, err := options.ReviewedChange()
			if err != nil {
				return e.blockAfterReviewFailure(runID, "could not identify the reviewed change")
			}
			err = e.CompleteRun(ctx, runID, CompletionEvidence{ChecksPassed: true, ReviewApproved: true, Developer: options.Developer, Reviewer: options.Reviewer, Commit: change})
			if ctx.Err() != nil {
				_ = e.CancelRun(context.Background(), runID)
				return ctx.Err()
			}
			return err
		}
		reason := "independent review requested changes after bounded fixes"
		if result.Verdict == ReviewIndeterminate {
			reason = "independent review was indeterminate after bounded fixes"
		} else if result.Verdict == ReviewExpired {
			reason = "independent review expired after bounded fixes"
		}
		if attempt >= options.MaxFixRetries {
			return e.blockAfterReviewFailure(runID, reason)
		}
		feedback = "Independent review " + string(result.Verdict) + ": " + strings.Join(findings, "; ")
		if err := e.prepareFix(ctx, runID); err != nil {
			return err
		}
	}
}

func boundReviewFindings(findings []string) []string {
	if len(findings) > maxReviewFindings {
		findings = findings[:maxReviewFindings]
	}
	bounded := make([]string, 0, len(findings))
	for _, finding := range findings {
		finding = strings.TrimSpace(finding)
		runes := []rune(finding)
		if len(runes) > maxReviewFindingLength {
			finding = string(runes[:maxReviewFindingLength])
		}
		if finding != "" {
			bounded = append(bounded, finding)
		}
	}
	return bounded
}

func (e *Engine) persistGateResults(runID, cycleID string, results []GateResult, runErr error) error {
	if len(results) == 0 && runErr == nil {
		return errors.New("engine: gate runner returned no results")
	}
	if len(results) == 0 {
		return e.addEvent(context.Background(), runID, "", "", "gate.finished", map[string]any{"cycle": cycleID, "name": "quality-gates", "passed": false, "reason": "no gate result"})
	}
	for _, result := range results {
		if err := e.addEvent(context.Background(), runID, "", "", "gate.finished", map[string]any{"cycle": cycleID, "result": result}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) latestGateCycle(ctx context.Context, runID string) (string, error) {
	events, err := e.allEvents(ctx, runID)
	if err != nil {
		return "", err
	}
	cycle := ""
	for _, event := range events {
		if event.Type != "gate.cycle.started" {
			continue
		}
		var payload struct {
			Cycle string `json:"cycle"`
		}
		if json.Unmarshal(event.Payload, &payload) == nil {
			cycle = payload.Cycle
		}
	}
	return cycle, nil
}

func (e *Engine) requireVerificationEvidence(ctx context.Context, runID, reviewer string) error {
	events, err := e.allEvents(ctx, runID)
	if err != nil {
		return fmt.Errorf("engine: load verification evidence: %w", err)
	}
	cycle := ""
	for _, event := range events {
		if event.Type != "gate.cycle.started" {
			continue
		}
		var payload struct {
			Cycle string `json:"cycle"`
		}
		if json.Unmarshal(event.Payload, &payload) == nil {
			cycle = payload.Cycle
		}
	}
	gateCount, lastGate := 0, -1
	for index, event := range events {
		if event.Type != "gate.finished" {
			continue
		}
		var payload struct {
			Cycle  string     `json:"cycle"`
			Result GateResult `json:"result"`
			Name   string     `json:"name"`
			Passed bool       `json:"passed"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return errors.New("engine: DONE requires passing gate evidence for the current run cycle")
		}
		if payload.Cycle != cycle {
			continue
		}
		name, passed := payload.Result.Name, payload.Result.Passed
		if name == "" {
			name, passed = payload.Name, payload.Passed
		}
		if name == "" || !passed {
			return errors.New("engine: DONE requires passing gate evidence for the current run cycle")
		}
		gateCount++
		lastGate = index
	}
	if gateCount == 0 {
		return errors.New("engine: DONE requires at least one passing quality gate")
	}
	for index, event := range events {
		if index <= lastGate || event.Type != "review.finished" {
			continue
		}
		var result struct {
			Cycle    string        `json:"cycle"`
			Reviewer string        `json:"reviewer"`
			Verdict  ReviewVerdict `json:"verdict"`
		}
		if json.Unmarshal(event.Payload, &result) == nil && result.Cycle == cycle && result.Reviewer == reviewer && result.Verdict == ReviewApprove {
			return nil
		}
	}
	return errors.New("engine: DONE requires an approved independent review after passing gates")
}

func (e *Engine) allEvents(ctx context.Context, runID string) ([]Event, error) {
	const pageSize, maxPages = 1000, 100
	var events []Event
	var after int64
	for page := 0; page < maxPages; page++ {
		batch, err := e.Events(ctx, runID, after, pageSize)
		if err != nil {
			return nil, err
		}
		events = append(events, batch...)
		if len(batch) < pageSize {
			return events, nil
		}
		after = batch[len(batch)-1].ID
	}
	return nil, errors.New("engine: run event history exceeds verification limit")
}

func (e *Engine) prepareFix(ctx context.Context, runID string) error {
	if err := e.Transition(ctx, runID, Fixing, "fix requested by a gate or reviewer"); err != nil {
		return err
	}
	if _, err := e.store.DB().ExecContext(ctx, `UPDATE tasks SET status=?,retries=0,updated_at=? WHERE run_id=?`, Queued, nowUTC(), runID); err != nil {
		return err
	}
	return nil
}

func (e *Engine) blockAfterReviewFailure(runID, reason string) error {
	if err := e.Transition(context.Background(), runID, Blocked, reason); err != nil {
		return err
	}
	return fmt.Errorf("engine: run %s blocked: %s", runID, reason)
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }
