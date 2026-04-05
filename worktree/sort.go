package worktree

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kawaken/gw/git"
)

// Entry holds parsed info about a worktree with commit timestamp.
type Entry struct {
	Path      string
	Branch    string // raw branch field from git worktree list
	Age       string // human-readable age (e.g. "2 days")
	Timestamp int64  // unix timestamp for sorting
}

// Sorted returns all worktrees sorted by most recent commit (newest first).
func Sorted(g git.Runner) ([]Entry, error) {
	wts, err := git.ListWorktrees(g)
	if err != nil {
		return nil, fmt.Errorf("sorted worktrees: %w", err)
	}

	entries := make([]Entry, 0, len(wts))
	for _, wt := range wts {
		out, err := g.RunIn(wt.Path, "log", "-1", "--format=%ct %cr")
		if err != nil {
			// Empty or otherwise unreadable worktrees sort last, but still appear in output.
			entries = append(entries, Entry{
				Path:   wt.Path,
				Branch: wt.Branch,
			})
			continue
		}
		ts, age := parseLogOutput(out)
		entries = append(entries, Entry{
			Path:      wt.Path,
			Branch:    wt.Branch,
			Age:       age,
			Timestamp: ts,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})
	return entries, nil
}

// parseLogOutput parses the output of `git log -1 --format="%ct %cr"`.
func parseLogOutput(out string) (int64, string) {
	out = strings.TrimSpace(out)
	if tsStr, age, ok := strings.Cut(out, " "); ok {
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			return 0, strings.TrimSuffix(age, " ago")
		}
		return ts, strings.TrimSuffix(age, " ago")
	}
	return 0, ""
}
