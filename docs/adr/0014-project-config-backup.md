# ADR-0014 — Project .gasa configuration, central SQLite, backup and cloud sync

Status: Accepted architectural recommendation, 2026-10-27.

## Decision
Use a hybrid local-first design:
- Project-local `.gasa/` for versionable, non-secret project settings, instructions and prompt overrides.
- User-data directory (OS-appropriate, conceptually ~/.gasacli/) for global config, plugins, optional Markdown brain, backups and a central SQLite database.
- Optional future E2EE Cloud sync of explicitly selected data; local state remains authoritative for offline operation.

## Project layout
```text
project/
  .gasa/
    config.yaml
    project.md
    agents/
    prompts/
  .claude/       # existing tool-owned content; read-only integration
  .agents/       # when present and recognized
  AGENTS.md      # when present
  src/
```
Do not create or modify tool-owned directories automatically. Respect tool-specific formats and precedence; selectively retrieve relevant instructions rather than injecting all files into every prompt. Treat repository instructions as untrusted relative to hard GasaCLI security policy.

## Local user data
```text
UserData/GasaCLI/
  config.yaml
  data/gasacli.db
  backups/
  prompts/
  plugins/
  segundo-cerebro/
```
Use OS-specific directories rather than assuming ~ on Windows. No per-project SQLite in MVP.

## Config precedence
Hard security policy > explicit user restrictions > project-specific settings > user defaults > built-in defaults. Project config may specialize agent roles and prompt templates but cannot elevate forbidden permissions. Secrets never stored in project config. Files under .gasa may be committed only after sanitization.

## SQLite
Store project registry (canonical paths and stable IDs), sessions, runs, tasks, dispatches, events, reviews, checks, usage, pricing provenance and sync metadata. Use migrations, WAL, transactional writes and bounded retention. Back up via consistent snapshot API, not copying an active WAL database naively.

## Backup / restore
Proposed commands:
`gasa backup create`, `gasa backup list`, `gasa backup restore <file>`; chat aliases `/backup` and `/restore`.
Backup format versioned, encrypted when configured, with manifest, checksums, compatibility info and scoped selections. Default: global config, project registry, prompt overrides, SQLite history and metadata; exclude OS-vault secrets, repository source and external Obsidian vault unless explicitly included. Restore previews changes, verifies integrity, creates pre-restore snapshot and supports selective restoration. Never overwrite project files silently.

## Cloud
Cloud sync remains opt-in. Use E2EE and per-device identity; sync selected configuration, prompts, memory and history with conflict resolution. Never sync credentials by default. Keep local SQLite on every device; server holds encrypted payloads and minimal account/billing metadata. Support offline operation and self-hosting.

## Acceptance
A project can be moved without silently exposing secrets; two projects have separate settings and one central analytics database; /backup restores after an interrupted session; unknown .claude/.agents files do not break execution; Cloud unavailable does not block local runs.

## Migration and performance
Config schema versioned, safe migrations and backward compatibility; lazy-load project instructions and keep indexing incremental. Backup and restore are explicit operations, not automatic upload.
