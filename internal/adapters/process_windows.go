//go:build windows

package adapters

import (
	"os/exec"
	"strconv"
	"syscall"
)

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return killWindowsTree(cmd.Process.Pid)
	}
}
func terminateProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return killWindowsTree(cmd.Process.Pid)
}
func killWindowsTree(pid int) error {
	err := exec.Command("taskkill.exe", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	if err != nil {
		if fallbackErr := exec.Command("taskkill.exe", "/PID", strconv.Itoa(pid), "/F").Run(); fallbackErr != nil {
			return err
		}
	}
	return nil
}
