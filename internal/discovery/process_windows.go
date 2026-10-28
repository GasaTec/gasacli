//go:build windows

package discovery

import (
	"os/exec"
	"strconv"
)

func configureProcessTree(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Windows .cmd launchers run through cmd.exe; taskkill /T terminates
		// the launcher and its descendants when the probe deadline expires.
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}
