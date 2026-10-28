//go:build !windows

package trust

import "path/filepath"

func resolvePath(path string) (string, error) { return filepath.EvalSymlinks(path) }
