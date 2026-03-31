package worktree

import (
	"fmt"
	"strings"
)

// FormatMode controls the output format for a worktree line.
type FormatMode int

const (
	ModeNormal  FormatMode = iota
	ModeVerbose            // show full path on right
	ModePath               // show full path instead of branch/purpose
)

// FormatLine builds the display string for a worktree entry.
//   - label: short name (from MakeLabel)
//   - branch: raw branch field (e.g. "[main]" or "[feature/foo]")
//   - age: human-readable age (may be empty)
//   - purpose: from metadata "purpose" key (may be empty)
//   - originalBranch: from metadata "original_branch" key
//   - archived: from metadata "archived" key == "true"
//   - wtPath: full path (used in verbose/path mode)
//   - mode: FormatMode
func FormatLine(label, branch, age, purpose, originalBranch string, archived bool, wtPath string, mode FormatMode) string {
	left := label
	if age != "" {
		left = fmt.Sprintf("%s (%s)", label, age)
	}

	if mode == ModePath {
		return fmt.Sprintf("%s\t%s", left, wtPath)
	}

	// Build right side
	var rightParts []string

	if archived {
		rightParts = append(rightParts, "[Archived]")
	}

	// Show branch if it differs from label / original_branch
	branchName := strings.Trim(branch, "[]")
	showBranch := true
	if label == "main" && branchName == "main" {
		showBranch = false
	} else if originalBranch != "" && branchName == originalBranch {
		showBranch = false
	} else if label == branchName {
		showBranch = false
	}
	if showBranch && branch != "" {
		rightParts = append(rightParts, branch)
	}

	if purpose != "" {
		rightParts = append(rightParts, purpose)
	}

	if mode == ModeVerbose {
		rightParts = append(rightParts, wtPath)
	}

	right := strings.Join(rightParts, " ")
	if right != "" {
		return fmt.Sprintf("%s\t%s", left, right)
	}
	return left
}
