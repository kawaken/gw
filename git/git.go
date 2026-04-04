// Package git provides a thin wrapper around the git CLI.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner is the interface for running git commands.
type Runner interface {
	Run(args ...string) (string, error)
	RunIn(dir string, args ...string) (string, error)
	// Toplevel returns the absolute path of the top-level directory of the current git repo.
	Toplevel() (string, error)
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

// Toplevel returns the absolute path of the top-level directory of the current git repo.
func (c *CLI) Toplevel() (string, error) {
	return c.Run("rev-parse", "--show-toplevel")
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// New returns a CLI Runner.
func New() Runner {
	return &CLI{}
}

// WorktreeEntry is a parsed line from `git worktree list --porcelain`.
type WorktreeEntry struct {
	Path   string
	HEAD   string
	Branch string // e.g. "[main]", "[feature/foo]", "(detached HEAD)", "(bare)"
}

// ListWorktrees runs `git worktree list --porcelain` and returns parsed entries.
func ListWorktrees(g Runner) ([]WorktreeEntry, error) {
	out, err := g.Run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var entries []WorktreeEntry
	var current WorktreeEntry

	flush := func() {
		if current.Path == "" {
			return
		}
		entries = append(entries, current)
		current = WorktreeEntry{}
	}

	for line := range strings.SplitSeq(out, "\n") {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			flush()
			continue
		}

		key, value, ok := strings.Cut(trimmedLine, " ")
		if !ok {
			switch key {
			case "detached":
				current.Branch = "(detached HEAD)"
			case "bare":
				current.Branch = "(bare)"
			}
			continue
		}

		switch key {
		case "worktree":
			current.Path = value
		case "HEAD":
			current.HEAD = value
		case "branch":
			current.Branch = "[" + shortRef(value) + "]"
		}
	}
	flush()
	return entries, nil
}

func shortRef(ref string) string {
	parts := strings.SplitN(ref, "/", 3)
	length := len(parts)

	if length == 3 && parts[0] == "refs" && parts[1] == "heads" {
		return parts[2]
	}
	if parts[0] == "refs" {
		return strings.Join(parts[1:], "/")
	}
	return ref
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
// Falls back to "main" then "master" if origin/HEAD is not determinable.
func DefaultBranch(g Runner) (string, error) {
	out, err := g.Run("symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(out)
		// refs/remotes/origin/feature/auth → feature/auth
		branch, ok := strings.CutPrefix(ref, "refs/remotes/origin/")
		if !ok || branch == "" {
			return "", fmt.Errorf("unexpected origin/HEAD ref: %q", ref)
		}
		return branch, nil
	}
	// fallback: check if main exists
	if _, err := g.Run("show-ref", "--verify", "--quiet", "refs/heads/main"); err == nil {
		return "main", nil
	}
	return "master", nil
}
