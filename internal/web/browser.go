package web

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenBrowser launches the OS URL handler without a shell. A nil launch uses
// exec.Command.Start; tests and callers can inject a launcher instead.
func OpenBrowser(url string, launch func(name string, args ...string) error) error {
	if launch == nil {
		launch = func(name string, args ...string) error {
			cmd := exec.Command(name, args...)
			if err := cmd.Start(); err != nil {
				return err
			}
			go func() { _ = cmd.Wait() }()
			return nil
		}
	}
	name, args := browserCommand(runtime.GOOS, url)
	if err := launch(name, args...); err != nil {
		return fmt.Errorf("open dashboard browser: %w", err)
	}
	return nil
}

func browserCommand(platform, url string) (string, []string) {
	switch platform {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}
