package main

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type CleanupReport struct {
	SchemaVersion int        `json:"schema_version"`
	Repository    Repository `json:"repository"`
	Mode          string     `json:"mode"`
	Candidates    []Worktree `json:"candidates,omitempty"`
	// Planned lists what an apply run would remove (dry-run only).
	Planned []CleanupEntry `json:"planned,omitempty"`
	Removed []CleanupEntry `json:"removed,omitempty"`
	// Skipped lists worktrees given on the command line that were not removed.
	Skipped []CleanupEntry `json:"skipped,omitempty"`
	Errors  []ResultError  `json:"errors,omitempty"`
}

// CleanupEntry describes one worktree in a cleanup report, including what
// removing it loses. Commit is the branch head, so a deleted branch can be
// restored with git branch <branch> <commit>.
type CleanupEntry struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	// DeleteBranch is whether the branch is (removed: was) deleted together
	// with the worktree.
	DeleteBranch bool     `json:"delete_branch"`
	Forced       bool     `json:"forced,omitempty"`
	Reasons      []string `json:"reasons,omitempty"`
	Uncommitted  int      `json:"uncommitted"`
	Unpushed     *int     `json:"unpushed,omitempty"`
	PRState      string   `json:"pr_state,omitempty"`
}

// forceFlag counts --force: once skips the cleanup judgment, twice also
// overrides running agents and locks, like git worktree remove -f -f.
type forceFlag int

func (f *forceFlag) String() string { return strconv.Itoa(int(*f)) }

func (f *forceFlag) Set(value string) error {
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if enabled {
		*f++
	} else {
		*f = 0
	}
	return nil
}

func (f *forceFlag) IsBoolFlag() bool { return true }

func runClean(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "show what would be removed without removing anything")
	jsonOutput := fs.Bool("json", false, "output JSON")
	saveBranch := fs.Bool("save-branch", false, "keep the local branch of removed worktrees")
	var force forceFlag
	fs.Var(&force, "force", "remove the given worktrees regardless of the cleanup judgment; twice to also override running agents and locks")
	targets, err := parseInterspersed(fs, args)
	if err != nil {
		return 2
	}
	if force > 0 && len(targets) == 0 {
		fmt.Fprintln(stderr, "gw clean: --force requires one or more worktrees")
		return 2
	}

	result, err := collectResult(true)
	if err != nil {
		return printCommandError(stderr, err)
	}
	repoPath := result.Repository.Path
	report := CleanupReport{
		SchemaVersion: schemaVersion,
		Repository:    result.Repository,
		Mode:          "apply",
		Errors:        append([]ResultError{}, result.Errors...),
	}
	if *dryRun {
		report.Mode = "dry-run"
	}

	var plan []CleanupEntry
	byPath := make(map[string]Worktree, len(result.Worktrees))
	for _, wt := range result.Worktrees {
		byPath[wt.Path] = wt
	}
	if len(targets) == 0 {
		for _, wt := range result.Worktrees {
			if wt.Cleanup.Recommendation == "recommended" && wt.Path != repoPath && wt.Path != result.mainPath {
				plan = append(plan, cleanupEntry(wt, wt.Cleanup.Reasons, false, *saveBranch))
				report.Candidates = append(report.Candidates, wt)
			}
		}
	} else {
		records := make([]worktreeRecord, 0, len(result.Worktrees))
		for _, wt := range result.Worktrees {
			records = append(records, worktreeRecord{Path: wt.Path, Branch: wt.Branch})
		}
		seen := make(map[string]bool)
		for _, query := range targets {
			record, err := matchTarget(records, query)
			if err != nil {
				fmt.Fprintf(stderr, "gw clean: %v\n", err)
				report.Errors = append(report.Errors, ResultError{Source: "gw", Code: "worktree_not_found", Message: err.Error()})
				continue
			}
			if seen[record.Path] {
				continue
			}
			seen[record.Path] = true
			wt := byPath[record.Path]
			reasons, forced, ok := decideClean(wt, result.mainPath, repoPath, int(force))
			if !ok {
				report.Skipped = append(report.Skipped, cleanupEntry(wt, reasons, false, true))
				continue
			}
			plan = append(plan, cleanupEntry(wt, reasons, forced, *saveBranch))
			report.Candidates = append(report.Candidates, wt)
		}
	}

	if *dryRun {
		report.Planned = plan
		if *jsonOutput {
			return finishClean(stdout, report, true)
		}
		printCleanPlan(stdout, repoPath, plan, report.Skipped, len(targets) == 0)
		return cleanExitCode(report, true)
	}

	for _, entry := range plan {
		wt := byPath[entry.Path]
		if err := removeWorktreeWith(repoPath, wt.Path, wt.Locked); err != nil {
			fmt.Fprintf(stderr, "gw clean: cannot remove %s: %v\n", displayPath(repoPath, wt.Path), err)
			report.Errors = append(report.Errors, ResultError{Source: "git", Code: "worktree_remove_failed", Message: fmt.Sprintf("%s: %v", wt.Path, err)})
			continue
		}
		_ = removeLock(wt.Path)
		if entry.DeleteBranch {
			if err := gitRun(repoPath, "branch", "-D", entry.Branch); err != nil {
				entry.DeleteBranch = false
				fmt.Fprintf(stderr, "gw clean: cannot delete branch %s: %v\n", entry.Branch, err)
				report.Errors = append(report.Errors, ResultError{Source: "git", Code: "branch_delete_failed", Message: fmt.Sprintf("%s: %v", entry.Branch, err)})
			}
		}
		report.Removed = append(report.Removed, entry)
		if !*jsonOutput {
			fmt.Fprintf(stdout, "removed %s\n", displayPath(repoPath, wt.Path))
			switch {
			case entry.DeleteBranch:
				fmt.Fprintf(stdout, "  deleted branch %s (was %s)\n", entry.Branch, entry.Commit)
			case entry.Branch != "":
				fmt.Fprintf(stdout, "  kept branch %s (%s)\n", entry.Branch, entry.Commit)
			}
		}
	}
	if *jsonOutput {
		return finishClean(stdout, report, false)
	}
	for _, entry := range report.Skipped {
		fmt.Fprintf(stdout, "skipped %s: %s\n", displayPath(repoPath, entry.Path), strings.Join(entry.Reasons, ", "))
	}
	if len(plan) == 0 && len(targets) == 0 {
		fmt.Fprintln(stdout, "No recommended worktrees to remove.")
	}
	return cleanExitCode(report, false)
}

