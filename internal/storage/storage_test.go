package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesWALDatabaseAndVersionTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "gasacli.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var mode string
	if err := store.DB().QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || strings.ToLower(mode) != "wal" {
		t.Fatalf("journal_mode=%q err=%v", mode, err)
	}
	var tableCount int
	if err := store.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&tableCount); err != nil || tableCount != 1 {
		t.Fatalf("initial table count=%d err=%v; want only schema_version", tableCount, err)
	}
	var version int
	if err := store.DB().QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil || version != 0 {
		t.Fatalf("initial version=%d err=%v", version, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterMigrationAndMigrateForwardOnly(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	migration := Migration{Version: 1, Name: "test_table", SQL: `CREATE TABLE migration_test (id INTEGER PRIMARY KEY)`}
	if err := RegisterMigration(migration); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.DB().QueryRow(`SELECT count(*) FROM schema_version WHERE version=1 AND name='test_table'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("migration record count=%d err=%v", count, err)
	}
	if _, err := store.DB().Exec(`INSERT INTO migration_test(id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if err := store.IntegrityCheck(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTxCommitsAndRollsBack(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.DB().Exec(`CREATE TABLE values_test (value TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := store.Tx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO values_test(value) VALUES ('committed')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	wantRollback := os.ErrPermission
	if err := store.Tx(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO values_test(value) VALUES ('rolled back')`); err != nil {
			return err
		}
		return wantRollback
	}); err != wantRollback {
		t.Fatalf("Tx error=%v, want callback error", err)
	}
	var count int
	if err := store.DB().QueryRow(`SELECT count(*) FROM values_test`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("committed row count=%d err=%v", count, err)
	}
}

func TestRegisterMigrationValidatesInputs(t *testing.T) {
	for _, migration := range []Migration{{}, {Version: 1, Name: " ", SQL: "SELECT 1"}, {Version: 1, Name: "missing_sql"}} {
		if err := RegisterMigration(migration); err == nil {
			t.Fatalf("accepted invalid migration: %+v", migration)
		}
	}
}
