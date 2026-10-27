# ADR-0008 — Observable interactive orchestration TUI

Status: Accepted product requirement; UI mock data is illustrative.

## Goal
Expose what is running, which agent owns each role, what it is doing, agent-to-agent communication, RAM/CPU, token usage, provider limits where observable, elapsed time and cost estimates. Keep parent GasaCLI lightweight.

## Views
- Chat: direct conversation with orchestrator, tool approvals, user prompts.
- Agents: roster with role, provider, model, PID, task, status, start time, last activity and worktree.
- Monitor: run DAG, task dependencies, progress from completed criteria (never invented percentages), memory/CPU parent vs children, token breakdown, cost and elapsed time.
- Messages: structured inter-agent exchanges with source, destination, timestamp, summary and optional expanded sanitized payload. Do not display hidden model reasoning or secrets.
- Logs: bounded stream of subprocess events, tests, diffs and errors; filter by agent/task/severity.

## Metrics and provenance
Each metric has a source and quality flag: measured, provider-reported, estimated or unavailable.
- RAM: OS process RSS, optionally process-tree aggregation; avoid double counting and account for platform differences.
- CPU: per-process OS samples with bounded sampling interval, disabled by default in low-resource mode.
- Tokens: input/output/cache when CLI emits reliable usage; never infer subscription quota from token usage. If missing show 'not reported'.
- Cost: API-equivalent estimate only with known model/pricing and usage, clearly labeled; subscription billing cannot be inferred from tokens. Separate user subscription, actual API billing, and estimates.
- Progress: verified task/check count and statuses, not LLM self-reported completion percentages.
- Agent status: queued, running, waiting, reviewing, blocked, done, failed, cancelled.

## Agent communication
Structured events: task assigned, status changed, message sent, review requested, review completed, fix requested, check failed/passed, decision required. Include correlation IDs and timestamp. Support human-readable summaries and drill-down to raw sanitized records.

## Interaction
Tabs/chat switch, hotkeys, compact mode, details toggle, filters, follow-tail, pause rendering, cancel/approve/resume. TUI event loop must not block execution. Telemetry bounded and lazy; retain aggregate metrics in SQLite, rotate logs.

## Privacy and trust
No raw credentials, sensitive environment variables or private vault excerpts in default telemetry. Redact known secrets. Local metrics by default; Cloud upload only after explicit opt-in. Never claim to expose a provider's hidden reasoning.

## Acceptance
A real two-agent run shows accurate role assignment, lifecycle events, communication, test results and metrics with explicit provenance. When tokens/cost cannot be measured, show unavailable instead of fabricated values. Idle GasaCLI resource budgets remain governed by ADR-0007.
