package engine

import "github.com/GasaTec/gasacli/internal/storage"

func init() {
	if err := storage.RegisterMigration(storage.Migration{
		Version: 10,
		Name:    "orchestration_runs",
		SQL: `CREATE TABLE runs (
			id TEXT PRIMARY KEY, project TEXT NOT NULL, goal TEXT NOT NULL,
			status TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		);
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			name TEXT NOT NULL, status TEXT NOT NULL, dependencies TEXT NOT NULL DEFAULT '[]',
			retries INTEGER NOT NULL DEFAULT 0, timeout_ms INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL
		);
		CREATE TABLE dispatches (
			id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			attempt INTEGER NOT NULL, worker TEXT NOT NULL DEFAULT '', status TEXT NOT NULL,
			started_at TEXT NOT NULL, finished_at TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE events (
			id INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			task_id TEXT NOT NULL DEFAULT '', dispatch_id TEXT NOT NULL DEFAULT '', type TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL
		);
		CREATE INDEX events_run_id_id ON events(run_id, id);
		CREATE TABLE checkpoints (
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE, task_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL, payload TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL,
			PRIMARY KEY(run_id, task_id, name)
		);`,
	}); err != nil {
		panic(err)
	}
}
