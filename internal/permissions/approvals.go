package permissions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Approver interface {
	Ask(context.Context, Request) (Decision, error)
}
type NonInteractive struct{}

func (NonInteractive) Ask(_ context.Context, req Request) (Decision, error) {
	return Decision{Effect: Deny, Reason: "approval unavailable in non-interactive mode", Rule: req.Action, Scope: ProfileScope}, nil
}

type Scripted struct {
	Decisions []Decision
	Index     int
}

func (s *Scripted) Ask(_ context.Context, req Request) (Decision, error) {
	if s.Index >= len(s.Decisions) {
		return Decision{Effect: Deny, Reason: "no scripted decision", Rule: req.Action}, nil
	}
	d := s.Decisions[s.Index]
	s.Index++
	return d, nil
}

type ApprovalEvent struct {
	SchemaVersion int       `json:"schema_version"`
	EventID       string    `json:"event_id"`
	OccurredAt    time.Time `json:"occurred_at"`
	Decision      Effect    `json:"decision"`
	Actor         struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"actor"`
	Requester struct {
		AgentID string `json:"agent_id"`
		RunID   string `json:"run_id"`
	} `json:"requester"`
	Permission Permission `json:"permission"`
	Scope      struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"scope"`
	Operation struct {
		Summary string `json:"summary"`
		Digest  string `json:"digest,omitempty"`
	} `json:"operation"`
	ExpiresAt      *time.Time `json:"expires_at"`
	PolicyRevision string     `json:"policy_revision"`
	Reason         string     `json:"reason,omitempty"`
	PreviousHash   string     `json:"previous_hash,omitempty"`
	Hash           string     `json:"hash"`
}

var approvalMaxBytes = 1024 * 1024

var secretPattern = regexp.MustCompile(`(?i)(bearer\s+[^\s"']+|(?:token|password|secret|api[_-]?key|authorization)\s*[:=]\s*[^\s,;"']+)`)

func RedactTarget(target string) string {
	target = secretPattern.ReplaceAllString(target, "[REDACTED]")
	if len(target) > 512 {
		target = target[:512] + "…"
	}
	return target
}

// RecordApproval appends a hash-chained event to approvals/events.jsonl, rotating
// the active file before it would exceed 1 MiB. A lock serializes writers.
func (e *Engine) RecordApproval(event ApprovalEvent) error {
	dir := filepath.Join(e.Home, "approvals")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	unlock, err := lockApprovals(dir)
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(dir, "events.jsonl")
	previous, err := lastApprovalHash(path)
	if err != nil {
		return err
	}
	event.SchemaVersion = 1
	if event.EventID == "" {
		event.EventID, err = randomID()
		if err != nil {
			return err
		}
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	event.OccurredAt = event.OccurredAt.UTC()
	event.Operation.Summary = RedactTarget(event.Operation.Summary)
	event.Reason = RedactTarget(event.Reason)
	event.PreviousHash = previous
	event.Hash = ""
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	event.Hash = hex.EncodeToString(sum[:])
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	info, err := os.Stat(path)
	if err == nil && info.Size()+int64(len(line)) > int64(approvalMaxBytes) {
		rotated := filepath.Join(dir, fmt.Sprintf("events-%s.jsonl", time.Now().UTC().Format("20060102T150405.000000000")))
		if err = os.Rename(path, rotated); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(line); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

func lastApprovalHash(path string) (string, error) {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && (entry.Name() == "events.jsonl" || (strings.HasPrefix(entry.Name(), "events-") && strings.HasSuffix(entry.Name(), ".jsonl"))) {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	previous := ""
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for i, line := range lines {
			if line == "" {
				continue
			}
			var event ApprovalEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				return "", fmt.Errorf("decode approval event %d: %w", i+1, err)
			}
			if event.PreviousHash != previous {
				return "", fmt.Errorf("approval event %d has invalid previous hash", i+1)
			}
			stored := event.Hash
			event.Hash = ""
			raw, err := json.Marshal(event)
			if err != nil {
				return "", err
			}
			sum := sha256.Sum256(raw)
			if stored != hex.EncodeToString(sum[:]) {
				return "", fmt.Errorf("approval event %d hash mismatch", i+1)
			}
			previous = stored
		}
	}
	return previous, nil
}
func lockApprovals(dir string) (func(), error) {
	path := filepath.Join(dir, "events.lock")
	deadline := time.Now().Add(2 * time.Second)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, errors.New("timed out waiting for approval log lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:]), nil
}
