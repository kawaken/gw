package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const schemaVersion = 1

type Result struct {
	SchemaVersion int           `json:"schema_version"`
	Repository    Repository    `json:"repository"`
	Worktrees     []Worktree    `json:"worktrees,omitempty"`
	Sources       Sources       `json:"sources"`
	Errors        []ResultError `json:"errors,omitempty"`
}

type Repository struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
}

type Worktree struct {
	Path     string       `json:"path"`
	Branch   string       `json:"branch,omitempty"`
	Head     string       `json:"head,omitempty"`
	Detached bool         `json:"detached,omitempty"`
	Locked   bool         `json:"locked,omitempty"`
	Git      GitState     `json:"git"`
	GitHub   GitHubState  `json:"github"`
	Agent    AgentState   `json:"agent"`
	Cleanup  CleanupState `json:"cleanup"`
}

type GitState struct {
	Clean       bool   `json:"clean"`
	StatusError string `json:"status_error,omitempty"`
	Ahead       *int   `json:"ahead,omitempty"`
	Behind      *int   `json:"behind,omitempty"`
	LastCommit  string `json:"last_commit_at,omitempty"`
}

type GitHubState struct {
	PR     *PullRequest `json:"pr"`
	Status string       `json:"status"`
}

type PullRequest struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	State      string `json:"state"`
	MergedAt   string `json:"merged_at,omitempty"`
	URL        string `json:"url"`
	HeadBranch string `json:"head_branch"`
}

type ghPullRequest struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	State      string `json:"state"`
	MergedAt   string `json:"mergedAt"`
	URL        string `json:"url"`
	HeadBranch string `json:"headRefName"`
}

type AgentState struct {
	Provider   string `json:"provider,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Lifecycle  string `json:"lifecycle"`
	Activity   string `json:"activity"`
	ObservedAt string `json:"observed_at,omitempty"`
}

type CleanupState struct {
	Recommendation string   `json:"recommendation"`
	Reasons        []string `json:"reasons,omitempty"`
}

type Sources struct {
	GitHub string `json:"github"`
	Agent  string `json:"agent"`
}

type ResultError struct {
	Source  string `json:"source"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CleanupReport struct {
	SchemaVersion int           `json:"schema_version"`
	Repository    Repository    `json:"repository"`
	Mode          string        `json:"mode"`
	Candidates    []Worktree    `json:"candidates,omitempty"`
	Removed       []string      `json:"removed,omitempty"`
	Errors        []ResultError `json:"errors,omitempty"`
}

type worktreeRecord struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
	Locked   bool
}

type sessionStore struct {
	Version  int             `json:"version"`
	Sessions []sessionRecord `json:"sessions"`
}

type sessionRecord struct {
	Provider       string `json:"provider"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	CWD            string `json:"cwd"`
	Event          string `json:"event"`
	Reason         string `json:"reason,omitempty"`
	Lifecycle      string `json:"lifecycle"`
	ObservedAt     string `json:"observed_at"`
}

type hookEvent struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	Event          string `json:"hook_event_name"`
	Reason         string `json:"reason"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "help", "--help", "-h":
		printHelp(stdout)
		return 0
	case "list", "ls":
		return runList(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "refresh":
		return runRefresh(args[1:], stdout, stderr)
	case "clean":
		return runClean(args[1:], stdout, stderr)
	case "guide":
		return runGuide(args[1:], stdout, stderr)
	case "agent-event":
		return runAgentEvent(args[1:], os.Stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gw: unknown command %q\n\n", args[0])
		printHelp(stderr)
		return 2
	}
}

func runList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "gw list: unexpected arguments")
		return 2
	}

	result, err := collectResult()
	if err != nil {
		return printCommandError(stderr, err)
	}
	if *jsonOutput {
		return writeJSON(stdout, result)
	}
	printList(stdout, result)
	return 0
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "gw inspect: expected at most one worktree path")
		return 2
	}

	result, err := collectResult()
	if err != nil {
		return printCommandError(stderr, err)
	}
	query := ""
	if fs.NArg() == 1 {
		query = fs.Arg(0)
	}
	selected, err := resolveWorktree(result.Worktrees, query)
	if err != nil {
		return printCommandError(stderr, err)
	}
	if *jsonOutput {
		return writeJSON(stdout, Result{
			SchemaVersion: schemaVersion,
			Repository:    result.Repository,
			Worktrees:     []Worktree{selected},
			Sources:       result.Sources,
			Errors:        result.Errors,
		})
	}
	printInspect(stdout, selected)
	return 0
}

