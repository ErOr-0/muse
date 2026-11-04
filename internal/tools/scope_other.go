//go:build !windows

package tools

import "path/filepath"

// canonicalPath resolves every symlink in an existing path.
func canonicalPath(p string) (string, error) {
	return filepath.EvalSymlinks(p)
}
