// Package orca is a bounded, version-discovered adapter for Orca's public CLI.
// It never opens Orca, consumes a mailbox, or fabricates coordinator identity.
package orca

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/integrations/host"
)

var ErrCoordinatorAuthority = errors.New("no supported persistent coordinator identity bound to this gateway")

type Runner = host.Runner
type Command = host.Command
type Result = host.Result
type Receipt struct {
	Process   Result          `json:"process"`
	OK        bool            `json:"ok"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     json.RawMessage `json:"error,omitempty"`
	RuntimeID string          `json:"runtimeId,omitempty"`
	Unknown   bool            `json:"unknown"`
}
type Capability struct {
	Available            bool     `json:"available"`
	Compatible           bool     `json:"compatible"`
	CanObserve           bool     `json:"canObserve"`
	CanExecute           bool     `json:"canExecute"`
	CanLaunchCoordinator bool     `json:"canLaunchCoordinator"`
	CanSubmitCoordinator bool     `json:"canSubmitCoordinator"`
	RuntimeID            string   `json:"runtimeId,omitempty"`
	Version              string   `json:"version,omitempty"`
	HostID               string   `json:"hostId"`
	Executable           string   `json:"executable,omitempty"`
	Reason               string   `json:"reason,omitempty"`
	Capabilities         []string `json:"capabilities"`
}
type commandSchema struct {
	Command string   `json:"command"`
	Flags   []string `json:"flags"`
}
type Bridge struct {
	mu              sync.Mutex
	executable, dir string
	runner          Runner
	capability      Capability
	commands        map[string]map[string]bool
	coordinator     CoordinatorTransport
	binding         CoordinatorBinding
}

// New resolves once and never silently falls through to a different build.
func New(dir string) (*Bridge, error) {
	executable := os.Getenv("ORCA_CLI_COMMAND")
	if executable == "" {
		switch {
		case os.Getenv("ORCA_DEV_REPO_ROOT") != "":
			executable = "orca-dev"
		case runtime.GOOS == "linux":
			executable = "orca-ide"
		default:
			executable = "orca"
		}
	}
	path, err := exec.LookPath(executable)
	if err != nil {
		return nil, err
	}
	return NewWithRunner(path, dir, host.ExecRunner{})
}
func NewWithRunner(executable, dir string, runner Runner) (*Bridge, error) {
	if runner == nil || executable == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("absolute workspace, executable and runner required")
	}
	return &Bridge{executable: executable, dir: dir, runner: runner, commands: map[string]map[string]bool{}, capability: Capability{HostID: host.HostID(), Executable: executable, Reason: "not probed", Capabilities: []string{}}}, nil
}
func (b *Bridge) Capabilities() Capability {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.capability
	c.Capabilities = append([]string(nil), c.Capabilities...)
	return c
}

// Probe reads public schema/status only. A compatible host is observation-ready;
// its running user's agent session is not authority for this HTTP gateway.
func (b *Bridge) Probe(ctx context.Context) (Capability, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.capability = Capability{HostID: host.HostID(), Executable: b.executable, Capabilities: []string{}}
	schema, err := b.run(ctx, []string{"agent-context", "--json"}, 10*time.Second, false)
	if err != nil {
		b.capability.Reason = "Public command schema unavailable"
		return b.capability, err
	}
	var catalog struct {
		SchemaVersion int             `json:"schemaVersion"`
		Commands      []commandSchema `json:"commands"`
	}
	// agent-context is a local command and returns a bare schema, not an RPC envelope.
	if err = json.Unmarshal([]byte(schema.Process.Stdout), &catalog); err != nil || catalog.SchemaVersion != 1 {
		b.capability.Reason = "Unsupported public command schema"
		return b.capability, errors.New(b.capability.Reason)
	}
	b.commands = map[string]map[string]bool{}
	for _, c := range catalog.Commands {
		flags := map[string]bool{}
		for _, flag := range c.Flags {
			flags[flag] = true
		}
		b.commands[c.Command] = flags
	}
	status, err := b.run(ctx, []string{"status", "--json"}, 8*time.Second, false)
	if err != nil {
		b.capability.Reason = "Orca runtime unavailable; gateway does not start or restart it"
		return b.capability, err
	}
	var state struct {
		Runtime struct {
			RuntimeID    string   `json:"runtimeId"`
			Version      string   `json:"appVersion"`
			State        string   `json:"state"`
			Reachable    bool     `json:"reachable"`
			Capabilities []string `json:"capabilities"`
		} `json:"runtime"`
	}
	if err = json.Unmarshal(status.Result, &state); err != nil {
		return b.capability, err
	}
	b.capability.Available = state.Runtime.Reachable && state.Runtime.State == "ready"
	b.capability.RuntimeID = state.Runtime.RuntimeID
	b.capability.Version = state.Runtime.Version
	b.capability.Capabilities = state.Runtime.Capabilities
	b.capability.Compatible = b.capability.Available && state.Runtime.RuntimeID != "" && contains(state.Runtime.Capabilities, "orchestration.contract.v1")
	for _, required := range []string{"orchestration run-show", "orchestration worker-show", "orchestration worker-read", "orchestration inbox", "orchestration request-show"} {
		if _, ok := b.commands[required]; !ok {
			b.capability.Compatible = false
		}
	}
	b.capability.CanObserve = b.capability.Compatible
	b.capability.CanLaunchCoordinator = b.capability.Compatible && b.commands["agent launch"]["retry-launch"] && contains(state.Runtime.Capabilities, "agent.launch.replay.v1")
	b.capability.CanSubmitCoordinator = b.capability.Compatible && b.commands["terminal send"]["retry-request"] && b.commands["terminal send"]["wait-submit"] && contains(state.Runtime.Capabilities, "terminal.prompt-delivery.v1")
	b.capability.Reason = ErrCoordinatorAuthority.Error()
	if !b.capability.Compatible {
		b.capability.Reason = "Runtime lacks the supported public orchestration observation contract"
	}
	return b.capability, nil
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (b *Bridge) run(ctx context.Context, args []string, timeout time.Duration, mutation bool) (Receipt, error) {
	process, err := b.runner.Run(ctx, Command{Executable: b.executable, Args: args, Dir: b.dir, Timeout: timeout})
	r := Receipt{Process: process, Unknown: mutation && process.Started}
	var envelope struct {
		OK     *bool           `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
		Meta   struct {
			RuntimeID string `json:"runtimeId"`
		} `json:"_meta"`
	}
	parseErr := json.Unmarshal([]byte(process.Stdout), &envelope)
	if parseErr == nil {
		r.Result = envelope.Result
		r.Error = envelope.Error
		r.RuntimeID = envelope.Meta.RuntimeID
		if envelope.OK != nil {
			r.OK = *envelope.OK
		}
	}
	if err != nil {
		return r, err
	}
	if process.Truncated {
		return r, errors.New("Orca response exceeded output limit")
	}
	if process.ExitCode != 0 {
		return r, errors.New("Orca process returned nonzero exit")
	}
	if len(args) > 0 && args[0] == "agent-context" {
		if parseErr != nil {
			return r, errors.New("invalid command schema JSON")
		}
		r.OK = true
		return r, nil
	}
	if parseErr != nil || envelope.OK == nil {
		return r, errors.New("invalid Orca response envelope")
	}
	if !r.OK {
		return r, errors.New("Orca rejected public operation")
	}
	r.Unknown = false
	return r, nil
}

