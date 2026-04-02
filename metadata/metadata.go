// Package metadata reads and writes gw_metadata files stored in .git/worktrees/{name}/.
package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// metadataPath returns the path to gw_metadata for a given worktree.
func metadataPath(wtPath string) (string, error) {
	adminDir, err := worktreeAdminDir(wtPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(adminDir, "gw_metadata"), nil
}

func worktreeAdminDir(wtPath string) (string, error) {
	gitPath := filepath.Join(wtPath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", gitPath, err)
	}
	if info.IsDir() {
		return gitPath, nil
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", gitPath, err)
	}
	line := strings.TrimSpace(string(data))
	gitDir, ok := strings.CutPrefix(line, "gitdir: ")
	if !ok {
		return "", fmt.Errorf("invalid gitdir file: %s", gitPath)
	}
	if filepath.IsAbs(gitDir) {
		return filepath.Clean(gitDir), nil
	}
	return filepath.Clean(filepath.Join(wtPath, gitDir)), nil
}

// Get reads a key from the metadata file.
// Returns empty string if file or key doesn't exist.
func Get(wtPath, key string) (string, error) {
	path, err := metadataPath(wtPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read metadata %s: %w", path, err)
	}
	prefix := key + "="
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return line[len(prefix):], nil
		}
	}
	return "", nil
}

// Set writes (or replaces) a key=value in the metadata file.
func Set(wtPath, key, value string) error {
	if strings.ContainsAny(key, "=\n\r") {
		return fmt.Errorf("metadata key must not contain '=', '\\n', or '\\r': %q", key)
	}
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("metadata value must not contain '\\n' or '\\r': %q", value)
	}

	path, err := metadataPath(wtPath)
	if err != nil {
		return err
	}

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
func GetAll(wtPath string) (map[string]string, error) {
	path, err := metadataPath(wtPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read metadata %s: %w", path, err)
	}
	m := map[string]string{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			m[k] = v
		}
	}
	return m, nil
}
