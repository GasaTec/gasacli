# GasaCLI roadmap — local-first orchestration

## M1: usable single-agent CLI
- TypeScript CLI and `gasa` command.
- Project trust gate and permissions.
- Agent discovery, version and readiness probes.
- One real supported CLI adapter, task execution, logs and tests.
- Rules-first routing and compact built-in prompts.

## M2: native orchestration
- Persistent Run/Task/Dispatch/Worker/Message/Gate contracts.
- Task dependency graph, supervised completion and retry/resume.
- Git worktrees, safe concurrency and quality gates.
- Second CLI adapter and provider-specific capability detection.

## M3: cost and skills
- Model/agent capability registry, preferences and budget policies.
- Actual usage reporting when available; estimates labeled.
- Versioned planner/implementer/reviewer/tester prompts.
- Optional, user-approved Ponytail 5, Caveman and other audited skills.
- Benchmarks for correctness, elapsed time and consumption.

## M4: optional intelligence and integrations
- JEV as optional advanced decision engine.
- Obsidian/Markdown second-brain adapter with local SQLite FTS5.
- Orca adapter using supported JSON commands against a running runtime.
- Additional CLI agents (Gemini, Cursor, OpenCode, etc.).

## MVP definition of done
A new user runs `gasa` in a project, approves trust, sees detected agents, selects an available one (or accepts automatic routing), requests a code change, receives a tested result and sees transparent usage information, without supplying GasaCLI with a new API key.