func runRefresh(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "gw refresh: unexpected arguments")
		return 2
	}

	result, err := collectResult()
	if err != nil {
		return printCommandError(stderr, err)
	}
	if *jsonOutput {
		return writeJSON(stdout, result)
	}
	fmt.Fprintln(stdout, "Git and available integrations refreshed.")
	return 0
}

func runClean(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "show recommended cleanup without removing worktrees")
	jsonOutput := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "gw clean: unexpected arguments")
		return 2
	}

	result, err := collectResult()
	if err != nil {
		return printCommandError(stderr, err)
	}
	candidates := make([]Worktree, 0)
	for _, wt := range result.Worktrees {
		if wt.Cleanup.Recommendation == "recommended" && wt.Path != result.Repository.Path {
			candidates = append(candidates, wt)
		}
	}
	if *dryRun {
		if *jsonOutput {
			return writeJSON(stdout, CleanupReport{
				SchemaVersion: schemaVersion,
				Repository:    result.Repository,
				Mode:          "dry-run",
				Candidates:    candidates,
				Errors:        result.Errors,
			})
		}
		for _, wt := range candidates {
			fmt.Fprintf(stdout, "[recommended] %s\n", wt.Path)
			for _, reason := range wt.Cleanup.Reasons {
				fmt.Fprintf(stdout, "  - %s\n", reason)
			}
		}
		if len(candidates) == 0 {
			fmt.Fprintln(stdout, "No recommended worktrees to remove.")
		}
		return 0
	}

	removed := make([]string, 0, len(candidates))
	cleanupErrors := append([]ResultError{}, result.Errors...)
	for _, wt := range candidates {
		if err := gitRun(result.Repository.Path, "worktree", "remove", wt.Path); err != nil {
			fmt.Fprintf(stderr, "gw clean: cannot remove %s: %v\n", wt.Path, err)
			cleanupErrors = append(cleanupErrors, ResultError{Source: "git", Code: "worktree_remove_failed", Message: fmt.Sprintf("%s: %v", wt.Path, err)})
			continue
		}
		removed = append(removed, wt.Path)
		if !*jsonOutput {
			fmt.Fprintf(stdout, "removed %s\n", wt.Path)
		}
	}
	if *jsonOutput {
		return writeJSON(stdout, CleanupReport{
			SchemaVersion: schemaVersion,
			Repository:    result.Repository,
			Mode:          "apply",
			Candidates:    candidates,
			Removed:       removed,
			Errors:        cleanupErrors,
		})
	}
	if len(removed) == 0 {
		fmt.Fprintln(stdout, "No recommended worktrees to remove.")
	}
	return 0
}

func runGuide(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printGuideOverview(stdout)
		return 0
	}

	switch args[0] {
	case "list", "inspect", "refresh", "clean", "json":
		printGuideTopic(stdout, args[0])
		return 0
	case "agent-hook":
		if len(args) != 2 || (args[1] != "claude" && args[1] != "codex") {
			fmt.Fprintln(stderr, "usage: gw guide agent-hook claude|codex")
			return 2
		}
		printAgentHookGuide(stdout, args[1])
		return 0
	default:
		fmt.Fprintf(stderr, "gw guide: unknown topic %q\n", args[0])
		fmt.Fprintln(stderr, "available topics: list, inspect, refresh, clean, json, agent-hook claude|codex")
		return 2
	}
}

