package engine

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultGateTimeout = 5 * time.Minute
	defaultGateOutput  = int64(2 << 20)
)

// GateCommand is a project-configured argv command. It is always executed
// directly without a shell.
type GateCommand struct {
	Name    string
	Argv    []string
	Timeout time.Duration
}

// GateResult records safe, bounded metadata about one quality gate. Output and
// arguments are intentionally omitted because either can contain secrets.
type GateResult struct {
	Name        string        `json:"name"`
	Passed      bool          `json:"passed"`
	ExitCode    int           `json:"exit_code"`
	OutputBytes int64         `json:"output_bytes"`
	Duration    time.Duration `json:"duration"`
	Reason      string        `json:"reason,omitempty"`
}

// RunGates executes configured commands in the project directory and appends
// one durable gate.finished event per command. At least one gate is required.
func (e *Engine) RunGates(ctx context.Context, runID, project string, gates []GateCommand) ([]GateResult, error) {
	if project == "" {
		return nil, errors.New("engine: gate project is required")
	}
	if len(gates) == 0 {
		return nil, errors.New("engine: no quality gates are configured")
	}
	run, err := e.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.Status != Gates {
		return nil, fmt.Errorf("engine: quality gates require GATES state, got %s", run.Status)
	}
	results := make([]GateResult, 0, len(gates))
	for _, gate := range gates {
		result, gateErr := runGate(ctx, project, gate)
		results = append(results, result)
		if gateErr != nil {
			return results, gateErr
		}
	}
	return results, nil
}

func runGate(parent context.Context, project string, gate GateCommand) (GateResult, error) {
	result := GateResult{Name: gate.Name, ExitCode: -1}
	if gate.Name == "" || len(gate.Argv) == 0 || gate.Argv[0] == "" {
		result.Reason = "invalid gate command"
		return result, errors.New("engine: invalid gate command")
	}
	for _, arg := range gate.Argv {
		if arg == "" || strings.IndexByte(arg, 0) >= 0 {
			result.Reason = "invalid gate argument"
			return result, fmt.Errorf("engine: gate %s has an invalid argument", gate.Name)
		}
	}
	timeout := gate.Timeout
	if timeout <= 0 {
		timeout = defaultGateTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gate.Argv[0], gate.Argv[1:]...)
	cmd.Dir = project
	var outputBytes atomic.Int64
	var overflow atomic.Bool
	capture := func(p []byte) (int, error) {
		count := outputBytes.Add(int64(len(p)))
		if count > defaultGateOutput {
			overflow.Store(true)
			cancel()
		}
		return len(p), nil
	}
	cmd.Stdout = writerFunc(capture)
	cmd.Stderr = writerFunc(capture)
	started := time.Now()
	err := cmd.Run()
	result.Duration = time.Since(started)
	result.OutputBytes = outputBytes.Load()
	if err == nil && !overflow.Load() {
		result.Passed = true
		result.ExitCode = 0
		return result, nil
	}
	if overflow.Load() {
		result.Reason = "output limit exceeded"
		return result, fmt.Errorf("engine: gate %s exceeded the output limit", gate.Name)
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Reason = "timed out"
			return result, fmt.Errorf("engine: gate %s timed out", gate.Name)
		}
		result.Reason = "cancelled"
		return result, fmt.Errorf("engine: gate %s cancelled", gate.Name)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.Reason = "could not start"
	}
	return result, fmt.Errorf("engine: gate %s failed with exit code %d", gate.Name, result.ExitCode)
}

type writerFunc func([]byte) (int, error)

func (write writerFunc) Write(value []byte) (int, error) { return write(value) }
