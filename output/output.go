package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// Result is the common JSON output structure for all gw subcommands.
type Result struct {
	Messages []string `json:"messages,omitempty"`
	CD       string   `json:"cd,omitempty"`
	// list-specific fields
	Worktrees []WorktreeInfo `json:"worktrees,omitempty"`
}

// WorktreeInfo holds structured info for a single worktree (used by list).
type WorktreeInfo struct {
	Label    string `json:"label"`
	Branch   string `json:"branch,omitempty"`
	Age      string `json:"age,omitempty"`
	Path     string `json:"path,omitempty"`
	Archived bool   `json:"archived,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
}

// Print writes the Result as JSON to stdout.
func Print(r Result) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		fmt.Fprintf(os.Stderr, "output error: %v\n", err)
	}
}

// Error writes an error result as JSON to stdout and returns exit code 1.
func Error(msg string) {
	Print(Result{Messages: []string{msg}})
}

// Errorf formats and writes an error result.
func Errorf(format string, args ...any) {
	Error(fmt.Sprintf(format, args...))
}
