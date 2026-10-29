package engine

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"

	"github.com/GasaTec/gasacli/internal/permissions"
)

// AgentMetrics is a redacted summary; prompts, response text, and event data
// are never persisted here.
type AgentMetrics struct {
	Role          string  `json:"role"`
	Agent         string  `json:"agent"`
	ExitCode      int     `json:"exit_code"`
	EventCount    int     `json:"event_count"`
	OutputBytes   int     `json:"output_bytes"`
	DurationMS    int64   `json:"duration_ms"`
	CostUSD       float64 `json:"cost_usd,omitempty"`
	CostKnown     bool    `json:"cost_known"`
	InputTokens   int64   `json:"input_tokens,omitempty"`
	OutputTokens  int64   `json:"output_tokens,omitempty"`
	FailureReason string  `json:"failure_reason,omitempty"`
}

func (e *Engine) RecordAgentEvent(ctx context.Context, runID, role, agent, eventType string) error {
	return e.RecordAgentEventDetail(ctx, runID, role, agent, eventType, "")
}

func (e *Engine) RecordAgentEventDetail(ctx context.Context, runID, role, agent, eventType, detail string) error {
	if !validAgentRole(role) || !validAgent(agent) || !validEventType(eventType) {
		return errors.New("engine: invalid agent event metadata")
	}
	payload := map[string]string{"role": role, "agent": agent, "event_type": eventType}
	if detail != "" {
		payload["detail"] = permissions.RedactTarget(detail)
	}
	return e.addEvent(ctx, runID, "", "", "agent.event", payload)
}

func (e *Engine) RecordAgentMetrics(ctx context.Context, runID string, metrics AgentMetrics) error {
	if !validAgentRole(metrics.Role) || !validAgent(metrics.Agent) || metrics.DurationMS < 0 || metrics.OutputBytes < 0 || metrics.EventCount < 0 || metrics.InputTokens < 0 || metrics.OutputTokens < 0 || metrics.CostUSD < 0 || math.IsNaN(metrics.CostUSD) || math.IsInf(metrics.CostUSD, 0) {
		return errors.New("engine: invalid agent metrics")
	}
	metrics.FailureReason = permissions.RedactTarget(metrics.FailureReason)
	return e.addEvent(ctx, runID, "", "", "agent.finished", metrics)
}

// RecordChangedFiles stores only bounded relative paths for a run summary.
func (e *Engine) RecordChangedFiles(ctx context.Context, runID string, files []string) error {
	if len(files) > 100 {
		files = files[:100]
	}
	clean := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, file)
		file = filepath.ToSlash(filepath.Clean(strings.TrimSpace(file)))
		if file == "." || filepath.IsAbs(file) || strings.HasPrefix(file, "../") || strings.ContainsAny(file, "\x00\r\n") {
			return errors.New("engine: invalid changed file path")
		}
		clean = append(clean, permissions.RedactTarget(file))
	}
	return e.addEvent(ctx, runID, "", "", "run.files_changed", map[string]any{"files": clean})
}

func validAgentRole(role string) bool { return role == "developer" || role == "reviewer" }

func validAgent(agent string) bool {
	return agent != "" && len(agent) <= 64 && !strings.ContainsAny(agent, "\x00\r\n")
}

func validEventType(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
