# Storage

`storage.Open(path)` opens a pure-Go SQLite database, enables WAL and a 5 second
busy timeout, creates `schema_version`, and applies all registered migrations.
The global database path is `<config.Home()>/gasacli.db`. `Store.Tx(ctx, fn)`
provides an atomic transaction; `Store.IntegrityCheck(ctx)` checks the database,
and `Store.Close()` releases the handle.

## Adding a migration

Each feature owns its migration in its own package/file. Register it during
package initialization, using a globally unique, increasing version:

```go
func init() {
    if err := storage.RegisterMigration(storage.Migration{
        Version: 1,
        Name:    "create_sessions",
        SQL:     `CREATE TABLE sessions (id TEXT PRIMARY KEY, created_at TEXT NOT NULL)`,
    }); err != nil {
        panic(err)
    }
}
```

Migrations run in ascending version order inside individual transactions and
are recorded in `schema_version`. Versions are forward-only; do not reuse a
version or edit a migration after it has shipped. A migration package must be
linked into the executable (normally by importing its owning package) for its
`init` registration to run. Keep migration SQL self-contained and safe to run
once; do not modify a shared migration list.

The optional project directory is `<project>/gasacli/` and is created only by
`config.EnsureProjectLayout` after the project trust store confirms approval.
If committing generated projections is not desired, add this to the project
`.gitignore`:

```gitignore
/gasacli/session.md
```

`context.md` may be reviewed and committed as shared project context; do not
place secrets, provider credentials, or a database there.
