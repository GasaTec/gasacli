//go:build windows

package tui

import "testing"

func TestWindowsWorkingSetIsAvailable(t *testing.T) {
	value, ok := processRSSBytes()
	if !ok || value == 0 {
		t.Fatalf("working set unavailable: bytes=%d available=%v", value, ok)
	}
}
