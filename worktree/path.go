// Package worktree provides helpers for Git worktree path calculation, label generation,
// name resolution, sorting, and display formatting.
package worktree

import (
	"path/filepath"
	"strings"
)

// ShortenName replaces "/" with "-" and truncates to 20 chars (appending "__").
func ShortenName(name string) string {
	s := strings.ReplaceAll(name, "/", "-")
	if len(s) > 20 {
		s = s[:18] + "__"
	}
	return s
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
	if taskName == "" || taskName == "main" {
		return mainRepoPath
	}
	short := ShortenName(taskName)
	return filepath.Join(BaseDir(mainRepoPath, configDir), short)
}

// MakeLabel returns a short label for a worktree path.
// If the worktree lives inside {repo}-wt/, the label is its basename.
// If it IS the main repo, the label is "main".
func MakeLabel(wtPath, mainRepoPath string) string {
	repoName := filepath.Base(mainRepoPath)
	dirName := filepath.Base(wtPath)
	parentName := filepath.Base(filepath.Dir(wtPath))

	if wtPath == mainRepoPath {
		return "main"
	}
	if parentName == repoName+"-wt" {
		return dirName
	}
	return dirName
}
