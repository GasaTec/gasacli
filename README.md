# GasaCLI

AI agent orchestration with fast personal knowledge retrieval, model routing and cost controls.

> Status: private, early-stage MVP.

## Vision
GasaCLI exposes a single `gasa` command to connect an Obsidian/Markdown vault, retrieve project decisions, select an execution strategy and delegate work to agents via pluggable runtimes such as Orca.

## Architecture
- **CLI:** TypeScript/Node.js command interface.
- **Decision Engine:** intent, relevant context, task risk and execution strategy.
- **Memory:** local-first Obsidian/Markdown adapter; incremental indexing with SQLite FTS5; explicit approval for vault writes.
- **Model Router:** configurable providers, capabilities, budgets and fallback.
- **Execution Adapter:** Orca integration without coupling core domain logic to Orca.
- **Quality Gate:** tests, review, audit trail and checkpoint/recovery.

## Planned commands (not implemented)
```sh
gasa init
gasa brain connect ~/Obsidian/Vault
gasa project add ./my-project
gasa plan "Implement authentication"
gasa run --budget 2
gasa status
```

## MVP success criteria
1. Connect and index a local Markdown vault without uploading it.
2. Retrieve relevant notes with source paths and show why they were selected.
3. Produce a validated execution plan and select providers under a configurable budget.
4. Run a real task through an execution adapter and resume after interruption.
5. Require approval for consequential writes and report tests, decisions and costs.

See [architecture](docs/architecture.md) and [roadmap](docs/roadmap.md).

## Security
Treat retrieved notes and tool output as untrusted data. Never execute instructions embedded in notes automatically. Do not expose API keys or private vault contents in logs. Require user approval for sensitive actions.

## License
Not selected. This repository is private; open-source licensing will be decided before any public release.
