package onboarding

import (
	"context"
	"os/exec"
	"time"
)

func runCommandTree(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	kill, cleanup, err := attachProcessTree(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	defer cleanup()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	case <-ctx.Done():
		_ = kill()
		select {
		case <-done:
		case <-time.After(cmd.WaitDelay):
		}
		return ctx.Err()
	}
}
