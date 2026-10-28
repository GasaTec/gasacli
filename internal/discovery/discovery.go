package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	probeTimeout = 3 * time.Second
	outputLimit  = 4 * 1024
	versionLimit = 200
	workerLimit  = 4
	authNote     = "authentication not checked"
)

type Status string

type AuthStatus string

const (
	Ready          Status = "ready"
	NeedsLogin     Status = "needs_login"
	Installed      Status = "installed"
	Missing        Status = "missing"
	VersionUnknown Status = "version_unknown"
	Error          Status = "error"

	// Available and NotFound are retained as source-compatible aliases for
	// consumers that used the previous installation-only status vocabulary.
	Available Status = Installed
	NotFound  Status = Missing

	AuthUnknown    AuthStatus = "unknown"
	AuthReady      AuthStatus = "ready"
	AuthNeedsLogin AuthStatus = "needs_login"
)

// Result reports installation/version and, when checked by a documented
// non-interactive command, authentication readiness.
type Result struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Path    string     `json:"path,omitempty"`
	Status  Status     `json:"status"`
	Auth    AuthStatus `json:"auth_status"`
	Version string     `json:"version,omitempty"`
	Note    string     `json:"-"`
	Err     string     `json:"-"`
}

// Deps makes probes testable. Nil functions use the production defaults.
// Injected functions may run concurrently; Run must honor context cancellation.
// Injected Run implementations must also bound their own output allocations.
type Deps struct {
	LookPath func(string) (string, error)
	Run      func(context.Context, string, ...string) ([]byte, error)
}

// Discover probes a trusted catalog with at most four concurrent workers and
// returns one result per entry in catalog order, including after cancellation.
func Discover(ctx context.Context, catalog []Agent, deps Deps) []Result {
	if deps.LookPath == nil {
		deps.LookPath = exec.LookPath
	}
	if deps.Run == nil {
		deps.Run = runVersion
	}
	results := make([]Result, len(catalog))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for n := 0; n < workerLimit && n < len(catalog); n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				results[i] = probe(ctx, catalog[i], deps)
			}
		}()
	}
	for i := range catalog {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	return results
}

func probe(ctx context.Context, agent Agent, deps Deps) Result {
	r := Result{ID: agent.ID, Name: agent.Name, Status: Missing, Auth: AuthUnknown, Note: authNote}
	if ctx.Err() != nil {
		r.Status, r.Err = Error, "discovery canceled"
		return r
	}
	for _, binary := range agent.Binaries {
		if ctx.Err() != nil {
			r.Status, r.Err = Error, "discovery canceled"
			return r
		}
		path, err := deps.LookPath(binary)
		if err != nil {
			if !errors.Is(err, exec.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
				r.Status, r.Err = Error, "binary lookup failed"
			}
			continue
		}
		if path == "" {
			r.Status, r.Err = Error, "invalid binary path"
			return r
		}
		path, err = filepath.Abs(path)
		if err != nil {
			r.Status, r.Err = Error, "invalid binary path"
			return r
		}
		r.Path, r.Err = path, ""
		flag := agent.VersionFlag
		if flag == "" {
			flag = "--version"
		}
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		if probeCtx.Err() != nil {
			cancel()
			r.Status, r.Err = Error, "discovery canceled"
			return r
		}
		output, runErr := deps.Run(probeCtx, path, flag)
		ctxErr := probeCtx.Err()
		if ctxErr == nil && runErr == nil && agent.RequireText != "" && !strings.Contains(strings.ToLower(sanitize(string(output), versionLimit)), strings.ToLower(agent.RequireText)) {
			output, runErr = deps.Run(probeCtx, path, "--help")
			ctxErr = probeCtx.Err()
		}
		cancel()
		if ctxErr != nil || runErr != nil {
			r.Status, r.Err = VersionUnknown, "version probe failed"
			if errors.Is(ctxErr, context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded) {
				r.Err = "version probe timed out"
			} else if errors.Is(ctxErr, context.Canceled) || errors.Is(runErr, context.Canceled) {
				r.Err = "version probe canceled"
			}
			return r
		}
		if len(output) > outputLimit {
			output = output[:outputLimit]
		}
		r.Version = sanitize(string(output), versionLimit)
		if agent.RequireText != "" && !strings.Contains(strings.ToLower(r.Version), strings.ToLower(agent.RequireText)) {
			r.Path, r.Status, r.Version = "", NotFound, ""
			continue
		}
		r.Status = Installed
		if r.Version == "" {
			r.Status, r.Err = VersionUnknown, "version output empty"
		}
		if r.Status == Installed && len(agent.ReadyArgs) > 0 {
			authCtx, authCancel := context.WithTimeout(ctx, probeTimeout)
			authOutput, authErr := deps.Run(authCtx, path, agent.ReadyArgs...)
			authCtxErr := authCtx.Err()
			authCancel()
			if authCtxErr == nil {
				r.Auth = authVerdict(agent.ID, authOutput, authErr)
				if r.Auth == AuthReady {
					r.Status = Ready
					r.Note = ""
				} else if r.Auth == AuthNeedsLogin {
					r.Status = NeedsLogin
					r.Note = ""
				}
			}
		}
		return r
	}
	return r
}

