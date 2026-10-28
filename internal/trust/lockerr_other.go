//go:build !windows

package trust

// isLockContention is only needed on Windows, where a file pending deletion is not reported as
// "already exists".
func isLockContention(error) bool { return false }
