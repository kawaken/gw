// Package worktree provides helpers for Git worktree path calculation, label generation,
// name resolution, sorting, and display formatting.
package worktree

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// mainLabel is the label and task name used to identify the main worktree.
const mainLabel = "main"

// ShortenName normalizes a task name for use as a directory name.
// Names exceeding 20 chars are truncated to 12 chars + "__" + 6-char SHA-256 fragment
// derived from the original name, avoiding collisions among names sharing a long prefix.
func ShortenName(name string) string {
	s := strings.ReplaceAll(name, "/", "-")
	if len(s) <= 20 {
		return s
	}
	h := sha256.Sum256([]byte(name))
	fragment := hex.EncodeToString(h[:])[:6]
	return s[:12] + "__" + fragment // 12 + 2 + 6 = 20
}

// BaseDir returns the worktree base directory: {parent}/{repo}-wt
// unless overridden by configDir.
func BaseDir(mainRepoPath, configDir string) string {
	if configDir != "" {
		if filepath.IsAbs(configDir) {
			return configDir
		}
		return filepath.Join(mainRepoPath, configDir)
	}
	parent := filepath.Dir(mainRepoPath)
	repo := filepath.Base(mainRepoPath)
	return filepath.Join(parent, repo+"-wt")
}

// Path returns the full path for a task worktree.
// If taskName is empty or "main", returns mainRepoPath.
func Path(mainRepoPath, configDir, taskName string) string {
	if taskName == "" || taskName == mainLabel {
		return mainRepoPath
	}
	short := ShortenName(taskName)
	return filepath.Join(BaseDir(mainRepoPath, configDir), short)
}

// MakeLabel returns a short label for a worktree path.
// If it IS the main repo, the label is "main". Otherwise the label is the basename.
func MakeLabel(wtPath, mainRepoPath string) string {
	if wtPath == mainRepoPath {
		return mainLabel
	}
	return filepath.Base(wtPath)
}
