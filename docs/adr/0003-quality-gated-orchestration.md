# ADR-0003: Orchestration lifecycle, independent review and optional JEV/brain

Status: Proposed product specification (2026-10-27)

## Core
GasaCLI is primarily a standalone local coding-agent orchestrator. `/go` starts a durable run. A task is DONE only after acceptance criteria, applicable automated tests/build, and independent review pass. Agent self-report is not proof. If blocked (missing tool, failing test, exhausted retry/budget, unavailable independent reviewer), report BLOCKED with evidence and request intervention; do not loop indefinitely.

## Roles
- Orchestrator: decompose, assign, track dependencies, request fixes, determine completion from verified gates.
- Developer: implement in isolated workspace; return diff, test results and concise summary.
- TL/Reviewer: review actual diff and relevant source, flag correctness/security/maintainability; never same underlying provider/model family as author when independent review required.
- Quality gate: deterministic verification first; review follows changed code. Never hold reviewer active while developer is writing.

## Routing
- `fixed`: use `config.yaml` agent assignments, allowed fallback, and limits. No JEV dependency.
- `jev`: optional decision service selects eligible agents/models by task complexity, capabilities, estimated consumption and risk; still subject to hard policy and review independence.
- If only one compatible agent is installed, mark independent review unavailable; do not silently claim DONE if required.
- Subscriptions and model identifiers are adapter-dependent. Respect terms and documented interfaces.

## Memory
Default operation requires no second brain. Optional Markdown/Obsidian vault at user-selected directory, e.g. `~/.gasacli/segundo-cerebro` or existing external path. CLI may initialize an empty local vault and allow user to copy/clone content after confirmation. Index locally and incrementally, never overwrite original notes. A CLI installation directory should not be used for durable user data.

## Proposed configuration
```yaml
version: 1
routing:
  strategy: fixed # fixed | jev
agents:
  orchestrator: { cli: claude, model: opus }
  developer: { cli: codex, reasoning: high }
  reviewer: { cli: claude, model: sonnet }
quality:
  require_tests: true
  require_build: true
  require_independent_review: true
orchestration:
  max_retries: 3
  on_blocked: ask_user
brain:
  enabled: false
  path: null
```
Example only; exact CLI/model identifiers subject to adapter discovery.

## Lifecycle
TRUST_PENDING -> PLANNING -> EXECUTING -> VERIFYING -> REVIEWING -> (FIXING -> VERIFYING -> REVIEWING)* -> DONE | BLOCKED | CANCELLED.

Each step emits structured events and persists state. Reviews can only pass a specific diff/commit. New code changes invalidate previous review approval. Tests/build commands are project-specific, detected and confirmed where appropriate.

## Integration
Orca can host GasaCLI as a conversational agent only if compatible registration is available (ADR-0002). The native orchestration engine must work independently.

## Acceptance
Demonstrate login screen task, runnable build, tests and cross-provider review with clear evidence. Never claim completion if checks are unavailable, failed or unverified.
