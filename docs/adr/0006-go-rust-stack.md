# ADR-0006 — Go-first CLI with selective Rust modules

Status: Accepted, 2026-10-27.

## Decision
Go is the primary implementation language for the GasaCLI terminal UI, native orchestrator, CLI discovery/adapters, process supervision, plugins, configuration and SQLite persistence. Rust is permitted for isolated, benchmark-justified performance or security components, not as a mandatory dependency for the first runnable MVP. The Cloud dashboard may use TypeScript; Cloud API language remains open (Go is preferred for shared operational expertise).

## Why
Go offers straightforward concurrency, cancellation, subprocess control, cross-platform binaries and approachable open-source contribution. Rust offers memory safety and fine-grained control, but dual-language complexity should be paid only when evidence warrants it.

## Go tool candidates
Cobra for command parsing, Bubble Tea for interactive terminal UI, SQLite driver chosen after CGO/cross-compilation evaluation, standard library context/exec for subprocess supervision. Validate Windows/Linux/macOS.

## Rust boundary
Use a versioned subprocess protocol (JSON Lines or equivalent) if a Rust worker is introduced. Avoid FFI initially; keep core contracts in Go. Benchmark before moving components to Rust.

## Proposed repository
```text
cmd/gasa/
internal/tui/
internal/core/
internal/orchestrator/
internal/discovery/
internal/adapters/
internal/routing/
internal/quality/
internal/storage/
internal/memory/
internal/plugins/
pkg/pluginprotocol/
rust/                   # optional future components
apps/cloud-api/         # optional managed service
apps/cloud-web/         # TypeScript dashboard
docs/
```

## Migration
Older TypeScript/Node.js CLI references in early ADRs and roadmaps are superseded by this decision. Do not rewrite historical ADRs; update canonical documentation and README. Existing issues mentioning TypeScript must be amended before implementation.

## Delivery
First deliver Go-only `gasa` chat, project trust, discovery and one CLI adapter; then /goal with durable state, review and quality gates. Add Rust only after profiling identifies a concrete need.
