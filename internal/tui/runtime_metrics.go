package tui

import "runtime"

// goHeapBytes reports currently allocated Go heap objects, not process RSS.
func goHeapBytes() uint64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}