// authVerdict interprets only documented status commands and never returns
// their output to callers.
func authVerdict(agentID string, output []byte, err error) AuthStatus {
	if err != nil {
		return AuthUnknown
	}
	switch agentID {
	case "codex":
		status := strings.ToLower(sanitize(string(output), versionLimit))
		if strings.HasPrefix(status, "logged in") {
			return AuthReady
		}
		if strings.HasPrefix(status, "not logged in") {
			return AuthNeedsLogin
		}
	case "opencode":
		var providers []json.RawMessage
		if json.Unmarshal(output, &providers) == nil {
			if len(providers) > 0 {
				return AuthReady
			}
			return AuthNeedsLogin
		}
	}
	return AuthUnknown
}

// cappedOutput retains only the first 4 KiB while draining the pipe, so a child
// cannot grow memory unboundedly or block on a full stdout pipe.
type cappedOutput struct{ data []byte }

func (b *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := outputLimit - len(b.data); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

func runVersion(ctx context.Context, path string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	configureProcessTree(cmd)
	cmd.WaitDelay = 100 * time.Millisecond
	// Keep only runtime/path essentials, never inherited API tokens or credentials.
	cmd.Env = []string{}
	for _, key := range []string{"PATH", "HOME", "USERPROFILE", "SystemRoot", "WINDIR", "PATHEXT", "TEMP", "TMP", "TMPDIR", "LOCALAPPDATA", "APPDATA", "CODEX_HOME"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	var output cappedOutput
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	err := cmd.Run()
	return output.data, err
}

// sanitize strips terminal escape sequences (CSI, OSC and related strings),
// control/format characters, and all but the first line. Limit counts runes.
func sanitize(s string, limit int) string {
	var clean []rune
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\n' || r == '\r' {
			break
		}
		if r == 0x1b || r == 0x9b || r == 0x9d {
			kind := r
			if r == 0x1b {
				i++
				if i >= len(runes) {
					break
				}
				kind = runes[i]
			}
			if kind == '[' || kind == 0x9b {
				for i++; i < len(runes); i++ {
					if runes[i] >= 0x40 && runes[i] <= 0x7e {
						break
					}
				}
			} else if kind == ']' || kind == 0x9d || kind == 'P' || kind == '^' || kind == '_' {
				for i++; i < len(runes); i++ {
					if runes[i] == 7 || runes[i] == 0x9c {
						break
					}
					if runes[i] == 0x1b && i+1 < len(runes) && runes[i+1] == '\\' {
						i++
						break
					}
				}
			}
			continue
		}
		if !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) && r != unicode.ReplacementChar {
			clean = append(clean, r)
		}
	}
	clean = []rune(strings.TrimSpace(string(clean)))
	if len(clean) > limit {
		clean = clean[:limit]
	}
	return string(clean)
}
