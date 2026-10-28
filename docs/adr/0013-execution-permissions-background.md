# ADR-0013 — Parallel/sequential scheduling, shared engine, permissions and background execution

Status: Accepted product decisions (2026-10-27).

## Confirmed decisions
1. Support parallel and sequential task execution based on dependencies and resource policy.
2. Use one shared Go engine per machine, with separate trusted projects, sessions, worktrees, permissions, runs and logs. One localhost dashboard, default 127.0.0.1:3199 with safe fallback.
3. Expose `/permissions` with `/permissoes` alias, configurable per user, project and agent; allow model autonomy within hard limits.
4. Support optional background continuation after terminal exit, with persistent state and explicit enablement.
5. Future Cloud may provide opt-in remote execution as a separate capability alongside sync, never required for local core.

## Scheduler
Task DAG determines readiness; tasks with dependencies run sequentially, independent tasks may run in parallel. Global and per-project worker limits, bounded queues, fair scheduling, timeout, cancellation, retries and memory pressure controls. Serialize unsafe merges and shared file edits; worktree isolation where appropriate.

## Shared engine
Single-instance coordination via OS-appropriate lock and authenticated IPC/loopback handshake. Multiple TUI/web clients attach to same engine. Project IDs, scoped working directories and run IDs prevent cross-project context mixing. Never kill unrelated processes to claim port 3199.

## Permissions
Profiles: restricted, balanced, autonomous. The canonical vocabulary, per-profile matrix, scope precedence, approval-event schema and `/permissions`/`/permissoes` UX are defined in [ADR-0019](0019-permission-policy-and-approval-events.md). Agent cannot elevate its own permissions. Trusting a folder does not authorize all commands. Sensitive/destructive operations remain subject to hard constraints and scoped approval regardless of autonomy profile.

## Background
User opt-in for run continuation without an open TUI. Engine lifecycle independent of UI clients while runs are active; shutdown and resume safe. OS-specific process management to avoid orphaned workers, logs persisted locally. `gasa --no-browser` and headless use remain supported.

## Future remote execution
Explicit opt-in, device identity, scoped permissions, transport encryption, project data boundaries, billing transparency, job cancellation and audit trails. No remote execution in initial MVP.

## Acceptance criteria
- Two independent tasks can run concurrently, dependent tasks wait.
- Five project sessions share one engine and do not leak context.
- /permissions and /permissoes show effective rules and persist user-confirmed changes.
- Autonomous agent cannot bypass hard restrictions.
- Background run survives terminal exit and can be reattached.
- Cloud absence never prevents local execution.

## Delivery priority
Implement minimal scheduler, shared engine and permission enforcement early. Multi-project sidebar, advanced background management and remote Cloud execution may be delivered incrementally.
