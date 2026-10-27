# ADR-0009 — Embedded localhost dashboard and port policy

Status: Accepted product direction, 2026-10-27.

## Distribution
Ship GasaCLI's Go CLI/TUI and compiled web dashboard assets together in the same distributable. Embed static assets in the Go binary with go:embed where practical. No separate Node.js installation or web frontend download required for end users. PWA is an optional installable shell, not a second backend.

## UX
- `gasa`: open terminal chat and ensure local dashboard server is available.
- `gasa ui`: open the existing dashboard in the browser.
- Default URL: http://127.0.0.1:3199
- Dashboard shows agents, role/task ownership, messages, RAM/CPU, provider-reported tokens, API-equivalent estimates and quality gates.
- TUI and web clients connect to one Go orchestrator and persistent local SQLite state.

## Port policy
3199 is a preference, never a reserved or guaranteed-unused port. Bind loopback only. If occupied, first verify whether an existing compatible GasaCLI instance is listening via authenticated local handshake; if yes reuse it. Otherwise bind an OS-selected free loopback port (port 0), persist actual endpoint securely, and print/open the actual URL. Never kill or hijack an unrelated process. Avoid port-scan loops and TOCTOU race between probing and binding.

## Process lifecycle
Single-instance coordination through OS-appropriate lock plus authenticated handshake. Support multiple TUI/web clients to the same engine. Avoid duplicate agent runs. Stop server when no sessions or background goals require it; support explicit daemon-like mode later if needed. Cleanly cancel/reap child processes on shutdown and persist resumable state.

## Security
- Listen on 127.0.0.1 by default; do not bind 0.0.0.0.
- Treat localhost as untrusted: random session secret stored with restrictive permissions, authenticated API/SSE/WebSocket requests, Origin and Host validation, CSRF defenses, safe cookie settings and no permissive CORS.
- Avoid passing session tokens in URL query strings, command-line arguments or logs.
- Prevent browser pages from accessing sensitive APIs without authorization.
- Project trust and command approvals enforced in Go core, never solely in frontend.
- Explicit opt-in and separate threat model for any future remote access.
- PWA service worker scope limited to local dashboard assets; do not cache secrets or private content.

## Performance
HTTP server and embedded assets must remain lightweight; lazy-load frontend views and metrics, avoid continuous polling and bounded event queues. Dashboard/browser RAM measured separately from Go core. Benchmark with TUI-only and TUI+web scenarios.

## Acceptance
One installed binary starts `gasa`, serves local UI on 3199 when available, falls back safely if occupied, reuses a genuine existing instance, blocks unauthorized local requests and keeps orchestration state consistent across terminal and browser.