func runAgentEvent(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent-event", flag.ContinueOnError)
	fs.SetOutput(stderr)
	provider := fs.String("provider", "", "agent provider: claude or codex")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *provider != "claude" && *provider != "codex" {
		fmt.Fprintln(stderr, "gw agent-event: --provider must be claude or codex")
		return 2
	}

	var event hookEvent
	decoder := json.NewDecoder(stdin)
	if err := decoder.Decode(&event); err != nil {
		return printCommandError(stderr, fmt.Errorf("read hook event: %w", err))
	}
	if event.SessionID == "" || event.CWD == "" || event.Event == "" {
		return printCommandError(stderr, errors.New("hook event must contain session_id, cwd, and hook_event_name"))
	}

	lifecycle := "unknown"
	switch event.Event {
	case "SessionStart":
		lifecycle = "active"
	case "SessionEnd":
		lifecycle = "ended"
	default:
		return printCommandError(stderr, fmt.Errorf("unsupported hook event %q", event.Event))
	}

	path, err := statePath("sessions.json")
	if err != nil {
		return printCommandError(stderr, err)
	}
	store, err := readSessionStore(path)
	if err != nil {
		return printCommandError(stderr, err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := sessionRecord{
		Provider:       *provider,
		SessionID:      event.SessionID,
		TranscriptPath: event.TranscriptPath,
		CWD:            mustAbs(event.CWD),
		Event:          event.Event,
		Reason:         event.Reason,
		Lifecycle:      lifecycle,
		ObservedAt:     now,
	}
	updated := false
	for i := range store.Sessions {
		if store.Sessions[i].Provider == record.Provider && store.Sessions[i].SessionID == record.SessionID {
			store.Sessions[i] = record
			updated = true
			break
		}
	}
	if !updated {
		store.Sessions = append(store.Sessions, record)
	}
	if err := writeSessionStore(path, store); err != nil {
		return printCommandError(stderr, err)
	}
	return 0
}

func collectResult() (Result, error) {
	repoPath, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return Result{}, fmt.Errorf("not inside a Git repository: %w", err)
	}
	repoPath = mustAbs(strings.TrimSpace(repoPath))
	records, err := gitWorktrees(repoPath)
	if err != nil {
		return Result{}, err
	}
	branch := ""
	if len(records) > 0 {
		branch = records[0].Branch
	}
	sessions, sessionErr := readCurrentSessions()
	result := Result{
		SchemaVersion: schemaVersion,
		Repository:    Repository{Path: repoPath, Branch: branch},
		Sources:       Sources{GitHub: "unavailable", Agent: "unavailable"},
	}
	githubStates, githubErr := collectGitHubStates(repoPath, records)
	if githubErr == nil {
		result.Sources.GitHub = "gh"
	} else if !errors.Is(githubErr, errGitHubUnavailable) {
		result.Sources.GitHub = "unknown"
		result.Errors = append(result.Errors, ResultError{Source: "github", Code: "unavailable", Message: githubErr.Error()})
	}
	if sessionErr == nil {
		if len(sessions) > 0 {
			result.Sources.Agent = "local-state"
		}
	} else if !errors.Is(sessionErr, os.ErrNotExist) {
		result.Errors = append(result.Errors, ResultError{Source: "agent", Code: "state_unavailable", Message: sessionErr.Error()})
	}

	for _, record := range records {
		wt := Worktree{
			Path:     record.Path,
			Branch:   record.Branch,
			Head:     record.Head,
			Detached: record.Detached,
			Locked:   record.Locked,
			Git:      collectGitState(record),
			GitHub:   githubStates[record.Branch],
			Agent:    agentForPath(record.Path, sessions),
		}
		wt.Cleanup = cleanupFor(wt, record.Path == repoPath)
		result.Worktrees = append(result.Worktrees, wt)
	}
	sort.SliceStable(result.Worktrees, func(i, j int) bool {
		return result.Worktrees[i].Path < result.Worktrees[j].Path
	})
	return result, nil
}

var errGitHubUnavailable = errors.New("gh is not available")

func collectGitHubStates(repoPath string, records []worktreeRecord) (map[string]GitHubState, error) {
	states := make(map[string]GitHubState)
	if _, err := exec.LookPath("gh"); err != nil {
		for _, record := range records {
			states[record.Branch] = GitHubState{Status: "unavailable"}
		}
		return states, errGitHubUnavailable
	}
	var firstErr error
	for _, record := range records {
		if record.Branch == "" {
			states[record.Branch] = GitHubState{Status: "unknown"}
			continue
		}
		if _, ok := states[record.Branch]; ok {
			continue
		}
		state, err := githubPRForBranch(repoPath, record.Branch)
		if err != nil {
			states[record.Branch] = GitHubState{Status: "unknown"}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		states[record.Branch] = state
	}
	return states, firstErr
}

func githubPRForBranch(repoPath, branch string) (GitHubState, error) {
	cmd := exec.Command("gh", "pr", "list", "--state", "all", "--head", branch, "--limit", "10", "--json", "number,title,state,mergedAt,url,headRefName")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return GitHubState{}, errors.New(strings.TrimSpace(string(exitErr.Stderr)))
		}
		return GitHubState{}, err
	}
	var rawPRs []ghPullRequest
	if err := json.Unmarshal(out, &rawPRs); err != nil {
		return GitHubState{}, fmt.Errorf("parse gh output: %w", err)
	}
	if len(rawPRs) == 0 {
		return GitHubState{Status: "available"}, nil
	}
	// A branch normally has one PR. Prefer an open PR, otherwise the most
	// recently listed historical PR, which is what gh returns first.
	selected := rawPRs[0]
	for _, pr := range rawPRs {
		if strings.EqualFold(pr.State, "OPEN") {
			selected = pr
			break
		}
	}
	return GitHubState{PR: &PullRequest{
		Number:     selected.Number,
		Title:      selected.Title,
		State:      selected.State,
		MergedAt:   selected.MergedAt,
		URL:        selected.URL,
		HeadBranch: selected.HeadBranch,
	}, Status: "available"}, nil
}

