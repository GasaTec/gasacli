//go:build windows

package discovery

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureProcessTree(*exec.Cmd) {}

func runProcess(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		killWindowsTree(cmd.Process.Pid)
		_ = cmd.Wait()
		return errors.New("process tree setup failed")
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if err == nil {
		var process windows.Handle
		process, err = windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
		if err == nil {
			err = windows.AssignProcessToJobObject(job, process)
			_ = windows.CloseHandle(process)
		}
	}
	if err != nil {
		_ = windows.CloseHandle(job)
		killWindowsTree(cmd.Process.Pid)
		_ = cmd.Wait()
		return errors.New("process tree setup failed")
	}
	waitDone := make(chan struct{})
	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		select {
		case <-ctx.Done():
			_ = windows.TerminateJobObject(job, 1)
		case <-waitDone:
		}
	}()
	err = cmd.Wait()
	close(waitDone)
	<-cancelDone
	_ = windows.CloseHandle(job)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func killWindowsTree(pid int) {
	// Windows .cmd launchers run through cmd.exe; taskkill /T terminates the
	// launcher and its descendants when Job Object assignment is unavailable.
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
