//go:build !windows

package discovery

import (
	"context"
	"os/exec"
	"syscall"
)

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func runProcess(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	waitDone := make(chan struct{})
	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-waitDone:
		}
	}()
	err := cmd.Wait()
	close(waitDone)
	<-cancelDone
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