func gitWorktrees(repoPath string) ([]worktreeRecord, error) {
	out, err := gitOutputFrom(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	return parseWorktreeList(out)
}

func parseWorktreeList(out string) ([]worktreeRecord, error) {
	var records []worktreeRecord
	var current *worktreeRecord
	flush := func() {
		if current != nil && current.Path != "" {
			records = append(records, *current)
		}
		current = nil
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			if line == "detached" && current != nil {
				current.Detached = true
			}
			continue
		}
		if key == "worktree" {
			flush()
			current = &worktreeRecord{Path: mustAbs(value)}
			continue
		}
		if current == nil {
			continue
		}
		switch key {
		case "HEAD":
			current.Head = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "locked":
			current.Locked = true
		}
	}
	flush()
	return records, nil
}

func collectGitState(record worktreeRecord) GitState {
	state := GitState{Clean: true}
	out, err := gitOutputFrom(record.Path, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		state.Clean = false
		state.StatusError = err.Error()
		return state
	}
	state.Clean = strings.TrimSpace(out) == ""
	if timestamp, err := gitOutputFrom(record.Path, "log", "-1", "--format=%cI"); err == nil {
		state.LastCommit = strings.TrimSpace(timestamp)
	}
	if counts, err := gitOutputFrom(record.Path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD"); err == nil {
		parts := strings.Fields(counts)
		if len(parts) == 2 {
			behind, ahead := parseInt(parts[0]), parseInt(parts[1])
			state.Behind, state.Ahead = &behind, &ahead
		}
	}
	return state
}

