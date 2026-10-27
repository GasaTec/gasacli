# ADR-0015 — Global agent configuration and project-scoped session projections

Status: Supersedes project-local agent/prompt duplication proposed in ADR-0014 (2026-10-27).

## Decision
Keep GasaCLI executable, agent definitions, prompts, skills, plugins, policy and central SQLite in OS-appropriate global user-data directory. Project-local .gasa is optional and contains only human-readable session/context projections for collaboration with other tools, not duplicated global configuration.

## Storage
Global user data:
- config.yaml, agents/, prompts/, skills/, plugins/, data/gasacli.db, segundo-cerebro/.
Project (optional):
- .gasa/context.md: stable project context, links to decisions and active goal.
- .gasa/session.md: current run summary, agent roles, task state and next actions.
- .gasa/.gitignore: exclude sensitive/generated session projections unless explicitly approved for version control.

No database or provider credentials in project. Default mode may leave project entirely untouched.

## Source of truth
SQLite central is authoritative for run/task/dispatch state, agent messages, approvals, events, checkpoints and metrics. Markdown projections are generated, bounded and replaceable snapshots; they are not used for transactional coordination or locking.

## Communication protocol
Message envelope:
- message_id, run_id, task_id, dispatch_id, sender, recipient, type, payload_version, created_at, delivery_status, idempotency_key.
- Types: task.assign, task.progress, task.question, task.answer, review.request, review.findings, fix.request, task.result, task.blocked, task.ack.
- Persist before dispatch, acknowledge delivery, deduplicate by message/idempotency key, retry safely and correlate replies.
- Orchestrator owns task transitions and DONE verification; agents cannot self-approve.

## Worker adapters
GasaCLI bridges between message bus and each CLI's documented stdin/args/session/stream interface. Where a CLI cannot receive live messages, deliver at a safe next-turn boundary or restart/resume with a concise state snapshot; do not claim real-time bidirectional chat without adapter support. Summarize and retrieve only relevant context to reduce tokens.

## Example
User /goal -> orchestrator creates plan -> bus assigns task to Codex -> Codex returns artifact/diff -> quality gate runs checks -> bus sends review request to Claude -> Claude sends findings -> bus sends fix request to Codex -> repeat bounded cycle -> orchestrator marks DONE only after checks and independent review.

## Project isolation and security
All messages scoped by run/project ID. Enforce permissions on message delivery and tool actions. Redact secrets in projections/logs; do not expose hidden chain-of-thought. Reconcile stale projections on resume and prevent cross-project context leakage.

## Acceptance
Two heterogeneous agents can exchange structured task/review/fix messages through the GasaCLI core with persistent acknowledgements and resume. Project .gasa may be deleted without losing run history or breaking core orchestration.
