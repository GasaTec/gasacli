# ADR-0011 — Auto-open dashboard, /help and multi-project workspace

Status: Accepted UX direction (2026-10-27). Multi-project sidebar is targeted for a subsequent release, not a prerequisite for first alpha.

## Default launch
`gasa` launches interactive terminal chat and ensures the embedded localhost dashboard is running, then opens its URL in the OS default browser (Chrome if configured as default). Preferred bind: 127.0.0.1:3199; authenticate/reuse an existing GasaCLI instance, otherwise safely select a free port. Do not kill other services. Browser auto-open can be disabled with `gasa --no-browser` and user config; headless/CI environments should not attempt to open a browser.

## Help
`/help` works inside the chat, grouped by workflow: goals, agents, projects, configuration, plugins, memory, monitoring and session control. `gasa --help` works before entering the TUI. Include descriptions, examples, aliases and discoverable shortcuts. Unknown slash commands offer nearest valid alternatives.

## Planned command map
- `/help` — commands and examples.
- `/goal <request>` — supervised execution.
- `/projects` — list/switch active projects.
- `/agents` — agents and roles.
- `/status` — task progress and gates.
- `/monitor` — open/focus local dashboard.
- `/config`, `/prompts`, `/skills`, `/plugins`, `/brain`, `/github`.
- `/resume`, `/cancel`, `/exit`.

## Multi-project workspace (post-alpha)
One local Go engine and one dashboard for all open projects, with each terminal attached to a project/session. Sidebar lists project name/path, running/waiting/reviewing/done status, active agent count, pending approvals and latest goal. Selecting a project shows chat, tasks, agents, metrics, messages and history.

## Concurrency and isolation
Project-scoped trust and permissions; separate runs, working directories, worktrees and logs; global worker/CPU/RAM concurrency limits; do not expose one project's private context to another. SQLite state keyed by project and session IDs. Reconnect after terminal/browser restarts. Prevent duplicate runs and cross-project command execution.

## Lightweight design
Single loopback HTTP server, lazy-loaded project panels, event-driven SSE, bounded logs and resource sampling, no polling all projects continuously. Opening dashboard must not spawn duplicate agents.

## Security
Local session auth, Origin/Host checks, CSRF defenses, explicit project trust, browser launch only to verified loopback endpoint. Keep actual endpoint out of insecure public discovery. No remote binding by default.

## Acceptance
First alpha: `gasa` opens TUI and browser, `/help` lists commands, port fallback and --no-browser work. Later release: five concurrent trusted projects appear in one sidebar with correct independent state and resource accounting.
