# One repository: Community + Cloud

Go is the primary language for the GasaCLI terminal application and native orchestration. Rust is reserved for optional benchmark-justified isolated components. The optional Cloud web dashboard may use TypeScript; Cloud API preferably Go.

## Planned layout
```text
cmd/gasa/               # Go entrypoint
internal/tui/           # Terminal chat
internal/core/          # Run/task/dispatch contracts
internal/orchestrator/  # Native coordination and quality gates
internal/discovery/     # Installed CLI probes
internal/adapters/      # Claude Code, Codex, Gemini etc.
internal/routing/       # Fixed/JEV selection
internal/storage/       # SQLite
internal/memory/        # Markdown + FTS5
internal/plugins/       # Optional integrations
pkg/pluginprotocol/     # External plugin process contract
rust/                   # Optional performance/security components
apps/cloud-api/         # Optional managed sync service
apps/cloud-web/         # Optional TypeScript dashboard
docs/
```

## Licensing and commercialization
All source in this repository is MIT-licensed unless a file explicitly states otherwise. Managed hosting and support may be paid; users retain the right to self-host or fork. Paid tiers cannot claim exclusive rights to MIT-licensed source.

## Privacy boundary
CLI requires no Cloud. Sync is opt-in and designed for E2EE with user-held keys. Do not sync provider credentials by default. Billing/account metadata is separate from encrypted sync payloads.

## Implementation status
Documentation only; paths are planned, not runnable. See [ADR-0006](adr/0006-go-rust-stack.md).
