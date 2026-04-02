// Package metadata reads and writes gw_metadata files stored in .git/worktrees/{name}/.
package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// metadataPath returns the path to gw_metadata for a given worktree.
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
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return line[len(prefix):]
		}
	}
	return ""
}

// Set writes (or replaces) a key=value in the metadata file.
func Set(mainRepoPath, wtPath, key, value string) error {
	if strings.ContainsAny(key, "=\n\r") {
		return fmt.Errorf("metadata key must not contain '=', '\\n', or '\\r': %q", key)
	}
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("metadata value must not contain '\\n' or '\\r': %q", value)
	}

	path := metadataPath(mainRepoPath, wtPath)

	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			if line != "" && !strings.HasPrefix(line, key+"=") {
				lines = append(lines, line)
			}
		}
	}
	lines = append(lines, fmt.Sprintf("%s=%s", key, value))

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("write metadata %s: %w", path, err)
	}
	return nil
}

// GetAll returns all key=value pairs from the metadata file as a map.
func GetAll(mainRepoPath, wtPath string) map[string]string {
	path := metadataPath(mainRepoPath, wtPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	m := map[string]string{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			m[k] = v
		}
	}
	return m
}
