//go:build !windows

package onboarding

import (
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
