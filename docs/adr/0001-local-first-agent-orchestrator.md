# ADR-0001: GasaCLI is a standalone, local-first coding-agent orchestrator

Status: Accepted for MVP design (2026-10-27)

## Product
GasaCLI (`gasa`) orchestrates coding CLIs already installed and authenticated on the user's computer. No mandatory API key, paid provider, Obsidian vault, JEV, or Orca runtime. Users issue plain-language development tasks in a trusted project directory; GasaCLI plans, routes, executes, tests and reports outcomes with minimal redundant context and cost.

## Non-goals
- Reimplementing Claude Code, Codex or other coding agents.
- Depending on Orca to orchestrate.
- Circumventing subscription, provider usage limits or authentication restrictions.
- Guaranteeing savings without telemetry and benchmarks.

## Architecture
1. **Trust gate:** ask approval for project path before inspecting or modifying project content; support revocation and granular permissions.
2. **Agent discovery:** probe PATH and known install locations for supported CLI executables; collect versions, documented noninteractive capabilities and verified readiness. Never read or print secrets. An installed binary does not imply a valid subscription.
3. **Native orchestration:** durable run, task DAG, dispatch attempt, worker, message and approval gate contracts. SQLite persistence, retry limits, checkpoints and structured event logs.
4. **Decision engine:** rules-first intent/risk classification and task sizing; optional JEV/LLM-based planner for ambiguous or complex work. Fallback to an eligible installed agent; no mandatory dedicated orchestration model.
5. **Model/agent router:** choose among installed, authorized agents and supported models according to task capabilities, provider limits, user preferences, reliability, estimated tokens and cost. Use provider-supported mechanisms only; never assume subscription token costs are directly observable.
6. **Prompt/skill engine:** short, versioned role prompts (planner, implementer, tester, reviewer, researcher), progressive disclosure, optional reviewed skills. Explicit consent before third-party plugin installation; pin source/version and audit execution permissions.
7. **Memory:** current repo context by default; optional Obsidian/Markdown vault with local incremental SQLite FTS5 indexing and cited retrieval. Write proposals require approval.
8. **Execution adapters:** native subprocess CLI adapters for Claude Code, Codex, Gemini CLI, Cursor CLI, OpenCode and other compatible CLIs. Optional Orca adapter; no hard dependency. Worktrees for concurrent code changes.
9. **Quality gate:** run appropriate tests/lint/build, verify completion, independent review for high-risk changes, present diff and ask approval for destructive actions.

## Orca research
Orca's documented orchestration model includes Run (durable namespace), Task (dependencies/status), Dispatch (attempt), supervised worker, message inbox and decision gates. The Orca CLI speaks to a running Orca runtime, and orchestration is experimental. Use the conceptual contracts as inspiration, not Orca's implementation as a dependency. Optional Orca integration should use documented JSON CLI commands and feature detection. Reference: https://www.onorca.dev/docs/cli/orchestration

## First-run experience
```
cd my-project
gasa
# project trust prompt
# local agent discovery and readiness
# optional skill recommendations
> Implement authentication
```

## Cost optimization policy
Prefer single-agent execution for small tasks; avoid duplicative planning, full-vault ingestion and gratuitous parallel workers. Retrieve minimal relevant context; cap retries; reuse deterministic summaries; select only relevant skills. Track measured tokens where CLIs expose usage, otherwise report estimates or unavailable, never fabricated savings. Compare task success rate, time, usage and cost against a baseline.

## Acceptance tests
- Runs with one supported local CLI and no API key entered into GasaCLI.
- Reports missing/unauthenticated CLIs accurately.
- Trust prompt precedes project access.
- Small task executes end-to-end and passes tests.
- Interrupted run resumes without duplicating completed work.
- Orca and Obsidian remain optional.
- Third-party skills require explicit installation approval.
