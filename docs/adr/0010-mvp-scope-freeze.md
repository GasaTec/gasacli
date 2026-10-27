# ADR-0010 — MVP scope freeze and implementation sequence

Status: Accepted for initial build, 2026-10-27.

## Product baseline
GasaCLI is an MIT-licensed, local-first, Go-based coding-agent orchestrator. Users launch `gasa` for terminal chat, run `/goal` for supervised execution, and optionally view an embedded local dashboard at 127.0.0.1:3199 (safe port fallback). SQLite persists runs. Agent adapters use installed and authenticated coding CLIs. The orchestrator must validate acceptance, tests and independent review where configured. Resource/usage metrics are labeled measured, reported, estimated or unavailable.

## MVP scope (build now)
1. Go executable, project trust and terminal chat.
2. Embedded localhost dashboard, single engine, secure local session and port fallback.
3. Discovery/readiness for Claude Code and Codex.
4. Fixed YAML routing and editable orchestrator/developer/reviewer prompts.
5. Native durable /goal with bounded retries, checkpoints, cancellation, SQLite and task events.
6. Real execution with one agent, then cross-provider reviewer and test/build gates.
7. Basic live observability: roles, task states, agent events, RAM/CPU, usage and API-equivalent estimates when available.
8. Tests, cross-platform builds, performance benchmarks and security checks.

## Deferred, not required for MVP
JEV dynamic routing, Obsidian vault integration, third-party skills, GitHub issue plugin, Orca inbound/outbound integration, Rust optimization, multi-device Cloud sync, billing, team management and Linear.

## Order
A. Build a real Go binary and minimal TUI.
B. Add trust, discovery and adapters.
C. Deliver one /goal end-to-end with SQLite and deterministic checks.
D. Add independent reviewer and durable recovery.
E. Embed secure local dashboard and telemetry.
F. Harden, benchmark and publish an alpha.

## Quality bar
No false DONE, no secrets in logs, no mandatory Gasatec account, no provider limit circumvention, no hidden reasoning exposure, no fabricated token/cost metrics. Keep parent Go process lean and children separately accounted.

## Governance
New features after this ADR are backlog only until the alpha is usable. Update this ADR if priorities intentionally change. Prior ADRs remain historical; this document defines MVP scope.
