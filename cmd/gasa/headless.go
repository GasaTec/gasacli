package main

import (
	"os"
	"strings"

	"golang.org/x/term"
)

func headlessMode() bool {
	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	return isHeadless(tty, os.Getenv)
}

func isHeadless(tty bool, getenv func(string) string) bool {
	if !tty {
		return true
	}
	ci := strings.ToLower(strings.TrimSpace(getenv("CI")))
	if ci == "true" || ci == "1" || ci == "yes" {
		return true
	}
	return getenv("SSH_TTY") != "" && getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == ""
}
