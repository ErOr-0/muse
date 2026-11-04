package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scope confines all file access to a workspace root.
type Scope struct{ Root string }

func NewScope(root string) (*Scope, error) {
	if root == "" {
		return nil, fmt.Errorf("no workspace selected")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	// A missing workspace must fail here with a readable message, not later
	// as a cryptic child-process error (fork/exec: directory name is invalid).
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace folder does not exist: %s — choose the workspace folder again", abs)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("workspace is not a folder: %s", abs)
	}
	if canon, err := canonicalPath(abs); err == nil {
		abs = canon
	}
	return &Scope{Root: abs}, nil
}

// Resolve rejects absolute paths and ../ escapes, then canonicalizes the
// longest existing prefix (resolving symlinks and, on Windows, junctions
// and other reparse points) and re-checks containment. A path whose
// existing prefix cannot be canonicalized is rejected, not trusted.
func (s *Scope) Resolve(rel string) (string, error) {
	if rel == "" {
		rel = "."
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
		return "", fmt.Errorf("path must be relative to workspace: %q", rel)
	}
	clean := filepath.Clean(rel)
	joined := filepath.Join(s.Root, clean)
	// Canonicalize the longest existing prefix; the remainder does not
	// exist yet, so it cannot hide a reparse point.
	probe := joined
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	canon, err := canonicalPath(probe)
	if err != nil {
		return "", fmt.Errorf("path cannot be verified inside workspace: %q", rel)
	}
	if rest, rerr := filepath.Rel(probe, joined); rerr == nil {
		joined = filepath.Join(canon, rest)
	}
	relToRoot, err := filepath.Rel(s.Root, joined)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %q", rel)
	}
	return joined, nil
}

func (s *Scope) Rel(abs string) string {
	r, err := filepath.Rel(s.Root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(r)
}
