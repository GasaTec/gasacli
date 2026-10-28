# Real dashboard browser evidence

All PNGs below came from the local Orca browser session against the temporary `GASACLI_HOME` and the real `gasa` binary. Files are under 600 KB. The session-chat capture contains real local transcript content with tool calls collapsed; it does not expose the single-use login code.

| Design screen | Real view / evidence | Coverage |
|---|---|---|
| web-01 first-run connect code | Single-use `#code` exchange was exercised in the browser; transient login screen is not retained. | Login succeeded and authenticated API/SSE requests. |
| web-02 workspace shell | `workspace-shell.png` | Three-pane shell, projects, tabs and status bar. |
| web-03 project overview | `project-overview.png` | Current trusted project, session count, links and checklist. |
| web-04 session chat | `session-chat.png` | Real Claude transcript and collapsed tool calls. |
| web-05 run DAG; web-06 roles; web-07 message bus | Planned; no corresponding runtime data/view. | Intentionally not implemented. |
| web-08 global analytics; web-09 project analytics | `analytics.png` | Honest unavailable/no-usage-data state; no fabricated metrics. |
| web-10 monitor; web-11 logs; web-12 approvals; web-13 prompts; web-14 skills; web-15 plugins; web-16 brain; web-17 backup | Planned; no corresponding runtime data/view. | Intentionally not implemented. |
| web-18 settings | `settings.png` | Existing launch mode, language and dark/light settings. |
| web-19 cloud activation; web-20 sync | Planned; no cloud account or sync backend. | Intentionally not implemented. |
| web-21 empty/loading/error/offline | `empty-state.png`, `disconnect-state.png` | Empty trusted project and SSE reconnect state. API permission/server error variants were not exercised. |
| web-22 mobile/narrow | Responsive CSS, mobile project selector and bottom navigation are implemented; Orca retained a 2633×1660 page viewport when `window.resizeTo(390,844)` was requested. | A real 900 px/390 px screenshot is still unverified. |

## Defect evidence

1. **SSE disconnected — fixed.** `TestLaunchCodeAuthenticatesProjectsAndSSE` exercises single-use login, cookie-authenticated projects and an SSE stream that remains open. In the real browser the status changed to `Conectado`; after stopping the local server it changed to `Reconectando`.
2. **Trusted projects without sessions — fixed.** The browser listed `empty-project` with zero sessions and displayed its overview. The launched `gasacli` project was listed as current.
3. **Empty center — fixed.** `empty-state.png` shows the project cards, quick links, zero-session message and checklist.
4. **Analytics and Settings — fixed.** Both navigation items open real views; unknown analytics remain unavailable rather than zero. Settings uses the existing config writer.
5. **Brand — fixed.** The sidebar mark and favicon use the local G+chevron asset; no external assets are loaded.
6. **Workspace/session data — fixed for inspected paths.** The real browser displayed trusted project sessions and a Codex CLI transcript. Codex's `external_agent_tool_call/result` blocks are now parsed into collapsed tool rows; the redaction and parsing cases have deterministic tests.
7. **Light theme and narrow layout — partial.** Light/dark controls and responsive breakpoints exist in the implementation, but narrow/mobile behavior was not verified in a real narrow browser viewport.

The SSE root cause was the server-wide `WriteTimeout` expiring the long-lived event stream. The server now leaves streaming writes open, the handler flushes each event and applies a bounded per-write deadline; the browser opens the authenticated stream and retries with backoff after disconnect.

## Dashboard2 follow-up

- Sidebar rows now constrain the status dot, title and metadata within the row; the full session title is available as a tooltip, and the untrusted bucket label is in pt-BR.
- Session status is classified as working only when the transcript file changed within 10 seconds or the classifier is told the process is alive. Regression tests cover recent, old, future and live-process cases.
- Redaction now targets credential-shaped tokens, Authorization/Bearer values, private-key blocks and secret-named environment assignments. Benign tags, UUIDs, hashes and ordinary `.env` paths are preserved in tests.
- Consecutive tool records render in one collapsed summary row. A skeleton appears before transcript loading; session history loads in pages, and the initial detail page contains at most 100 messages.
- `internal/sessions.Sender` and an authenticated capabilities endpoint define the optional composer boundary. No sender is registered, so the composer remains disabled.
- Local Codex and Claude sessions were visible in the real browser; the browser reported `Conectado`, one trusted current project and seven local sessions. The Codex detail request loaded 100 messages and returned about 55 KB. Initial first-contentful-paint was 1.48 s; the page viewport was 2633×1660.
- The narrow CSS path now includes a project selector and bottom navigation. Orca did not honor the requested viewport resize, so mobile and ~900 px screenshots and layout behavior remain unverified.
- `gasa web status|stop|restart` was outside the allowed `internal/web`, `internal/sessions`, tests and screenshots paths; shell lifecycle behavior was not changed or reverified here.
