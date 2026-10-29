//go:build linux

package tui

import (
	"math"
	"os"
	"strconv"
	"strings"
)

// processRSSBytes reports resident memory for the GasaCLI process only.
func processRSSBytes() (uint64, bool) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, false
	}
	return parseLinuxRSS(string(data))
}

func parseLinuxRSS(status string) (uint64, bool) {
	for _, line := range strings.Split(status, "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "VmRSS:"))
		if len(fields) != 2 || fields[1] != "kB" {
			return 0, false
		}
		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil || kb > math.MaxUint64/1024 {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}
