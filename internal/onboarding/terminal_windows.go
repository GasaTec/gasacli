//go:build windows

package onboarding

import "golang.org/x/sys/windows"

func enableVT(fd uintptr) (func(), error) {
	h := windows.Handle(fd)
	var old uint32
	if err := windows.GetConsoleMode(h, &old); err != nil {
		return func() {}, err
	}
	if err := windows.SetConsoleMode(h, old|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return func() {}, err
	}
	return func() { _ = windows.SetConsoleMode(h, old) }, nil
}
