//go:build !linux && !windows

package tui

// No current resident-set measurement is available on this platform yet.
func processRSSBytes() (uint64, bool) { return 0, false }
