//go:build !windows

package host

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func configureProcessTree(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}
func attachProcessTree(cmd *exec.Cmd) (func() error, func(), error) {
	return func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return nil
		}
		return err
	}, func() {}, nil
}
func processAlive(pid int) bool {
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		text := string(data)
		fields := strings.Fields(text[strings.LastIndex(text, ")")+1:])
		if len(fields) > 0 && fields[0] == "Z" {
			return false
		}
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func processIdentity(pid int) (string, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", err
	}
	text := string(data)
	end := strings.LastIndex(text, ")")
	if end < 0 {
		return "", errors.New("unrecognized process identity")
	}
	fields := strings.Fields(text[end+1:])
	if len(fields) <= 19 {
		return "", errors.New("unrecognized process identity")
	}
	if fields[0] == "Z" {
		return "", os.ErrNotExist
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	return "linux:" + strings.TrimSpace(string(boot)) + ":" + fields[19], nil
}
func processTreeAbsent(pid int) bool { return syscall.Kill(-pid, 0) == syscall.ESRCH }