// decideClean decides whether a worktree given on the command line is removed.
// The main and current worktrees are never removed. Without --force only
// worktrees that the cleanup judgment recommends are removed; --force skips
// the judgment; --force twice also removes worktrees with a running agent or
// a lock. forced reports that the judgment was overridden.
func decideClean(wt Worktree, mainPath, currentPath string, force int) (reasons []string, forced, ok bool) {
	if wt.Path == mainPath {
		return []string{"main_worktree"}, false, false
	}
	if wt.Path == currentPath {
		return []string{"current_worktree"}, false, false
	}
	var running []string
	if wt.Locked {
		running = append(running, "worktree_locked")
	}
	if wt.Lock != nil && wt.Lock.Alive {
		running = append(running, "agent_process_running")
	}
	if wt.Agent.Lifecycle == "active" {
		running = append(running, "agent_session_active")
	}
	if len(running) > 0 && force < 2 {
		return running, false, false
	}
	if wt.Cleanup.Recommendation == "recommended" {
		return wt.Cleanup.Reasons, false, true
	}
	if force >= 1 {
		if len(running) > 0 {
			return running, true, true
		}
		return wt.Cleanup.Reasons, true, true
	}
	return wt.Cleanup.Reasons, false, false
}

func cleanupEntry(wt Worktree, reasons []string, forced, saveBranch bool) CleanupEntry {
	entry := CleanupEntry{
		Path:         wt.Path,
		Branch:       wt.Branch,
		Commit:       wt.Head,
		DeleteBranch: !saveBranch && wt.Branch != "",
		Forced:       forced,
		Reasons:      reasons,
		Uncommitted:  wt.Git.Uncommitted,
		Unpushed:     wt.Git.Unpushed,
	}
	if wt.GitHub.PR != nil {
		entry.PRState = wt.GitHub.PR.State
	}
	return entry
}

func finishClean(stdout io.Writer, report CleanupReport, dryRun bool) int {
	if code := writeJSON(stdout, report); code != 0 {
		return code
	}
	return cleanExitCode(report, dryRun)
}

// cleanExitCode is 1 when a given worktree was not found or, when applying,
// when any error occurred (as before, including GitHub lookup errors).
// Worktrees skipped by the judgment are not failures.
func cleanExitCode(report CleanupReport, dryRun bool) int {
	for _, e := range report.Errors {
		if !dryRun || e.Source == "gw" {
			return 1
		}
	}
	return 0
}

func printCleanPlan(w io.Writer, repoPath string, plan, skipped []CleanupEntry, recommendedOnly bool) {
	for _, entry := range plan {
		label := "remove"
		if recommendedOnly {
			label = "recommended"
		}
		fmt.Fprintf(w, "[%s] %s\n", label, displayPath(repoPath, entry.Path))
		if entry.Forced {
			fmt.Fprintf(w, "  - forced: %s\n", strings.Join(entry.Reasons, ", "))
		} else {
			for _, reason := range entry.Reasons {
				fmt.Fprintf(w, "  - %s\n", reason)
			}
		}
		printLoss(w, entry)
	}
	for _, entry := range skipped {
		fmt.Fprintf(w, "[skip] %s\n", displayPath(repoPath, entry.Path))
		for _, reason := range entry.Reasons {
			fmt.Fprintf(w, "  - %s\n", reason)
		}
	}
	if len(plan) == 0 && len(skipped) == 0 && recommendedOnly {
		fmt.Fprintln(w, "No recommended worktrees to remove.")
	}
}

// printLoss prints what removing the entry loses.
func printLoss(w io.Writer, entry CleanupEntry) {
	switch {
	case entry.DeleteBranch:
		fmt.Fprintf(w, "    branch: %s at %s (delete)\n", entry.Branch, entry.Commit)
	case entry.Branch != "":
		fmt.Fprintf(w, "    branch: %s at %s (keep)\n", entry.Branch, entry.Commit)
	}
	fmt.Fprintf(w, "    uncommitted files: %d\n", entry.Uncommitted)
	if entry.Unpushed != nil {
		fmt.Fprintf(w, "    unpushed commits: %d\n", *entry.Unpushed)
	} else {
		fmt.Fprintln(w, "    unpushed commits: unknown")
	}
	fmt.Fprintf(w, "    pull request: %s\n", valueOr(entry.PRState, "none"))
}
