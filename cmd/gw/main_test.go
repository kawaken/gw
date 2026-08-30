package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseWorktreeList(t *testing.T) {
	input := "worktree /tmp/example\nHEAD abc123\nbranch refs/heads/main\n\nworktree /tmp/example-wt/fix\nHEAD def456\nbranch refs/heads/fix/login\n\nworktree /tmp/detached\nHEAD 789abc\ndetached\n"

	got, err := parseWorktreeList(input)
	if err != nil {
		t.Fatalf("parseWorktreeList returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d worktrees, want 3", len(got))
	}
	if got[1].Branch != "fix/login" {
		t.Fatalf("got branch %q, want fix/login", got[1].Branch)
	}
	if !got[2].Detached {
		t.Fatal("detached worktree was not marked detached")
	}
}

func TestCleanupFor(t *testing.T) {
	base := Worktree{Git: GitState{Clean: true}, Agent: AgentState{Lifecycle: "unknown"}}
	if got := cleanupFor(base, false); got.Recommendation != "review" {
		t.Fatalf("unknown GitHub state recommendation = %q, want review", got.Recommendation)
	}
	if got := cleanupFor(base, true); got.Recommendation != "keep" {
		t.Fatalf("main worktree recommendation = %q, want keep", got.Recommendation)
	}
	base.Git.Clean = false
	if got := cleanupFor(base, false); got.Recommendation != "review" {
		t.Fatalf("dirty worktree recommendation = %q, want review", got.Recommendation)
	}
	base.Git.Clean = true
	base.Agent.Lifecycle = "active"
	if got := cleanupFor(base, false); got.Recommendation != "keep" {
		t.Fatalf("active agent recommendation = %q, want keep", got.Recommendation)
	}
	base.Agent.Lifecycle = "ended"
	base.GitHub = GitHubState{Status: "available", PR: &PullRequest{State: "MERGED"}}
	if got := cleanupFor(base, false); got.Recommendation != "recommended" {
		t.Fatalf("merged PR recommendation = %q, want recommended", got.Recommendation)
	}
}

func TestPrintListAlignsColumns(t *testing.T) {
	result := Result{Repository: Repository{Path: "/tmp/repository"}, Worktrees: []Worktree{
		{Path: "/tmp/repository", Branch: "main", Git: GitState{Clean: true}, Agent: AgentState{Lifecycle: "ended", Provider: "codex"}, Cleanup: CleanupState{Recommendation: "keep"}},
		{Path: "/tmp/repository-wt/to-go", Branch: "to-go", Git: GitState{}, Agent: AgentState{Lifecycle: "unknown"}, Cleanup: CleanupState{Recommendation: "review"}},
		{Path: "/tmp/repository/.claude/worktrees/codex-hooks-json-structure-0a606c", Git: GitState{Clean: true}, Agent: AgentState{Lifecycle: "ended", Provider: "claude"}, Cleanup: CleanupState{Recommendation: "review"}},
	}}

	var output bytes.Buffer
	printList(&output, result)
	want := "PATH                                                 BRANCH      GIT    AGENT         CLEANUP\n" +
		".                                                    main        clean  codex:ended   keep\n" +
		"/tmp/repository-wt/to-go                             to-go       dirty  unknown       review\n" +
		".claude/worktrees/codex-hooks-json-structure-0a606c  (detached)  clean  claude:ended  review\n"
	if output.String() != want {
		t.Fatalf("printList output =\n%s, want =\n%s", output.String(), want)
	}
}

func TestDisplayPath(t *testing.T) {
	repoPath := "/tmp/repository"
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "repository root", path: repoPath, want: "."},
		{name: "inside repository", path: "/tmp/repository/.claude/worktrees/fix", want: ".claude/worktrees/fix"},
		{name: "outside repository", path: "/tmp/repository-wt/fix", want: "/tmp/repository-wt/fix"},
		{name: "shared prefix outside repository", path: "/tmp/repository-other/fix", want: "/tmp/repository-other/fix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayPath(repoPath, tt.path); got != tt.want {
				t.Fatalf("displayPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestSessionStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	want := sessionStore{Version: 1, Sessions: []sessionRecord{{
		Provider:  "claude",
		SessionID: "session-1",
		CWD:       "/tmp/worktree",
		Event:     "SessionStart",
		Lifecycle: "active",
	}}}
	if err := writeSessionStore(path, want); err != nil {
		t.Fatalf("writeSessionStore returned error: %v", err)
	}
	got, err := readSessionStore(path)
	if err != nil {
		t.Fatalf("readSessionStore returned error: %v", err)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].SessionID != "session-1" {
		t.Fatalf("round trip result = %+v", got)
	}
	if mode := fileMode(t, path); mode.Perm() != 0o600 {
		t.Fatalf("session state mode = %o, want 600", mode.Perm())
	}
}

