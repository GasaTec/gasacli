// Package storage provides the local SQLite database used by GasaCLI.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

const (
	busyTimeoutMS = 5000
	driverName    = "sqlite"
)

// Migration is a forward-only database schema change. Versions must be unique
// positive integers; migrations are applied in ascending version order.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

var migrations = struct {
	sync.RWMutex
	byVersion map[int]Migration
}{byVersion: make(map[int]Migration)}

// RegisterMigration adds a migration to the process registry. Call it from a
// package init function in the file that owns the schema being introduced.
func RegisterMigration(m Migration) error {
	if m.Version < 1 || strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.SQL) == "" {
		return errors.New("storage: migration needs a positive version, name, and SQL")
	}
	migrations.Lock()
	defer migrations.Unlock()
	if _, exists := migrations.byVersion[m.Version]; exists {
		return fmt.Errorf("storage: migration version %d is already registered", m.Version)
	}
	migrations.byVersion[m.Version] = m
	return nil
}

// Store is a handle to the global SQLite database.
type Store struct {
	db *sql.DB
}

// Open creates or opens the SQLite database at path, enables WAL and a bounded
// busy timeout, creates the schema_version table, and applies registered
// migrations. The caller owns the returned handle and must close it.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("storage: database path is empty")
	}
	if path != ":memory:" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("storage: resolve database path: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
			return nil, fmt.Errorf("storage: create database directory: %w", err)
		}
		path = abs
	}
	dsn := path
	if path != ":memory:" {
		uriPath := filepath.ToSlash(path)
		if volume := filepath.VolumeName(path); volume != "" {
			uriPath = "/" + uriPath
		}
		dsn = (&url.URL{Scheme: "file", Path: uriPath, RawQuery: "_pragma=journal_mode(WAL)&_pragma=busy_timeout(" + fmt.Sprint(busyTimeoutMS) + ")"}).String()
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: connect to database: %w", err)
	}
	if path != ":memory:" {
		if err := os.Chmod(path, 0600); err != nil {
			db.Close()
			return nil, fmt.Errorf("storage: restrict database permissions: %w", err)
		}
	}
	s := &Store{db: db}
	if err := s.Migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// DB returns the underlying database handle for query APIs not wrapped here.
func (s *Store) DB() *sql.DB { return s.db }

// Tx runs fn in a transaction, rolling it back on error and committing on
// success. A commit error is returned to the caller.
func (s *Store) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s == nil || s.db == nil {
		return errors.New("storage: store is closed")
	}
	if fn == nil {
		return errors.New("storage: transaction callback is nil")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("storage: rollback transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit transaction: %w", err)
	}
	committed = true
	return nil
}

// Migrate applies every registered migration newer than the current schema.
func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("storage: store is closed")
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("storage: create schema version table: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_version(version, name) VALUES (0, 'initial')`); err != nil {
		return fmt.Errorf("storage: initialize schema version: %w", err)
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("storage: read schema version: %w", err)
	}
	migrations.RLock()
	list := make([]Migration, 0, len(migrations.byVersion))
	for _, m := range migrations.byVersion {
		if m.Version > current {
			list = append(list, m)
		}
	}
	migrations.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Version < list[j].Version })
	for _, m := range list {
		if err := s.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
				return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Name, err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version(version, name) VALUES (?, ?)`, m.Version, m.Name); err != nil {
				return fmt.Errorf("record migration %d (%s): %w", m.Version, m.Name, err)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("storage: %w", err)
		}
		current = m.Version
	}
	return nil
}

// IntegrityCheck runs SQLite's built-in integrity check and requires the
// database to report exactly "ok".
func (s *Store) IntegrityCheck(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("storage: store is closed")
	}
	rows, err := s.db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("storage: run integrity check: %w", err)
	}
	defer rows.Close()
	var messages []string
	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			return fmt.Errorf("storage: read integrity check: %w", err)
		}
		if message != "ok" {
			messages = append(messages, message)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("storage: read integrity check: %w", err)
	}
	if len(messages) != 0 {
		return fmt.Errorf("storage: integrity check failed: %s", strings.Join(messages, "; "))
	}
	return nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