func validSelector(value string) bool {
	return value != "" && len(value) <= 512 && !strings.HasPrefix(value, "-") && !strings.ContainsAny(value, "\x00\r\n")
}
func (b *Bridge) observe(ctx context.Context, command string, flags ...string) (Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.capability.CanObserve {
		return Receipt{}, errors.New("Orca observation contract not verified")
	}
	known, ok := b.commands[command]
	if !ok {
		return Receipt{}, errors.New("command absent from host public schema")
	}
	args := strings.Split(command, " ")
	for index := 0; index < len(flags); index += 2 {
		if index+1 >= len(flags) || !known[strings.TrimPrefix(flags[index], "--")] || !validSelector(flags[index+1]) {
			return Receipt{}, errors.New("invalid public command argument")
		}
		args = append(args, flags[index], flags[index+1])
	}
	args = append(args, "--json")
	r, err := b.run(ctx, args, 10*time.Second, false)
	if r.RuntimeID != "" && r.RuntimeID != b.capability.RuntimeID {
		b.capability.CanObserve = false
		b.capability.Compatible = false
		b.capability.Reason = "Runtime identity changed; reconcile existing work before rebinding"
		return r, errors.New(b.capability.Reason)
	}
	return r, err
}
func (b *Bridge) Runs(ctx context.Context) (Receipt, error) {
	return b.observe(ctx, "orchestration run-list", "--limit", "50")
}
func (b *Bridge) Workspaces(ctx context.Context) (Receipt, error) {
	return b.observe(ctx, "worktree list", "--limit", "100")
}
func (b *Bridge) Plugins(ctx context.Context) (Receipt, error) { return b.observe(ctx, "plugin list") }
func (b *Bridge) Terminal(ctx context.Context, handle, cursor string) (Receipt, error) {
	if !validSelector(handle) {
		return Receipt{}, errors.New("explicit runtime terminal handle required")
	}
	flags := []string{"--terminal", handle, "--limit", "100"}
	if cursor != "" {
		if _, err := strconv.ParseUint(cursor, 10, 64); err != nil {
			return Receipt{}, errors.New("invalid terminal cursor")
		}
		flags = append(flags, "--cursor", cursor)
	}
	return b.observe(ctx, "terminal read", flags...)
}
func (b *Bridge) Run(ctx context.Context, id string) (Receipt, error) {
	return b.observe(ctx, "orchestration run-show", "--id", id)
}
func (b *Bridge) Tasks(ctx context.Context, runID string) (Receipt, error) {
	return b.observe(ctx, "orchestration task-list", "--run", runID)
}
func (b *Bridge) Worker(ctx context.Context, dispatchID string) (Receipt, error) {
	return b.observe(ctx, "orchestration worker-show", "--dispatch", dispatchID)
}
func (b *Bridge) ReadWorker(ctx context.Context, dispatchID, cursor string, limit int) (Receipt, error) {
	return b.readWorker(ctx, dispatchID, cursor, limit, "auto")
}

