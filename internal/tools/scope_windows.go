//go:build windows

package tools

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// canonicalPath resolves every reparse point (symlink or junction) in an
// existing path. filepath.EvalSymlinks does not resolve directory junctions
// on Windows, so use GetFinalPathNameByHandle, which does.
func canonicalPath(p string) (string, error) {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			out := windows.UTF16ToString(buf[:n])
			if strings.HasPrefix(out, `\\?\UNC\`) {
				out = `\\` + out[len(`\\?\UNC\`):]
			} else {
				out = strings.TrimPrefix(out, `\\?\`)
			}
			return filepath.Clean(out), nil
		}
		buf = make([]uint16, len(buf)*2)
	}
}
