// Package git provides a thin wrapper around the git CLI.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner is the interface for running git commands.
type Runner interface {
	Run(args ...string) (string, error)
	RunIn(dir string, args ...string) (string, error)
}

// CLI implements Runner using the git CLI.
type CLI struct{}

// Run executes a git command in the current directory.
func (c *CLI) Run(args ...string) (string, error) {
	return runGit("", args...)
}

// RunIn executes a git command in the specified directory.
func (c *CLI) RunIn(dir string, args ...string) (string, error) {
	return runGit(dir, args...)
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // always calling "git"
	if dir != "" {
		cmd.Dir = dir
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// New returns a CLI Runner.
func New() Runner {
	return &CLI{}
}

// WorktreeEntry is a parsed line from `git worktree list`.
type WorktreeEntry struct {
	Path   string
	HEAD   string
	Branch string // e.g. "[main]" or "(HEAD detached at abc1234)"
}

// ListWorktrees runs `git worktree list` and returns parsed entries.
func ListWorktrees(g Runner) ([]WorktreeEntry, error) {
	out, err := g.Run("worktree", "list")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	var entries []WorktreeEntry
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		entries = append(entries, WorktreeEntry{
			Path:   fields[0],
			HEAD:   fields[1],
			Branch: fields[2],
		})
	}
	return entries, nil
}

// MainWorktreePath returns the path of the main (first) worktree.
func MainWorktreePath(g Runner) (string, error) {
	entries, err := ListWorktrees(g)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", errors.New("no worktrees found")
	}
	return entries[0].Path, nil
}

// DefaultBranch returns the default branch name from origin/HEAD.
// Falls back to "main" then "master" if not determinable.
func DefaultBranch(g Runner) string {
	out, err := g.Run("symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		// refs/remotes/origin/main → main
		parts := strings.Split(strings.TrimSpace(out), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	// fallback: check if main exists
	if _, err := g.Run("show-ref", "--verify", "--quiet", "refs/heads/main"); err == nil {
		return "main"
	}
	return "master"
}
