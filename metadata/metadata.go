package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// metadataPath returns the path to gw_metadata for a given worktree.
// mainRepoPath is the main repo (where .git lives).
// wtPath is the worktree path.
func metadataPath(mainRepoPath, wtPath string) string {
	name := filepath.Base(wtPath)
	return filepath.Join(mainRepoPath, ".git", "worktrees", name, "gw_metadata")
}

// Get reads a key from the metadata file.
// Returns empty string if file or key doesn't exist.
func Get(mainRepoPath, wtPath, key string) string {
	path := metadataPath(mainRepoPath, wtPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return line[len(prefix):]
		}
	}
	return ""
}

// Set writes (or replaces) a key=value in the metadata file.
func Set(mainRepoPath, wtPath, key, value string) error {
	path := metadataPath(mainRepoPath, wtPath)

	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" && !strings.HasPrefix(line, key+"=") {
				lines = append(lines, line)
			}
		}
	}
	lines = append(lines, fmt.Sprintf("%s=%s", key, value))

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// GetAll returns all key=value pairs from the metadata file as a map.
func GetAll(mainRepoPath, wtPath string) map[string]string {
	path := metadataPath(mainRepoPath, wtPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if idx := strings.IndexByte(line, '='); idx > 0 {
			m[line[:idx]] = line[idx+1:]
		}
	}
	return m
}
