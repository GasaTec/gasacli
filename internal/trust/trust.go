// Package trust persists explicit approval of canonical project paths.
package trust

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	home string
	mu   sync.Mutex
}

func NewStore(home string) *Store { return &Store{home: home} }

// Canonical resolves an existing project path to its absolute, symlink-free path.
func Canonical(path string) (string, error) { return canonical(path) }

func canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute project path: %w", err)
	}
	resolved, err := resolvePath(abs)
	if err != nil {
		return "", fmt.Errorf("resolve project symlinks: %w", err)
	}
	return resolved, nil
}

func (s *Store) read() (map[string]bool, error) {
	data, err := os.ReadFile(filepath.Join(s.home, "trust.json"))
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]bool), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project trust: %w", err)
	}
	var entries map[string]bool
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("decode project trust: %w", err)
	}
	if entries == nil {
		return nil, fmt.Errorf("decode project trust: expected JSON object")
	}
	return entries, nil
}

func (s *Store) IsTrusted(path string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, err := canonical(path)
	if err != nil {
		return false, err
	}
	entries, err := s.read()
	if err != nil {
		return false, err
	}
	return entries[key], nil
}

func (s *Store) Grant(path string) error  { return s.update(path, true) }
func (s *Store) Revoke(path string) error { return s.update(path, false) }

func (s *Store) update(path string, grant bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, err := canonical(path)
	if err != nil {
		return err
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	entries, err := s.read()
	if err != nil {
		return err
	}
	if grant {
		entries[key] = true
	} else {
		delete(entries, key)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project trust: %w", err)
	}
	if err := os.MkdirAll(s.home, 0700); err != nil {
		return fmt.Errorf("create trust directory: %w", err)
	}
	f, err := os.CreateTemp(s.home, ".trust-*")
	if err != nil {
		return fmt.Errorf("create temporary trust file: %w", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write project trust: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync project trust: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close project trust: %w", err)
	}
	if err := os.Rename(f.Name(), filepath.Join(s.home, "trust.json")); err != nil {
		return fmt.Errorf("replace project trust: %w", err)
	}
	return nil
}

// lock serializes read-modify-write operations across independent stores and
// processes. A crashed writer leaves a lock that may be reclaimed after 30s.
func (s *Store) lock() (func(), error) {
	if err := os.MkdirAll(s.home, 0700); err != nil {
		return nil, fmt.Errorf("create trust directory: %w", err)
	}
	path := filepath.Join(s.home, "trust.json.lock")
	deadline := time.Now().Add(2 * time.Second)
	backoff := 5 * time.Millisecond
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			info, statErr := f.Stat()
			closeErr := f.Close()
			if statErr != nil || closeErr != nil {
				os.Remove(path)
				return nil, fmt.Errorf("initialize project trust lock: %w", errors.Join(statErr, closeErr))
			}
			return func() {
				// Do not remove another writer's lock if ours was reclaimed.
				current, err := os.Stat(path)
				if err == nil && os.SameFile(info, current) {
					if err := os.Remove(path); err != nil {
						log.Printf("warning: remove project trust lock: %v", err)
					}
				}
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire project trust lock: %w", err)
		}
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect project trust lock: %w", err)
		}
		if time.Since(info.ModTime()) > 30*time.Second {
			if err := os.Remove(path); err == nil {
				log.Print("warning: removed stale project trust lock older than 30s")
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("remove stale project trust lock: %w", err)
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for project trust lock after 2s")
		}
		time.Sleep(backoff)
		if backoff < 50*time.Millisecond {
			backoff *= 2
		}
	}
}
