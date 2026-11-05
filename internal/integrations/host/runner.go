package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

type Command struct {
	Executable string
	Args       []string
	Dir        string
	Timeout    time.Duration
	// Started must durably record process identity; failure terminates this tree.
	OnStart func(int) error
}
type Result struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exitCode"`
	Started   bool   `json:"started"`
	TimedOut  bool   `json:"timedOut"`
	Truncated bool   `json:"truncated"`
}
type Runner interface {
	Run(context.Context, Command) (Result, error)
}
type ExecRunner struct{}
type boundedBuffer struct {
	data      []byte
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remain := (256 * 1024) - len(b.data)
	if len(p) > remain {
		p = p[:remain]
		b.truncated = true
	}
	b.data = append(b.data, p...)
	return n, nil
}

// Run never invokes a shell. It strips provider tokens and Orca session identity:
// an HTTP service must not impersonate the agent that happened to start it.
func (ExecRunner) Run(ctx context.Context, c Command) (Result, error) {
	result := Result{ExitCode: -1}
	if c.Timeout <= 0 || c.Timeout > 20*time.Minute {
		return result, errors.New("invalid command timeout")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	cmd := exec.Command(c.Executable, c.Args...)
	cmd.Dir = c.Dir
	cmd.WaitDelay = time.Second
	cmd.Env = []string{}
	for _, key := range []string{"PATH", "HOME", "USERPROFILE", "SystemRoot", "WINDIR", "PATHEXT", "TEMP", "TMP", "TMPDIR", "LOCALAPPDATA", "APPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	var out, errout boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	configureProcessTree(cmd)
	if err := cmd.Start(); err != nil {
		return result, err
	}
	result.Started = true
	kill, cleanup, err := attachProcessTree(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return result, errors.New("process tree containment unavailable")
	}
	defer cleanup()
	if c.OnStart != nil {
		if err := c.OnStart(cmd.Process.Pid); err != nil {
			_ = kill()
			_ = cmd.Wait()
			return result, err
		}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = kill()
		err = <-done
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}
	result.Stdout = string(out.data)
	result.Stderr = string(errout.data)
	result.Truncated = out.truncated || errout.truncated
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	return result, err
}
