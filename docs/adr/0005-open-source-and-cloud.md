# ADR-0005: Open-source core and optional managed cloud

Status: Accepted product direction, 2026-10-28. The repository is public and the chosen license is MIT; Cloud pricing and service are not yet available.

## Principles
- GasaCLI core is open-source and local-first. Full native orchestration, interactive /goal, multiple installed agents, independent review, editable prompts/skills, local second brain, GitHub plugin and local SQLite remain free.
- No required Gasatec account or paid cloud for core execution.
- Paid GasaCLI Cloud offers managed multi-device sync, encrypted backups, web management, device management and convenience features. Self-hosted sync should remain technically possible.
- No model API usage bundled by default; users use their own installed/authenticated coding CLIs and applicable provider terms.
- Do not upload source code, conversations or vault content by default. Optional sync must be opt-in, encrypted in transit and at rest; design for end-to-end encryption with user-held keys and transparent recovery limitations.
- Secrets stored in OS credential vault, not synced by default.
- Clearly distinguish subscription billing metadata from synced content.
- Validate pricing and retention costs before launch; no promise of unlimited storage.
- The repository uses the MIT License in `LICENSE`; this records the chosen repository license and is not a legal opinion on ownership of every contribution.
- Community governance, private vulnerability reporting, secret scanning, Discussions, branch protection and dependency license inventory are tracked in `docs/launch-checklist.md`. Repository configuration items remain unverified until a maintainer checks them.
- The repository is already public. This ADR does not claim that secret scanning, private vulnerability reporting, Discussions, branch protection or any other GitHub setting is enabled.
