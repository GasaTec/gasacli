# One repository: Community + Cloud

The same GitHub repository will contain the open-source CLI and the optional managed cloud service implementation.

## Planned layout
```
apps/
  cli/               # Interactive terminal UI
  cloud-api/         # Optional managed sync API
  cloud-web/         # Optional account and device dashboard
packages/
  core/              # Runs, tasks, dispatches, permissions
  agent-adapters/    # Claude Code, Codex, Gemini, Cursor, etc.
  orchestrator/      # Native coordination and quality gates
  model-router/      # Fixed or optional JEV-based selection
  memory/            # Markdown vault + SQLite FTS5
  plugin-sdk/        # Plugin contracts
  plugins-github/    # gh-based issue integration
  plugins-jev/       # Optional JEV connection
  sync-protocol/     # Optional encrypted multi-device sync
docs/
```

## Licensing and commercialization
All source in this repository is MIT-licensed unless a file explicitly states otherwise. Hosting a managed sync service, providing support and operating infrastructure can be paid offerings, while users retain the right to self-host or fork the MIT-licensed software. Paid tiers must not be represented as exclusive rights to code published under MIT.

If the business later requires proprietary code, do not silently mix it into this MIT-licensed repository; obtain a deliberate licensing and repository architecture decision first.

## Privacy boundary
The CLI does not require the cloud. The managed cloud should store billing/account/device metadata separately from end-to-end encrypted sync payloads. Do not sync provider credentials by default. Cloud deployment, billing and account management remain future work.

## Implementation status
Architecture and governance documentation only. Paths above are planned, not yet runnable.
