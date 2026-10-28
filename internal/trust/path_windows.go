package trust

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var finalPathName = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")

// Opening without OPEN_REPARSE_POINT follows both symlinks and NTFS junctions,
// including junctions in ancestor directories. EvalSymlinks can leave junctions
// unresolved on Windows because they are not always reported as ModeSymlink.
func resolvePath(path string) (string, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := syscall.CreateFile(name, 0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer syscall.CloseHandle(handle)
	buffer := make([]uint16, 512)
	for {
		n, _, err := finalPathName.Call(uintptr(handle), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0)
		if n == 0 {
			return "", err
		}
		if n >= uintptr(len(buffer)) {
			buffer = make([]uint16, int(n)+1)
			continue
		}
		resolved := syscall.UTF16ToString(buffer[:n])
		// Preserve the ordinary DOS/UNC representation used in trust.json.
		if strings.HasPrefix(resolved, `\\?\UNC\`) {
			resolved = `\\` + strings.TrimPrefix(resolved, `\\?\UNC\`)
		} else {
			resolved = strings.TrimPrefix(resolved, `\\?\`)
		}
		return filepath.Clean(resolved), nil
	}
}
