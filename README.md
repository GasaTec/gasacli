# GasaCLI

**Make your coding AIs work together.** A local-first, open-source terminal orchestrator for coding agents already installed on your machine.

> **Status:** pre-alpha. The architecture, governance and roadmap are documented; the interactive CLI and cloud service are not implemented yet.

## Vision
Run `gasa` in a trusted project directory to open a terminal chat. Use `/goal` to request an outcome, not a sequence of prompts. GasaCLI will discover available coding CLIs, plan work, coordinate implementers and independent reviewers, and only mark tasks DONE after applicable verification gates pass. If blocked, it must explain why rather than loop indefinitely.

## Planned capabilities
- Interactive chat with `/goal`, `/agents`, `/config`, `/prompts`, `/skills`, `/brain`, `/github` and `/resume`.
- Installed agent discovery for supported CLIs (Claude Code, Codex, Gemini CLI, Cursor CLI, OpenCode and others), subject to each provider's supported interfaces and account limits.
- Native durable orchestration: run/task/dispatch/worker events, retries, checkpoints, worktrees, test/build gates and independent review.
- Fixed agent routing through editable YAML, or optional intelligent routing via the JEV plugin.
- Versioned, editable prompts, microagents and opt-in third-party skills.
- Optional Markdown/Obsidian second brain, indexed locally using SQLite FTS5.
- Optional GitHub Issues integration via the `gh` CLI; Linear may follow.
- Optional Orca integration, both as an executor and (if supported upstream) a selectable hosted agent.

## Technology decisions
**Go** powers the CLI, native orchestrator, agent discovery and local SQLite integration. **Rust** is reserved for isolated components when profiling justifies it. **TypeScript** may power the optional Cloud web dashboard. See [ADR-0006](docs/adr/0006-go-rust-stack.md). Earlier TypeScript CLI proposals are superseded.

## Community and Cloud in one repository
**Community:** fully functional local orchestration, no Gasatec account required, free and open-source.

**GasaCLI Cloud (planned):** paid managed multi-device synchronization, encrypted backups and device management. Sync is opt-in, and a self-hosted implementation may be used instead. Cloud does not bundle third-party AI subscriptions.

All repository code is MIT-licensed. The commercial offering charges for hosting and service, not exclusive rights to the source.

See [monorepo design](docs/monorepo.md), [interactive UX](docs/adr/0004-interactive-cli-and-github-plugin.md), [orchestration contract](docs/adr/0003-quality-gated-orchestration.md), and [open-source direction](docs/adr/0005-open-source-and-cloud.md).

## Planned commands (not yet available)
```sh
gasa
gasa doctor
gasa goal "Implement a login screen"
gasa brain connect ~/Obsidian/Vault
gasa integration install orca
```

## Contribute
Read [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md). Issues and PRs are welcome once the repository is public.

## Security and privacy
Project trust approval comes before accessing files. Credentials belong in the OS credential vault, not Git or logs. Source files and Markdown are untrusted input. No automatic code or vault upload to Gasatec. Third-party model providers process data according to their own terms when used.

## License
[MIT](LICENSE) © 2026 Gasatec.
