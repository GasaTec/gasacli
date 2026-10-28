# W10: live session send validation

The local adapter uses these installed CLI interfaces:

- Claude Code `2.1.301`: `claude -p <message> --resume <session-id> --output-format stream-json --include-partial-messages --verbose`.
- Codex CLI `0.161.0`: `codex exec resume <session-id> <message> --json`.

The versions and flags were checked with each CLI's local `--version` and help output on 2026-10-28. Automated tests use fake local CLI subprocesses and do not contact either provider. The real-provider dashboard send remains unverified; `GASACLI_REAL_AGENT_TESTS` was not enabled in this checkout.