func cleanupFor(wt Worktree, isRepository bool) CleanupState {
	if isRepository {
		return CleanupState{Recommendation: "keep", Reasons: []string{"main_worktree"}}
	}
	if wt.Locked {
		return CleanupState{Recommendation: "keep", Reasons: []string{"worktree_locked"}}
	}
	if !wt.Git.Clean {
		return CleanupState{Recommendation: "review", Reasons: []string{"worktree_dirty"}}
	}
	if wt.Agent.Lifecycle == "active" {
		return CleanupState{Recommendation: "keep", Reasons: []string{"agent_session_active"}}
	}
	if wt.GitHub.Status == "unknown" || wt.GitHub.Status == "unavailable" {
		return CleanupState{Recommendation: "review", Reasons: []string{"github_state_unknown"}}
	}
	if wt.GitHub.PR == nil {
		return CleanupState{Recommendation: "review", Reasons: []string{"no_pull_request"}}
	}
	switch strings.ToUpper(wt.GitHub.PR.State) {
	case "MERGED":
		return CleanupState{Recommendation: "recommended", Reasons: []string{"pull_request_merged", "worktree_clean"}}
	case "CLOSED":
		return CleanupState{Recommendation: "recommended", Reasons: []string{"pull_request_closed", "worktree_clean"}}
	default:
		return CleanupState{Recommendation: "review", Reasons: []string{"pull_request_open"}}
	}
}

func agentForPath(path string, sessions []sessionRecord) AgentState {
	path = mustAbs(path)
	var selected *sessionRecord
	for i := range sessions {
		if mustAbs(sessions[i].CWD) != path {
			continue
		}
		if selected == nil || sessions[i].ObservedAt > selected.ObservedAt {
			selected = &sessions[i]
		}
	}
	if selected == nil {
		return AgentState{Lifecycle: "unknown", Activity: "unknown"}
	}
	return AgentState{
		Provider:   selected.Provider,
		SessionID:  selected.SessionID,
		Lifecycle:  selected.Lifecycle,
		Activity:   "unknown",
		ObservedAt: selected.ObservedAt,
	}
}

func resolveWorktree(worktrees []Worktree, query string) (Worktree, error) {
	if query == "" || query == "main" {
		return worktrees[0], nil
	}
	absQuery := mustAbs(query)
	for _, wt := range worktrees {
		if wt.Path == absQuery || filepath.Base(wt.Path) == query {
			return wt, nil
		}
	}
	return Worktree{}, fmt.Errorf("worktree %q not found", query)
}

func readCurrentSessions() ([]sessionRecord, error) {
	path, err := statePath("sessions.json")
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	} else if err != nil {
		return nil, err
	}
	store, err := readSessionStore(path)
	if err != nil {
		return nil, err
	}
	return store.Sessions, nil
}

func readSessionStore(path string) (sessionStore, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return sessionStore{Version: 1}, nil
	}
	if err != nil {
		return sessionStore{}, fmt.Errorf("read session state: %w", err)
	}
	var store sessionStore
	if err := json.Unmarshal(data, &store); err != nil {
		return sessionStore{}, fmt.Errorf("parse session state: %w", err)
	}
	if store.Version == 0 {
		store.Version = 1
	}
	return store, nil
}

func writeSessionStore(path string, store sessionStore) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session state: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sessions-*.tmp")
	if err != nil {
		return fmt.Errorf("create session state temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set session state permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write session state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close session state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace session state: %w", err)
	}
	return nil
}

func statePath(name string) (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "gw", name), nil
}

func gitOutput(args ...string) (string, error) {
	return gitOutputFrom("", args...)
}

func gitOutputFrom(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			message := strings.TrimSpace(string(exitErr.Stderr))
			if message != "" {
				return "", errors.New(message)
			}
		}
		return "", err
	}
	return string(out), nil
}

func gitRun(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func writeJSON(w io.Writer, value any) int {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return 1
	}
	return 0
}

