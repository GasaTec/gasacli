# ADR-0018 — Configurable dashboard launch mode

Status: Proposed default, awaiting user's onboarding choice (2026-10-27).

## Onboarding choices
- auto (recommended): launching `gasa` ensures local web server is available and opens the OS default browser. Server runs while clients or background jobs require it.
- manual: TUI does not open browser automatically. `/web open` or `gasa web open` starts/reuses local server and opens browser.
- always: user explicitly enables persistent local service in background, even when TUI is closed. Must have stop/disable control and OS-appropriate service lifecycle.

## Commands
Inside TUI: `/web`, `/web open`, `/web status`, `/web stop`, `/web restart`.
Shell: `gasa web open|status|stop|restart`.
Stopping web must not silently terminate active orchestration; separate UI listener lifecycle from engine jobs and require confirmation if action would affect running goals.

## Config (draft)
```yaml
web:
  mode: auto # auto | manual | always
  host: 127.0.0.1
  port: 3199
  fallback_port: true
```

## Safety
Loopback-only, authenticated API, Origin/Host and CSRF controls, safe port fallback and single-instance handshake. Never force browser to stay open. Avoid permanent background process unless user chooses always or an active background goal needs it.

## Tests
Onboarding preference persistence, browser-open behavior, headless mode, no-browser override, port occupied, existing engine reuse, stop with active runs, restart without duplicate workers.
