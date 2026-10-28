//go:build !windows

package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func canonicalExistingPath(path string) (string, error) {
	parts := splitPath(path)
	cur := string(filepath.Separator)
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		next := filepath.Join(cur, part)
		info, err := os.Lstat(next)
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.Join(append([]string{cur}, parts[i:]...)...), nil
			}
			return "", fmt.Errorf("inspect path component %s: %w", next, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, ok := systemAliasTarget(next)
			if !ok {
				return "", fmt.Errorf("refusing data directory: path component %s is a symbolic link", next)
			}
			next = target
		}
		cur = next
	}
	return filepath.Clean(cur), nil
}

// systemAliasTarget allows the few root-level links the operating system itself ships
// (/var -> /private/var on macOS, /bin -> usr/bin on merged-/usr Linux) and resolves them.
// Any other link is still refused.
func systemAliasTarget(link string) (string, bool) {
	if filepath.Dir(link) != string(filepath.Separator) {
		return "", false
	}
	var aliases map[string]string
	switch runtime.GOOS {
	case "darwin":
		aliases = map[string]string{"/var": "/private/var", "/tmp": "/private/tmp", "/etc": "/private/etc"}
	case "linux":
		aliases = map[string]string{"/bin": "/usr/bin", "/sbin": "/usr/sbin", "/lib": "/usr/lib", "/lib32": "/usr/lib32", "/lib64": "/usr/lib64", "/libx32": "/usr/libx32"}
	default:
		return "", false
	}
	want, ok := aliases[link]
	if !ok {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil || !samePath(resolved, want) {
		return "", false
	}
	return filepath.Clean(resolved), true
}
func splitPath(path string) []string {
	return strings.Split(filepath.Clean(path), string(filepath.Separator))
}
func isReparse(info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }
func windowsProtectedPaths() []string { return nil }

// validateWindowsSyntax is only meaningful on Windows (guard.go calls it behind a GOOS check).
func validateWindowsSyntax(string) error { return nil }
