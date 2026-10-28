//go:build windows

package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
	"unsafe"
)

const fileAttributeReparsePoint = 0x400

func validateWindowsSyntax(path string) error {
	lower := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	if strings.HasPrefix(lower, `\\?\`) || strings.HasPrefix(lower, `\\.\`) || strings.HasPrefix(lower, `\??\`) {
		return fmt.Errorf("refusing data directory: extended-length and device paths are not allowed")
	}
	if strings.HasPrefix(lower, `\`) {
		return fmt.Errorf("refusing data directory: UNC paths are not allowed")
	}
	if len(path) < 3 || !((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) || path[1] != ':' || (path[2] != '\\' && path[2] != '/') {
		return fmt.Errorf("refusing data directory: path must be absolute and cannot be drive-relative (for example C:foo)")
	}
	for i := 2; i < len(path); i++ {
		if path[i] == ':' {
			return fmt.Errorf("refusing data directory: colon outside drive letter")
		}
	}
	for _, component := range strings.Split(strings.ReplaceAll(path[3:], "/", `\`), `\`) {
		if component == "" {
			continue
		}
		base := strings.ToUpper(strings.SplitN(strings.TrimRight(component, " ."), ".", 2)[0])
		reserved := base == "CON" || base == "PRN" || base == "AUX" || base == "NUL"
		if (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') || base == "COM¹" || base == "COM²" || base == "COM³" || base == "LPT¹" || base == "LPT²" || base == "LPT³" {
			reserved = true
		}
		if reserved || strings.IndexFunc(component, func(r rune) bool { return unicode.IsControl(r) }) >= 0 {
			return fmt.Errorf("refusing data directory: reserved Windows path component %q", component)
		}
	}
	return nil
}

func canonicalExistingPath(path string) (string, error) {
	// Resolve 8.3 aliases using GetLongPathNameW when the path exists.
	if _, err := os.Stat(path); err == nil {
		in, err := syscall.UTF16PtrFromString(path)
		if err == nil {
			proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetLongPathNameW")
			buf := make([]uint16, 32768)
			n, _, _ := proc.Call(uintptr(unsafe.Pointer(in)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			if n > 0 && n < uintptr(len(buf)) {
				path = syscall.UTF16ToString(buf[:n])
			}
		}
	}
	cur := filepath.VolumeName(path) + string(filepath.Separator)
	rest := strings.TrimLeft(path[len(filepath.VolumeName(path)):], `\/`)
	for _, part := range strings.Split(rest, string(filepath.Separator)) {
		if part == "" {
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
		if isReparse(info) {
			return "", fmt.Errorf("refusing data directory: path component %s is a reparse point or link", cur)
		}
	}
	return filepath.Clean(path), nil
}
func windowsProtectedPaths() []string {
	var values []string
	for _, name := range []string{"USERPROFILE", "LOCALAPPDATA", "APPDATA", "ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "ProgramData", "SystemRoot", "windir", "TEMP", "TMP"} {
		if value := os.Getenv(name); value != "" {
			values = append(values, value)
		}
	}
	if p := os.Getenv("ProgramFiles"); p != "" {
		values = append(values, filepath.Join(p, "WindowsApps"))
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		values = append(values, filepath.Join(root, "System32"))
	}
	return values
}
func isReparse(info os.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && data.FileAttributes&fileAttributeReparsePoint != 0
}
