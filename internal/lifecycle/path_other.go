//go:build !windows

package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func canonicalExistingPath(path string) (string, error) {
	cur := string(filepath.Separator)
	for _, part := range splitPath(path) {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.Clean(path), nil
			}
			return "", fmt.Errorf("inspect path component %s: %w", cur, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing data directory: path component %s is a symbolic link", cur)
		}
	}
	return filepath.Clean(path), nil
}
func splitPath(path string) []string {
	return strings.Split(filepath.Clean(path), string(filepath.Separator))
}
func isReparse(info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }
func windowsProtectedPaths() []string { return nil }

// validateWindowsSyntax is only meaningful on Windows (guard.go calls it behind a GOOS check).
func validateWindowsSyntax(string) error { return nil }
