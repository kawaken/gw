package worktree

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kawaken/gw/git"
)

// Resolve finds the worktree path for a query string.
// Supported queries:
//   - "" or "main" → main repo path
//   - basename match (e.g. "task-123")
//   - PR number formats: "PR-123", "#123", "123" (maps to "PR-123" in {repo}-wt/)
func Resolve(g git.Runner, query string) (string, error) {
	mainPath, err := git.MainWorktreePath(g)
	if err != nil {
		return "", err
	}

	if query == "" || query == "main" {
		return mainPath, nil
	}

	entries, err := git.ListWorktrees(g)
	if err != nil {
		return "", err
	}

	repoName := filepath.Base(mainPath)
	wtDir := repoName + "-wt"

	// Normalize PR query: PR-123, #123, or bare number
	prNum := extractPRNumber(query)

	for _, e := range entries {
		dirName := filepath.Base(e.Path)
		parentName := filepath.Base(filepath.Dir(e.Path))

		// Exact basename match
		if dirName == query {
			return e.Path, nil
		}
		// PR number match inside {repo}-wt/
		if parentName == wtDir && prNum != "" && dirName == "PR-"+prNum {
			return e.Path, nil
		}
	}
	return "", fmt.Errorf("worktree %q not found", query)
}

// extractPRNumber returns the numeric part if the query looks like PR-N, #N, or N.
// Returns empty string otherwise.
func extractPRNumber(query string) string {
	q := query
	if strings.HasPrefix(q, "PR-") {
		q = q[3:]
	} else if strings.HasPrefix(q, "#") {
		q = q[1:]
	}
	for _, ch := range q {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	if q == "" {
		return ""
	}
	return q
}
