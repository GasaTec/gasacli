//go:build windows

package trust

import (
	"errors"
	"syscall"
)

// isLockContention reports Windows errors that mean "another process is creating or deleting the
// lock right now": a file pending deletion makes CreateFile fail with access denied or a sharing
// violation instead of "already exists".
func isLockContention(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	const (
		errorAccessDenied     = syscall.Errno(5)
		errorSharingViolation = syscall.Errno(32)
		errorLockViolation    = syscall.Errno(33)
	)
	return errno == errorAccessDenied || errno == errorSharingViolation || errno == errorLockViolation
}