// ReadWorkerTranscript never falls back to terminal output. Its receipt still
// needs the public-message projector before it is sent to a browser.
func (b *Bridge) ReadWorkerTranscript(ctx context.Context, dispatchID, cursor string, limit int) (Receipt, error) {
	return b.readWorker(ctx, dispatchID, cursor, limit, "transcript")
}

func (b *Bridge) readWorker(ctx context.Context, dispatchID, cursor string, limit int, source string) (Receipt, error) {
	if limit < 1 || limit > 1000 {
		return Receipt{}, errors.New("worker output limit must be between 1 and 1000")
	}
	flags := []string{"--dispatch", dispatchID, "--source", source, "--limit", strconv.Itoa(limit)}
	if cursor != "" {
		flags = append(flags, "--cursor", cursor)
	}
	return b.observe(ctx, "orchestration worker-read", flags...)
}

// Inbox observes only. `check`, including its consuming default, is never exposed.
func (b *Bridge) Inbox(ctx context.Context, recipient string) (Receipt, error) {
	if !validSelector(recipient) {
		return Receipt{}, errors.New("explicit runtime recipient required")
	}
	return b.observe(ctx, "orchestration inbox", "--terminal", recipient, "--limit", "100")
}
func (b *Bridge) Request(ctx context.Context, requestID string) (Receipt, error) {
	return b.observe(ctx, "orchestration request-show", "--request", requestID)
}

// Public mutation vectors are deliberately typed and built here. The standalone
// gateway has no documented persistent coordinator binding in the current CLI.
// Returning an explicit error prevents using inherited session ids, --from, or
// --run as forged authority. No provider is launched on this unsupported path.
type WorkerRequest struct{ RunID, TaskID, AgentID, Worktree, Model, Effort string }
