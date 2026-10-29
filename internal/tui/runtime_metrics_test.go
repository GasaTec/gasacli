package tui

import "testing"

func TestGoHeapBytesReportsAllocation(t *testing.T) {
	if got := goHeapBytes(); got == 0 {
		t.Fatal("expected allocated Go heap bytes")
	}
}
