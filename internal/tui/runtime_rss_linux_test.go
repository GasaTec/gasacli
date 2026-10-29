//go:build linux

package tui

import "testing"

func TestParseLinuxRSS(t *testing.T) {
	cases := []struct {
		input string
		bytes uint64
		ok    bool
	}{
		{"Name:\tgasa\nVmRSS:\t123 kB\n", 123 * 1024, true},
		{"VmRSS: 0 kB\n", 0, true},
		{"VmRSS: 12 MB\n", 0, false},
		{"VmRSS: 18446744073709551615 kB\n", 0, false},
		{"Name: gasa\n", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseLinuxRSS(tc.input)
		if got != tc.bytes || ok != tc.ok {
			t.Errorf("parseLinuxRSS(%q) = %d, %v; want %d, %v", tc.input, got, ok, tc.bytes, tc.ok)
		}
	}
}

func TestProcessRSSAvailable(t *testing.T) {
	if bytes, ok := processRSSBytes(); !ok || bytes == 0 {
		t.Fatalf("RSS = %d, %v", bytes, ok)
	}
}
