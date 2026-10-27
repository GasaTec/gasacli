# ADR-0004: Interactive terminal UX, /goal, editable agents and GitHub plugin

Status: Proposed product specification (2026-10-27)

## Primary UX
Running `gasa` inside a trusted project opens an interactive terminal chat (TUI) similar in interaction pattern to existing coding CLIs. Natural-language chat is distinct from supervised execution. `/goal <request>` starts a durable orchestrated run. Completion requires acceptance criteria, appropriate tests/build and configured independent review; if blocked, report BLOCKED with evidence rather than falsely declaring DONE.

## In-chat commands
`/goal`, `/agents`, `/config`, `/skills`, `/prompts`, `/brain`, `/status`, `/resume`, `/github`, `/help`.

## Non-interactive commands
`gasa`, `gasa doctor`, `gasa config`, `gasa agents`, `gasa github`, `gasa goal "<task>"`. Names are design proposals, not implemented yet.

## Editable defaults
Ship built-in, versioned prompts and role contracts: orchestrator, tech lead, developer, tester, researcher; optional frontend/backend specialists. Copy defaults into user config on explicit customization, keep upstream templates intact, never overwrite local edits on upgrades. Resolve project overrides above user-level settings. Validate schema and show effective configuration.

## Plugin architecture
GitHub first via optional `gh` CLI adapter; detect `gh` and check authenticated access without reading tokens. User opts in to connect repository. Plugin can list/read issues, create issues from plans with user approval, link runs to issues, update status/comments and report results. No GitHub account required for core orchestration. Linear later via separate optional plugin. Skills and plugins have distinct permission boundaries.

## State
Store durable runs/tasks, chat sessions and issue references in SQLite. Keep Markdown second brain optional, separate from CLI installation directory. Preserve context efficiently with summarized state and cited retrieval.

## GitHub workflow
`/goal Implement login` -> plan/acceptance -> optionally create linked GitHub issues -> delegate to installed agents -> tests/build/review -> summarize result and optionally update issue(s) with evidence. Never auto-close issues unless policy explicitly permits and gates pass.

## Safety
Trust prompt before project access. Review diffs before destructive changes. Third-party plugins/skills require explicit installation approval and scoped permissions. Limit retries and token budgets; maintain user-visible progress.

## MVP priority
1. Interactive chat TUI and `/goal`.
2. Agent discovery, fixed routing, native execution and verification.
3. Editable default prompts and agent roles.
4. Optional GitHub `gh` plugin and issue tracking.
5. JEV, second brain, Orca-hosted agent, Linear later.
