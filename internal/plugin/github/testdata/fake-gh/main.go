package main

import (
	"os"
	"strings"
)

// This executable is used by package tests only; it never connects to GitHub.
func main() {
	if path := os.Getenv("GASA_TEST_GH_ARGS"); path != "" {
		_ = os.WriteFile(path, []byte(strings.Join(os.Args[1:], "\n")), 0600)
	}
	_, _ = os.Stdout.WriteString(os.Getenv("GASA_TEST_GH_OUTPUT"))
}
