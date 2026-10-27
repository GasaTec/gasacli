# ADR-0005: Open-source core and optional managed cloud

Status: Product direction agreed; license and pricing pending review.

## Principles
- GasaCLI core is open-source and local-first. Full native orchestration, interactive /goal, multiple installed agents, independent review, editable prompts/skills, local second brain, GitHub plugin and local SQLite remain free.
- No required Gasatec account or paid cloud for core execution.
- Paid GasaCLI Cloud offers managed multi-device sync, encrypted backups, web management, device management and convenience features. Self-hosted sync should remain technically possible.
- No model API usage bundled by default; users use their own installed/authenticated coding CLIs and applicable provider terms.
- Do not upload source code, conversations or vault content by default. Optional sync must be opt-in, encrypted in transit and at rest; design for end-to-end encryption with user-held keys and transparent recovery limitations.
- Secrets stored in OS credential vault, not synced by default.
- Clearly distinguish subscription billing metadata from synced content.
- Validate pricing and retention costs before launch; no promise of unlimited storage.
- Select a real OSI-approved license before public release; consider Apache-2.0 vs AGPL-3.0 with legal review. A public GitHub repository without a license is not automatically open-source.
- Community: CONTRIBUTING.md, CODE_OF_CONDUCT.md, SECURITY.md, discussions, issue templates, CI, release signing, clear governance and contributor review.
- Keep repository private until secret scan, dependency/license review, documentation and a working minimum release.