func TestRunGuideAgentHook(t *testing.T) {
	var output bytes.Buffer
	if code := run([]string{"guide", "agent-hook", "codex"}, &output, &output); code != 0 {
		t.Fatalf("run returned %d", code)
	}
	for _, want := range []string{"SessionStart", "SessionEnd", "gw agent-event --provider codex", "~/.codex/hooks.json"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("guide output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestRunGuideAgentHookJSONStructure(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		var output bytes.Buffer
		if code := run([]string{"guide", "agent-hook", provider}, &output, &output); code != 0 {
			t.Fatalf("run returned %d for provider %s", code, provider)
		}

		block := extractJSONCodeBlock(t, output.String())
		var parsed map[string]any
		if err := json.Unmarshal([]byte(block), &parsed); err != nil {
			t.Fatalf("guide output for %s is not valid JSON: %v\n%s", provider, err, block)
		}

		hooks, ok := parsed["hooks"].(map[string]any)
		if !ok {
			t.Fatalf("guide output for %s: expected top-level \"hooks\" object, got %#v", provider, parsed)
		}
		for _, event := range []string{"SessionStart", "SessionEnd"} {
			if _, ok := hooks[event]; !ok {
				t.Fatalf("guide output for %s: hooks.%s is missing", provider, event)
			}
		}
		for _, event := range []string{"SessionStart", "SessionEnd"} {
			if _, ok := parsed[event]; ok {
				t.Fatalf("guide output for %s: %s must be nested under \"hooks\", not top-level", provider, event)
			}
		}
	}
}

func extractJSONCodeBlock(t *testing.T, output string) string {
	t.Helper()
	start := strings.Index(output, "```json\n")
	if start == -1 {
		t.Fatalf("no ```json code block found in output:\n%s", output)
	}
	start += len("```json\n")
	end := strings.Index(output[start:], "```")
	if end == -1 {
		t.Fatalf("unterminated ```json code block in output:\n%s", output)
	}
	return output[start : start+end]
}

func TestRunAgentEvent(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	input := strings.NewReader(`{"session_id":"session-1","cwd":"/tmp/worktree","hook_event_name":"SessionStart"}`)
	var stderr bytes.Buffer
	if code := runAgentEvent([]string{"--provider", "claude"}, input, io.Discard, &stderr); code != 0 {
		t.Fatalf("runAgentEvent returned %d: %s", code, stderr.String())
	}
	path := filepath.Join(stateHome, "gw", "sessions.json")
	store, err := readSessionStore(path)
	if err != nil {
		t.Fatalf("readSessionStore returned error: %v", err)
	}
	if len(store.Sessions) != 1 || store.Sessions[0].Lifecycle != "active" {
		t.Fatalf("stored sessions = %+v", store.Sessions)
	}
}

func TestJSONResultShape(t *testing.T) {
	var output bytes.Buffer
	result := Result{SchemaVersion: schemaVersion, Sources: Sources{GitHub: "unknown", Agent: "unknown"}}
	if code := writeJSON(&output, result); code != 0 {
		t.Fatal("writeJSON failed")
	}
	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded["schema_version"] != float64(schemaVersion) {
		t.Fatalf("schema_version = %v", decoded["schema_version"])
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

func TestReadSessionStoreMissing(t *testing.T) {
	_, err := readSessionStore(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitHubCacheRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repoPath := t.TempDir()
	want := newGitHubCache(repoPath)
	want.Entries["feature/cache"] = githubCacheEntry{
		State:     GitHubState{Status: "available", PR: &PullRequest{Number: 42, State: "OPEN"}},
		FetchedAt: time.Now().UTC(),
	}
	if err := writeGitHubCache(repoPath, want); err != nil {
		t.Fatalf("writeGitHubCache returned error: %v", err)
	}

	got, err := readGitHubCache(repoPath)
	if err != nil {
		t.Fatalf("readGitHubCache returned error: %v", err)
	}
	if got.Repository != mustAbs(repoPath) || got.Entries["feature/cache"].State.PR.Number != 42 {
		t.Fatalf("cache = %+v", got)
	}
	path, err := githubCachePath(repoPath)
	if err != nil {
		t.Fatalf("githubCachePath returned error: %v", err)
	}
	if mode := fileMode(t, path); mode.Perm() != 0o600 {
		t.Fatalf("GitHub cache mode = %o, want 600", mode.Perm())
	}
}

func TestCollectGitHubStatesUsesCacheAndRefreshes(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repoPath := t.TempDir()
	records := []worktreeRecord{{Path: repoPath, Branch: "feature/cache"}}

	originalLookPath := lookPath
	originalFetcher := githubPRFetcher
	t.Cleanup(func() {
		lookPath = originalLookPath
		githubPRFetcher = originalFetcher
	})
	lookPath = func(string) (string, error) { return "/usr/bin/gh", nil }
	calls := 0
	githubPRFetcher = func(string, string) (GitHubState, error) {
		calls++
		return GitHubState{Status: "available", PR: &PullRequest{Number: calls, State: "OPEN"}}, nil
	}

	states, source, err := collectGitHubStates(repoPath, records, false)
	if err != nil || source != "gh" || calls != 1 || states["feature/cache"].PR.Number != 1 {
		t.Fatalf("initial fetch: states=%+v source=%q calls=%d err=%v", states, source, calls, err)
	}
	states, source, err = collectGitHubStates(repoPath, records, false)
	if err != nil || source != "cache" || calls != 1 || states["feature/cache"].PR.Number != 1 {
		t.Fatalf("cached fetch: states=%+v source=%q calls=%d err=%v", states, source, calls, err)
	}
	states, source, err = collectGitHubStates(repoPath, records, true)
	if err != nil || source != "gh" || calls != 2 || states["feature/cache"].PR.Number != 2 {
		t.Fatalf("forced refresh: states=%+v source=%q calls=%d err=%v", states, source, calls, err)
	}
}

func TestCollectGitHubStatesRefreshesExpiredCache(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repoPath := t.TempDir()
	cache := newGitHubCache(repoPath)
	cache.Entries["feature/expired"] = githubCacheEntry{
		State:     GitHubState{Status: "available", PR: &PullRequest{Number: 1, State: "MERGED"}},
		FetchedAt: time.Now().UTC().Add(-githubCacheTTL - time.Second),
	}
	if err := writeGitHubCache(repoPath, cache); err != nil {
		t.Fatalf("writeGitHubCache returned error: %v", err)
	}

	originalLookPath := lookPath
	originalFetcher := githubPRFetcher
	t.Cleanup(func() {
		lookPath = originalLookPath
		githubPRFetcher = originalFetcher
	})
	lookPath = func(string) (string, error) { return "/usr/bin/gh", nil }
	calls := 0
	githubPRFetcher = func(string, string) (GitHubState, error) {
		calls++
		return GitHubState{Status: "available", PR: &PullRequest{Number: 2, State: "OPEN"}}, nil
	}

	states, source, err := collectGitHubStates(repoPath, []worktreeRecord{{Branch: "feature/expired"}}, false)
	if err != nil || source != "gh" || calls != 1 || states["feature/expired"].PR.Number != 2 {
		t.Fatalf("expired fetch: states=%+v source=%q calls=%d err=%v", states, source, calls, err)
	}
}

func TestCollectGitHubStatesDoesNotOverwriteOnFetchFailure(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repoPath := t.TempDir()
	cache := newGitHubCache(repoPath)
	cache.Entries["feature/failure"] = githubCacheEntry{
		State:     GitHubState{Status: "available", PR: &PullRequest{Number: 7, State: "MERGED"}},
		FetchedAt: time.Now().UTC(),
	}
	if err := writeGitHubCache(repoPath, cache); err != nil {
		t.Fatalf("writeGitHubCache returned error: %v", err)
	}

	originalLookPath := lookPath
	originalFetcher := githubPRFetcher
	t.Cleanup(func() {
		lookPath = originalLookPath
		githubPRFetcher = originalFetcher
	})
	lookPath = func(string) (string, error) { return "/usr/bin/gh", nil }
	githubPRFetcher = func(string, string) (GitHubState, error) {
		return GitHubState{}, errors.New("network unavailable")
	}

	states, source, err := collectGitHubStates(repoPath, []worktreeRecord{{Branch: "feature/failure"}}, true)
	if err == nil || source != "unknown" || states["feature/failure"].Status != "unknown" {
		t.Fatalf("failed fetch: states=%+v source=%q err=%v", states, source, err)
	}
	got, err := readGitHubCache(repoPath)
	if err != nil {
		t.Fatalf("readGitHubCache returned error: %v", err)
	}
	if got.Entries["feature/failure"].State.PR.Number != 7 {
		t.Fatalf("cache was overwritten after failed fetch: %+v", got.Entries["feature/failure"])
	}
}