func printCommandError(w io.Writer, err error) int {
	fmt.Fprintf(w, "gw: %v\n", err)
	return 1
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: gw <command> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  list       worktreeの状態を一覧表示")
	fmt.Fprintln(w, "  inspect    worktreeの詳細と判定理由を表示")
	fmt.Fprintln(w, "  refresh    Git/GitHubの状態を更新")
	fmt.Fprintln(w, "  clean      cleanup推奨対象を削除")
	fmt.Fprintln(w, "  guide      AI向けに各サブコマンドの使い方や連携方法を説明")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "AI agents: run 'gw guide' for command usage and integration guidance.")
}

func printList(w io.Writer, result Result) {
	fmt.Fprintln(w, "PATH\tBRANCH\tGIT\tAGENT\tCLEANUP")
	for _, wt := range result.Worktrees {
		branch := wt.Branch
		if branch == "" {
			branch = "(detached)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", wt.Path, branch, gitStatus(wt), agentStatus(wt.Agent), wt.Cleanup.Recommendation)
	}
}

func printInspect(w io.Writer, wt Worktree) {
	fmt.Fprintf(w, "Path: %s\n", wt.Path)
	fmt.Fprintf(w, "Branch: %s\n", valueOr(wt.Branch, "(detached)"))
	fmt.Fprintf(w, "HEAD: %s\n", wt.Head)
	fmt.Fprintf(w, "Git: %s\n", gitStatus(wt))
	fmt.Fprintf(w, "Agent: %s\n", agentStatus(wt.Agent))
	fmt.Fprintf(w, "Cleanup: %s\n", wt.Cleanup.Recommendation)
	if len(wt.Cleanup.Reasons) > 0 {
		fmt.Fprintln(w, "Reasons:")
		for _, reason := range wt.Cleanup.Reasons {
			fmt.Fprintf(w, "  - %s\n", reason)
		}
	}
}

func printGuideOverview(w io.Writer) {
	fmt.Fprintln(w, "# gw guide")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "gwはGit worktreeの状態を観測し、GitHub PRとエージェントセッションの情報を組み合わせてcleanup候補を判定します。")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Commands")
	fmt.Fprintln(w, "- gw list [--json]")
	fmt.Fprintln(w, "- gw inspect <worktree> [--json]")
	fmt.Fprintln(w, "- gw refresh [--json]")
	fmt.Fprintln(w, "- gw clean --dry-run [--json]")
	fmt.Fprintln(w, "- gw clean")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Guide topics")
	fmt.Fprintln(w, "- gw guide list")
	fmt.Fprintln(w, "- gw guide inspect")
	fmt.Fprintln(w, "- gw guide refresh")
	fmt.Fprintln(w, "- gw guide clean")
	fmt.Fprintln(w, "- gw guide json")
	fmt.Fprintln(w, "- gw guide agent-hook claude|codex")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run gw guide <topic> for details.")
}

