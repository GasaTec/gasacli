# ADR-0016 — Canonical gasacli directory names and global backup/restore

Status: Accepted (2026-10-27). Supersedes naming and backup scope in ADR-0014/0015.

## Naming
- Global data directory is named `gasacli` under the OS-appropriate user data root. On Linux it may be `~/.local/share/gasacli`; on Windows use an appropriate AppData directory; on macOS use Application Support. Do not assume the same parent path on all systems.
- Optional project directory is `<project>/gasacli/`, **not** `.gasa/` or `.gasacli/`. It is only created on explicit activation.

## Global structure (conceptual)
```text
gasacli/
  config.yaml
  gasacli.db
  agents/
  prompts/
  skills/
  plugins/
  segundo-cerebro/
  backups/
  logs/
```
Global agent definitions, prompts, policy, SQLite runs/messages and plugin state live here.

## Optional project structure
```text
project/
  gasacli/
    context.md
    session.md
```
These are bounded, generated, human-readable projections for shared agent context. They are not authoritative execution state, and are never required for core orchestration. The user decides whether to enable them and whether to commit them to Git. No secrets, global agent copies or per-project database.

## Commands
- `/project enable`: opt in to creating project/gasacli.
- `/project disable`: stop generating project projections; do not delete files without confirmation.
- `/project status`: report active project context mode.
- `/backup` and `gasa backup create`: create a global snapshot.
- `/restore` and `gasa backup restore <archive>`: preview and restore global state with confirmation.

## Backup scope
Global backup includes config, agent/prompt customizations, plugin settings without secrets, project registry, sessions, run/task/message/check/review history and analytics via a transactionally consistent SQLite snapshot. Optional explicit inclusions: brain Markdown and project context projections. Never include source repositories or OS credential-vault secrets by default.

## Restore safety
Versioned manifest, checksums, encryption option, integrity verification, compatibility checks, selective restore, pre-restore snapshot and clear overwrite preview. If original project paths no longer exist, retain history and allow relinking. Never blindly overwrite external project files.

## Cloud
Opt-in encrypted sync can replicate selected global state between devices; local SQLite remains available offline. Sync is not a replacement for local backup or provider credential management.

## Compatibility
Older documentation referencing `.gasa/` is historical and superseded by this ADR. Update implementation issues and canonical docs to use `gasacli/`.
