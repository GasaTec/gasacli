//go:build windows

package onboarding

import (
	"golang.org/x/sys/windows"
	"os/exec"
)

func configureProcessTree(*exec.Cmd) {}
func attachProcessTree(cmd *exec.Cmd) (func() error, func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, func() {}, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_ALL_ACCESS, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, func() {}, err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(job)
		return nil, func() {}, err
	}
	return func() error { return windows.TerminateJobObject(job, 1) }, func() { _ = windows.CloseHandle(job) }, nil
}

func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	state, err := windows.WaitForSingleObject(h, 0)
	return err == nil && state == uint32(windows.WAIT_TIMEOUT)
}