func printGuideTopic(w io.Writer, topic string) {
	switch topic {
	case "list":
		fmt.Fprintln(w, "# gw list")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "現在のリポジトリに紐づくGit worktreeを一覧表示します。列はPATH、BRANCH、GIT（clean/dirty）、AGENT、CLEANUP（recommended/review/keep）です。")
		fmt.Fprintln(w, "GitHubやagent情報が取得できない場合は、値を推測せずunknownとして扱います。")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		fmt.Fprintln(w, "  --json    schema_version、repository、worktrees、sources、errorsを含む構造化出力（詳細は gw guide json）")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "個別のworktreeの判定理由まで見たい場合は gw inspect <worktree> を使ってください。")
	case "inspect":
		fmt.Fprintln(w, "# gw inspect [<worktree>]")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "指定したworktreeのGit、GitHub、agent、cleanup判定と判定理由を表示します。引数を省略するとmain worktreeを対象にします。")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		fmt.Fprintln(w, "  --json    対象worktree1件を含む gw list --json と同じResult構造で出力")
	case "refresh":
		fmt.Fprintln(w, "# gw refresh")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Gitと、利用可能なGitHub/agentの連携先から現在の状態を再取得して表示します。何かを永続化する更新処理ではありません。")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		fmt.Fprintln(w, "  --json    gw list --json と同じResult構造で出力")
	case "clean":
		fmt.Fprintln(w, "# gw clean")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "cleanup判定がrecommendedのworktreeにgit worktree removeを実行します。dirty、現在のworktree、activeなagent sessionは削除しません。ブランチは削除しません。")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		fmt.Fprintln(w, "  --dry-run    削除は行わず、削除候補と理由だけを表示")
		fmt.Fprintln(w, "  --json       CleanupReport（schema_version、repository、mode、candidates、removed、errors）を出力")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "判定基準:")
		fmt.Fprintln(w, "  recommended  pull requestがMERGED/CLOSED、worktreeがclean、activeなagentがない")
		fmt.Fprintln(w, "  review       dirty、pull requestがない/open、GitHubの状態が不明などで自動削除しない")
		fmt.Fprintln(w, "  keep         main worktree、ロック済み、activeなagentがある")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "最初に gw clean --dry-run を実行して削除候補を確認することを推奨します。")
	case "json":
		fmt.Fprintln(w, "# gw JSON output")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "list、inspect、refreshは次のトップレベル構造で出力します。")
		fmt.Fprintln(w, "  schema_version, repository, worktrees, sources, errors")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "各worktreeはpath、branch、head、detached、lockedに加えて次の状態を含みます。")
		fmt.Fprintln(w, "  git      clean/dirty、upstreamとの差分、最終コミット時刻")
		fmt.Fprintln(w, "  github   pull requestと取得状態")
		fmt.Fprintln(w, "  agent    provider、session ID、lifecycle、activity、観測時刻")
		fmt.Fprintln(w, "  cleanup  recommended/review/keepと判定理由")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "取得できない値はnullまたはunknownで表現し、推測しません。")
		fmt.Fprintln(w, "clean --jsonの結果はschema_version、repository、mode、candidates、removed、errorsを含みます（詳細は gw guide clean）。")
	}
}

func printAgentHookGuide(w io.Writer, provider string) {
	fmt.Fprintf(w, "# gw guide agent-hook %s\n\n", provider)
	fmt.Fprintf(w, "%sのSessionStart / SessionEnd hookから、gw agent-eventへJSONを渡します。既存設定を自動編集しないため、以下を手動で既存のhooks設定に追加してください。\n\n", provider)
	fmt.Fprintln(w, "```json")
	fmt.Fprintln(w, "{\n  \"hooks\": {")
	fmt.Fprintln(w, "    \"SessionStart\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"gw agent-event --provider "+provider+"\"}]}],")
	fmt.Fprintln(w, "    \"SessionEnd\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"gw agent-event --provider "+provider+"\"}]}]")
	fmt.Fprintln(w, "  }\n}")
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
	if provider == "claude" {
		fmt.Fprintln(w, "設定場所: ~/.claude/settings.json またはプロジェクトの .claude/settings.json")
	} else {
		fmt.Fprintln(w, "設定場所: ~/.codex/hooks.json、~/.codex/config.toml、またはプロジェクトの .codex/hooks.json / config.toml")
	}
	fmt.Fprintln(w, "hookはstdinのJSONをgwの状態ファイルへ保存し、stdoutには何も出力しません。")
	fmt.Fprintln(w, "動作確認: gw list --json または gw inspect <worktree> --json")
}

func gitStatus(wt Worktree) string {
	if wt.Git.StatusError != "" {
		return "error"
	}
	if wt.Git.Clean {
		return "clean"
	}
	return "dirty"
}

func agentStatus(agent AgentState) string {
	if agent.Provider == "" {
		return "unknown"
	}
	return agent.Provider + ":" + agent.Lifecycle
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func parseInt(value string) int {
	var result int
	fmt.Sscanf(value, "%d", &result)
	return result
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}
