// Package adapters runs installed coding CLIs with bounded output and explicit permissions.
package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GasaTec/gasacli/internal/permissions"
)

const defaultOutputLimit = 1 << 20
const defaultTimeout = 15 * time.Minute

type Request struct {
	Agent       string
	Project     string
	Prompt      string
	Profile     string
	Model       string
	Timeout     time.Duration
	Environment map[string]string
}
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}
type Result struct {
	ExitCode    int `json:"exit_code"`
	Events      int `json:"events"`
	OutputBytes int `json:"output_bytes"`
}
type Executor struct {
	Policy      *permissions.Engine
	Approver    permissions.Approver
	OutputLimit int
	OnEvent     func(Event)
	LookPath    func(string) (string, error)
}

func (e *Executor) Execute(ctx context.Context, request Request) (Result, error) {
	if e.Policy == nil {
		return Result{}, errors.New("adapter: permission engine is required")
	}
	if request.Project == "" || request.Prompt == "" {
		return Result{}, errors.New("adapter: project and prompt are required")
	}
	project, err := filepath.Abs(request.Project)
	if err != nil {
		return Result{}, err
	}
	if info, err := os.Stat(project); err != nil || !info.IsDir() {
		return Result{}, errors.New("adapter: project must be an existing directory")
	}
	profile, err := e.Policy.Profile()
	if err != nil {
		return Result{}, fmt.Errorf("adapter: load permission profile: %w", err)
	}
	request.Profile = profile
	command, args, err := commandFor(request)
	if err != nil {
		return Result{}, err
	}
	if e.LookPath != nil {
		path, lookErr := e.LookPath(command)
		if lookErr != nil {
			return Result{}, fmt.Errorf("adapter: %s CLI not found", request.Agent)
		}
		command = path
	}
	for _, action := range []permissions.Permission{"write_project", "execute_commands", "network_access"} {
		req := permissions.Request{Agent: request.Agent, Action: action, Project: project, Target: "."}
		decision := e.Policy.Decide(ctx, req)
		if decision.Effect == permissions.Ask {
			if e.Approver == nil {
				return Result{}, fmt.Errorf("adapter: permission %s requires approval", action)
			}
			decision, err = e.Approver.Ask(ctx, req)
			if err != nil {
				return Result{}, fmt.Errorf("adapter: permission %s approval: %w", action, err)
			}
		}
		if decision.Effect != permissions.Allow {
			return Result{}, fmt.Errorf("adapter: permission %s denied: %s", action, decision.Reason)
		}
	}
	commandCtx := ctx
	cancel := func() {}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	commandCtx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, command, args...)
	cmd.Dir = project
	cmd.Env = allowedEnvironment(request.Environment)
	cmd.WaitDelay = time.Second
	configureProcessTree(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, err
	}
	if err = cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("adapter: start %s: %w", request.Agent, err)
	}
	limit := e.OutputLimit
	if limit <= 1 || limit > defaultOutputLimit {
		limit = defaultOutputLimit
	}
	result := Result{ExitCode: -1}
	stdoutLimit, stderrLimit := limit/2, limit-limit/2
	var stderrBytes atomic.Int64
	stderrDone := make(chan struct{})
	stderrOverflow := make(chan struct{}, 1)
	go func() {
		defer close(stderrDone)
		count, _ := io.Copy(io.Discard, io.LimitReader(stderr, int64(stderrLimit)+1))
		stderrBytes.Store(count)
		if count > int64(stderrLimit) {
			stderrOverflow <- struct{}{}
		}
	}()
	go func() {
		select {
		case <-stderrOverflow:
			_ = terminateProcessTree(cmd)
		case <-commandCtx.Done():
		}
	}()
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 32*1024), stdoutLimit)
	for lines.Scan() {
		line := lines.Bytes()
		result.OutputBytes += len(line) + 1
		if result.OutputBytes > stdoutLimit {
			_ = terminateProcessTree(cmd)
			_ = cmd.Wait()
			<-stderrDone
			return result, errors.New("adapter: output limit exceeded")
		}
		if strings.TrimSpace(string(line)) != "" {
			var event Event
			if err := json.Unmarshal(line, &event); err != nil {
				_ = terminateProcessTree(cmd)
				_ = cmd.Wait()
				<-stderrDone
				return result, errors.New("adapter: invalid structured event")
			}
			result.Events++
			if e.OnEvent != nil {
				event.Data = nil
				e.OnEvent(event)
			}
		}
		if commandCtx.Err() != nil {
			_ = terminateProcessTree(cmd)
			_ = cmd.Wait()
			<-stderrDone
			return result, commandCtx.Err()
		}
	}
	if scanErr := lines.Err(); scanErr != nil {
		_ = terminateProcessTree(cmd)
		_ = cmd.Wait()
		<-stderrDone
		return result, fmt.Errorf("adapter: read output: %w", scanErr)
	}
	waitErr := cmd.Wait()
	<-stderrDone
	result.OutputBytes += int(stderrBytes.Load())
	if result.OutputBytes > limit {
		return result, errors.New("adapter: output limit exceeded")
	}
	if waitErr != nil {
		if commandCtx.Err() != nil {
			return result, commandCtx.Err()
		}
		if exit, ok := waitErr.(*exec.ExitError); ok {
			result.ExitCode = exit.ExitCode()
		}
		return result, fmt.Errorf("adapter: %s exited with status %d", request.Agent, result.ExitCode)
	}
	result.ExitCode = 0
	return result, nil
}

func commandFor(request Request) (string, []string, error) {
	switch strings.ToLower(request.Agent) {
	case "claude":
		args := []string{"-p", request.Prompt, "--output-format", "stream-json", "--verbose"}
		if request.Model != "" {
			args = append(args, "--model", request.Model)
		}
		if request.Profile == "restricted" {
			args = append(args, "--permission-mode", "acceptEdits")
		}
		return "claude", args, nil
	case "codex":
		args := []string{"exec", "--json", "--cd", request.Project, "--sandbox", "workspace-write"}
		if request.Profile == "restricted" {
			args = []string{"exec", "--json", "--cd", request.Project, "--sandbox", "workspace-write", "--ask-for-approval", "on-request"}
		}
		if request.Profile == "autonomous" {
			args = []string{"exec", "--json", "--cd", request.Project, "--sandbox", "danger-full-access", "--ask-for-approval", "never"}
		}
		args = append(args, request.Prompt)
		return "codex", args, nil
	case "opencode":
		return "", nil, errors.New("adapter: OpenCode non-interactive event mode is not supported by this build")
	default:
		return "", nil, fmt.Errorf("adapter: unsupported CLI %q", request.Agent)
	}
}

func allowedEnvironment(extra map[string]string) []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "USERPROFILE": true, "SystemRoot": true, "WINDIR": true, "PATHEXT": true, "TEMP": true, "TMP": true, "TMPDIR": true, "LOCALAPPDATA": true, "APPDATA": true, "CODEX_HOME": true, "ANTHROPIC_API_KEY": false, "OPENAI_API_KEY": false}
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && allowed[key] {
			values[key] = value
		}
	}
	for key, value := range extra {
		if allowed[key] && allowed[key] == true {
			values[key] = value
		}
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func outputLimit(limit int) int {
	if limit <= 0 {
		return defaultOutputLimit
	}
	return limit
}
