package worktree

import (
	"fmt"
	"strings"
)

// FormatMode controls the output format for a worktree line.
type FormatMode int

const (
	// ModeNormal shows label, age, branch (if different), and purpose.
	ModeNormal FormatMode = iota
	// ModeVerbose shows everything in ModeNormal plus the full path.
	ModeVerbose
	// ModePath shows label, age, and full path (omits branch/purpose).
	ModePath
)

// FormatInfo holds the display fields for a single worktree line.
type FormatInfo struct {
	Label          string
	Branch         string // raw branch field from git worktree list (e.g. "[main]")
	Age            string
	Purpose        string
	OriginalBranch string
	Archived       bool
	Path           string
}

// FormatLine builds the display string for a worktree entry.
func FormatLine(info FormatInfo, mode FormatMode) string {
	left := info.Label
	if info.Age != "" {
		left = fmt.Sprintf("%s (%s)", info.Label, info.Age)
	}

	if mode == ModePath {
		return fmt.Sprintf("%s\t%s", left, info.Path)
	}

	var rightParts []string

	if info.Archived {
		rightParts = append(rightParts, "[Archived]")
	}

	branchName := strings.Trim(info.Branch, "[]")
	showBranch := true
	switch {
	case info.Label == "main" && branchName == "main":
		showBranch = false
	case info.OriginalBranch != "" && branchName == info.OriginalBranch:
		showBranch = false
	case info.Label == branchName:
		showBranch = false
	}
	if showBranch && info.Branch != "" {
		rightParts = append(rightParts, info.Branch)
	}

	if info.Purpose != "" {
		rightParts = append(rightParts, info.Purpose)
	}

	if mode == ModeVerbose {
		rightParts = append(rightParts, info.Path)
	}

	right := strings.Join(rightParts, " ")
	if right != "" {
		return fmt.Sprintf("%s\t%s", left, right)
	}
	return left
}
