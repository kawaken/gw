package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
	base.GitHub = GitHubState{Status: githubStatusFound, PR: &PullRequest{State: "MERGED"}}
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

func TestRemoveWorktreeForceRemovesIgnoredFiles(t *testing.T) {
	repoPath := t.TempDir()
	gitTestCommand(t, repoPath, "init", "--quiet")
	gitTestCommand(t, repoPath, "config", "user.email", "test@example.com")
	gitTestCommand(t, repoPath, "config", "user.name", "Test User")
	gitTestCommand(t, repoPath, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repoPath, ".gitignore"), []byte(".ignored/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, repoPath, "add", ".gitignore")
	gitTestCommand(t, repoPath, "commit", "--quiet", "-m", "initial")

	worktreePath := filepath.Join(t.TempDir(), "worktree")
	gitTestCommand(t, repoPath, "worktree", "add", "--quiet", "-b", "feature", worktreePath)
	ignoredPath := filepath.Join(worktreePath, ".ignored", "build.bin")
	if err := os.MkdirAll(filepath.Dir(ignoredPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignoredPath, []byte("build artifact"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := removeWorktree(repoPath, worktreePath); err != nil {
		t.Fatalf("removeWorktree returned error: %v", err)
	}
	if _, err := os.Lstat(worktreePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worktree path still exists, err = %v", err)
	}
	records, err := gitWorktrees(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Path == worktreePath {
			t.Fatalf("worktree registration still exists for %s", worktreePath)
		}
	}
}

func TestGitRunIncludesGitError(t *testing.T) {
	repoPath := t.TempDir()
	gitTestCommand(t, repoPath, "init", "--quiet")

	err := gitRun(repoPath, "rev-parse", "--verify", "missing-ref")
	if err == nil {
		t.Fatal("gitRun returned nil for a missing ref")
	}
	if !strings.Contains(err.Error(), "fatal:") {
		t.Fatalf("gitRun error = %q, want Git stderr", err)
	}
}

func TestGitOutputFromTimesOut(t *testing.T) {
	repoPath := t.TempDir()
	gitTestCommand(t, repoPath, "init", "--quiet")

	original := execTimeout
	execTimeout = time.Nanosecond
	t.Cleanup(func() { execTimeout = original })

	_, err := gitOutputFrom(repoPath, "status", "--porcelain")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("gitOutputFrom error = %v, want a timeout error", err)
	}
}

func TestGitRunTimesOut(t *testing.T) {
	repoPath := t.TempDir()
	gitTestCommand(t, repoPath, "init", "--quiet")

	original := execTimeout
	execTimeout = time.Nanosecond
	t.Cleanup(func() { execTimeout = original })

	err := gitRun(repoPath, "status")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("gitRun error = %v, want a timeout error", err)
	}
}

func TestGithubPRForBranchTimesOut(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh is not installed")
	}

	original := execTimeout
	execTimeout = time.Nanosecond
	t.Cleanup(func() { execTimeout = original })

	_, err := githubPRForBranch(t.TempDir(), "does-not-matter")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("githubPRForBranch error = %v, want a timeout error", err)
	}
}

// TestCollectResultProcessesWorktreesConcurrently makes several worktrees'
// `git status` hang behind a fake `git` shimmed onto PATH and asserts that
// collectResult finishes in roughly one delay's worth of time rather than
// one delay per hung worktree, i.e. that worktrees are processed
// concurrently rather than one at a time.
func TestCollectResultProcessesWorktreesConcurrently(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake git shim is a POSIX shell script")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	repoPath := t.TempDir()
	gitTestCommand(t, repoPath, "init", "--quiet")
	gitTestCommand(t, repoPath, "config", "user.email", "test@example.com")
	gitTestCommand(t, repoPath, "config", "user.name", "Test User")
	gitTestCommand(t, repoPath, "config", "commit.gpgsign", "false")
	// This test injects its own artificial delay via the fake git shim below;
	// disable fsmonitor at the repo level (not just per gitTestCommand
	// invocation) so a developer machine with core.fsmonitor=true globally
	// configured cannot add an unrelated, real fsmonitor-daemon delay that
	// would make the timing assertion below flaky.
	gitTestCommand(t, repoPath, "config", "core.fsmonitor", "false")
	if err := os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, repoPath, "add", "file.txt")
	gitTestCommand(t, repoPath, "commit", "--quiet", "-m", "initial")

	const numDelayedWorktrees = 3
	var delayedPaths []string
	for i := 0; i < numDelayedWorktrees; i++ {
		wtPath := filepath.Join(t.TempDir(), fmt.Sprintf("delayed-%d", i))
		gitTestCommand(t, repoPath, "worktree", "add", "--quiet", "-b", fmt.Sprintf("delayed-%d", i), wtPath)
		delayedPaths = append(delayedPaths, mustAbs(wtPath))
	}

	fakeBinDir := t.TempDir()
	markerPath := filepath.Join(fakeBinDir, "delayed-dirs.txt")
	if err := os.WriteFile(markerPath, []byte(strings.Join(delayedPaths, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const delay = 300 * time.Millisecond
	sleepSeconds := fmt.Sprintf("%.3f", delay.Seconds())
	script := "#!/bin/sh\n" +
		"dir=$(pwd)\n" +
		"if [ \"$1\" = \"status\" ] && grep -qxF \"$dir\" \"" + markerPath + "\"; then sleep " + sleepSeconds + "; fi\n" +
		"exec \"" + realGit + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(fakeBinDir, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	originalLookPath := lookPath
	t.Cleanup(func() { lookPath = originalLookPath })
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound } // skip gh entirely

	t.Chdir(repoPath)
	start := time.Now()
	result, err := collectResult(false)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("collectResult returned error: %v", err)
	}
	if len(result.Worktrees) != numDelayedWorktrees+1 {
		t.Fatalf("got %d worktrees, want %d", len(result.Worktrees), numDelayedWorktrees+1)
	}
	// Serial processing would take at least numDelayedWorktrees*delay; give
	// generous slack above a single delay to keep this from being flaky.
	if elapsed >= delay*time.Duration(numDelayedWorktrees) {
		t.Fatalf("collectResult took %s, worktrees do not appear to have been processed concurrently", elapsed)
	}
}

func gitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitArgs := append([]string{"-c", "core.fsmonitor=false"}, args...)
	cmd := exec.Command("git", gitArgs...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(gitArgs, " "), err, output)
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
		State:     GitHubState{Status: githubStatusFound, PR: &PullRequest{Number: 42, State: "OPEN"}},
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
		return GitHubState{Status: githubStatusFound, PR: &PullRequest{Number: calls, State: "OPEN"}}, nil
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

func TestCollectGitHubStatesReportsNotFoundAndUnknown(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repoPath := t.TempDir()

	originalLookPath := lookPath
	originalFetcher := githubPRFetcher
	t.Cleanup(func() {
		lookPath = originalLookPath
		githubPRFetcher = originalFetcher
	})
	lookPath = func(string) (string, error) { return "/usr/bin/gh", nil }
	githubPRFetcher = func(string, string) (GitHubState, error) {
		return GitHubState{Status: githubStatusNotFound}, nil
	}

	states, source, err := collectGitHubStates(repoPath, []worktreeRecord{
		{Branch: "feature/no-pr"},
		{Detached: true},
	}, false)
	if err != nil {
		t.Fatalf("collectGitHubStates returned error: %v", err)
	}
	if source != "gh" {
		t.Fatalf("source = %q, want gh", source)
	}
	if states["feature/no-pr"].Status != githubStatusNotFound {
		t.Fatalf("not-found status = %q, want %q", states["feature/no-pr"].Status, githubStatusNotFound)
	}
	if states[""].Status != githubStatusUnknown {
		t.Fatalf("detached status = %q, want %q", states[""].Status, githubStatusUnknown)
	}
}

func TestCollectGitHubStatesRefreshesExpiredCache(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repoPath := t.TempDir()
	cache := newGitHubCache(repoPath)
	cache.Entries["feature/expired"] = githubCacheEntry{
		State:     GitHubState{Status: githubStatusFound, PR: &PullRequest{Number: 1, State: "MERGED"}},
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
		return GitHubState{Status: githubStatusFound, PR: &PullRequest{Number: 2, State: "OPEN"}}, nil
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
		State:     GitHubState{Status: githubStatusFound, PR: &PullRequest{Number: 7, State: "MERGED"}},
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
	if err == nil || source != "unknown" || states["feature/failure"].Status != githubStatusUnavailable {
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

func TestNormalizeGitHubStateMigratesLegacyStatus(t *testing.T) {
	withoutPR := normalizeGitHubState(GitHubState{Status: "available"})
	if withoutPR.Status != githubStatusNotFound {
		t.Fatalf("status without PR = %q, want %q", withoutPR.Status, githubStatusNotFound)
	}

	withPR := normalizeGitHubState(GitHubState{
		Status: "available",
		PR:     &PullRequest{Number: 42},
	})
	if withPR.Status != githubStatusFound {
		t.Fatalf("status with PR = %q, want %q", withPR.Status, githubStatusFound)
	}
}

func TestGitHubCacheMigratesLegacyVersion(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repoPath := t.TempDir()
	want := githubCache{
		Version:    1,
		Repository: repoPath,
		Entries: map[string]githubCacheEntry{
			"feature/no-pr": {State: GitHubState{Status: "available"}, FetchedAt: time.Now().UTC()},
		},
	}
	if err := writeGitHubCache(repoPath, want); err != nil {
		t.Fatalf("writeGitHubCache returned error: %v", err)
	}

	got, err := readGitHubCache(repoPath)
	if err != nil {
		t.Fatalf("readGitHubCache returned error: %v", err)
	}
	if got.Version != githubCacheVersion {
		t.Fatalf("cache version = %d, want %d", got.Version, githubCacheVersion)
	}
	if got.Entries["feature/no-pr"].State.Status != githubStatusNotFound {
		t.Fatalf("migrated status = %q, want %q", got.Entries["feature/no-pr"].State.Status, githubStatusNotFound)
	}
}

func TestCleanupForGitHubStatuses(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		recommend string
		reason    string
	}{
		{name: "unknown", status: githubStatusUnknown, recommend: "review", reason: "github_state_unknown"},
		{name: "unavailable", status: githubStatusUnavailable, recommend: "review", reason: "github_state_unavailable"},
		{name: "not found", status: githubStatusNotFound, recommend: "review", reason: "no_pull_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanupFor(Worktree{
				Git:    GitState{Clean: true},
				Agent:  AgentState{Lifecycle: "ended"},
				GitHub: GitHubState{Status: tt.status},
			}, false)
			if got.Recommendation != tt.recommend {
				t.Fatalf("recommendation = %q, want %q", got.Recommendation, tt.recommend)
			}
			if len(got.Reasons) != 1 || got.Reasons[0] != tt.reason {
				t.Fatalf("reasons = %v, want [%q]", got.Reasons, tt.reason)
			}
		})
	}
}

// newTestRepo creates a repository whose main branch is pushed to a bare
// "origin", so origin/main exists like in a normal clone.
func newTestRepo(t *testing.T) string {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin.git")
	gitTestCommand(t, filepath.Dir(origin), "init", "--quiet", "--bare", origin)
	repoPath := filepath.Join(t.TempDir(), "myrepo")
	if err := os.Mkdir(repoPath, 0o700); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, repoPath, "init", "--quiet", "-b", "main")
	gitTestCommand(t, repoPath, "config", "user.email", "test@example.com")
	gitTestCommand(t, repoPath, "config", "user.name", "Test User")
	gitTestCommand(t, repoPath, "config", "commit.gpgsign", "false")
	gitTestCommand(t, repoPath, "config", "core.fsmonitor", "false")
	if err := os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, repoPath, "add", "file.txt")
	gitTestCommand(t, repoPath, "commit", "--quiet", "-m", "initial")
	gitTestCommand(t, repoPath, "remote", "add", "origin", origin)
	gitTestCommand(t, repoPath, "push", "--quiet", "origin", "main")
	return repoPath
}

func runGW(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunNewCreatesWorktreeUnderRoot(t *testing.T) {
	repoPath := newTestRepo(t)
	root := t.TempDir()
	t.Setenv("GW_WORKTREE_ROOT", root)
	t.Chdir(repoPath)

	code, stdout, stderr := runGW(t, "new", "fix-a")
	if code != 0 {
		t.Fatalf("gw new returned %d: %s", code, stderr)
	}
	want := resolvePath(filepath.Join(root, "myrepo", "fix-a"))
	if strings.TrimSpace(stdout) != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	records, err := gitWorktrees(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		if record.Path == want && record.Branch == "fix-a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("worktree %s on branch fix-a not registered: %+v", want, records)
	}
	if _, err := gitOutputFrom(want, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		t.Fatal("new branch tracks the base; want no upstream")
	}

	// A second call with the same name returns the existing worktree.
	code, stdout, stderr = runGW(t, "new", "fix-a", "--json")
	if code != 0 {
		t.Fatalf("second gw new returned %d: %s", code, stderr)
	}
	var result NewResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if result.Created || result.Path != want || result.Branch != "fix-a" || result.SchemaVersion != schemaVersion {
		t.Fatalf("second gw new result = %+v", result)
	}

	// Removing the last worktree also removes the empty <repo> directory.
	if err := removeWorktree(repoPath, want); err != nil {
		t.Fatalf("removeWorktree returned error: %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(want)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repo directory still exists, err = %v", err)
	}
}

func TestRunNewJSONWithBranchAndBase(t *testing.T) {
	repoPath := newTestRepo(t)
	root := t.TempDir()
	t.Chdir(repoPath)

	code, stdout, stderr := runGW(t, "new", "--root", root, "--branch", "feature/x", "--base", "main", "task-1", "--json")
	if code != 0 {
		t.Fatalf("gw new returned %d: %s", code, stderr)
	}
	var result NewResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	want := NewResult{SchemaVersion: schemaVersion, Path: resolvePath(filepath.Join(root, "myrepo", "task-1")), Branch: "feature/x", Base: "main", Created: true}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
}

func TestRunNewDefaultRootUsesXDGDataHome(t *testing.T) {
	repoPath := newTestRepo(t)
	dataHome := t.TempDir()
	t.Setenv("GW_WORKTREE_ROOT", "")
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Chdir(repoPath)

	code, stdout, stderr := runGW(t, "new", "fix-b")
	if code != 0 {
		t.Fatalf("gw new returned %d: %s", code, stderr)
	}
	want := resolvePath(filepath.Join(dataHome, "gw", "worktrees", "myrepo", "fix-b"))
	if strings.TrimSpace(stdout) != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunNewFailures(t *testing.T) {
	repoPath := newTestRepo(t)
	root := t.TempDir()
	t.Setenv("GW_WORKTREE_ROOT", root)
	t.Chdir(repoPath)
	gitTestCommand(t, repoPath, "branch", "existing")
	if code, _, stderr := runGW(t, "new", "taken"); code != 0 {
		t.Fatalf("setup gw new returned %d: %s", code, stderr)
	}
	if err := os.MkdirAll(filepath.Join(root, "myrepo", "occupied"), 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		code int
	}{
		{name: "no name", args: []string{"new"}, code: 2},
		{name: "slash", args: []string{"new", "a/b"}, code: exitNewInvalidName},
		{name: "dot dot", args: []string{"new", "a..b"}, code: exitNewInvalidName},
		{name: "invalid branch", args: []string{"new", "ok", "--branch", "bad branch"}, code: exitNewInvalidName},
		{name: "missing base", args: []string{"new", "fresh", "--base", "origin/nope"}, code: exitNewBaseNotFound},
		{name: "existing branch", args: []string{"new", "existing"}, code: exitNewBranchConflict},
		{name: "branch checked out elsewhere", args: []string{"new", "other", "--branch", "taken"}, code: exitNewBranchConflict},
		{name: "same path, other branch", args: []string{"new", "taken", "--branch", "different"}, code: exitNewPathConflict},
		{name: "path exists", args: []string{"new", "occupied"}, code: exitNewPathConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := runGW(t, tt.args...)
			if code != tt.code {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, tt.code, stderr)
			}
		})
	}
}

func TestRunAgentEventManaged(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "1", want: "managed"},
		{value: "true", want: "managed"},
		{value: "0", want: "active"},
		{value: "", want: "active"},
	}
	for _, tt := range tests {
		t.Run("GW_MANAGED="+tt.value, func(t *testing.T) {
			stateHome := t.TempDir()
			t.Setenv("XDG_STATE_HOME", stateHome)
			t.Setenv("GW_MANAGED", tt.value)
			input := strings.NewReader(`{"session_id":"session-1","cwd":"/tmp/worktree","hook_event_name":"SessionStart"}`)
			var stderr bytes.Buffer
			if code := runAgentEvent([]string{"--provider", "codex"}, input, io.Discard, &stderr); code != 0 {
				t.Fatalf("runAgentEvent returned %d: %s", code, stderr.String())
			}
			store, err := readSessionStore(filepath.Join(stateHome, "gw", "sessions.json"))
			if err != nil {
				t.Fatal(err)
			}
			got := store.Sessions[0]
			if got.Lifecycle != tt.want || got.SessionID != "session-1" || got.CWD != "/tmp/worktree" || got.Event != "SessionStart" {
				t.Fatalf("stored session = %+v, want lifecycle %q", got, tt.want)
			}
		})
	}
}

func TestCleanupForManagedAndLock(t *testing.T) {
	merged := Worktree{
		Git:    GitState{Clean: true},
		Agent:  AgentState{Lifecycle: "managed"},
		GitHub: GitHubState{Status: githubStatusFound, PR: &PullRequest{State: "MERGED"}},
	}
	if got := cleanupFor(merged, false); got.Recommendation != "recommended" {
		t.Fatalf("managed lifecycle recommendation = %+v, want recommended", got)
	}

	locked := merged
	locked.Lock = &LockState{PID: 123, Alive: true}
	if got := cleanupFor(locked, false); got.Recommendation != "keep" || got.Reasons[0] != "agent_process_running" {
		t.Fatalf("live lock cleanup = %+v, want keep/agent_process_running", got)
	}
	locked.Git.Clean = false
	if got := cleanupFor(locked, false); got.Recommendation != "keep" {
		t.Fatalf("live lock on dirty worktree cleanup = %+v, want keep", got)
	}

	stale := merged
	stale.Lock = &LockState{PID: 123, Alive: false}
	if got := cleanupFor(stale, false); got.Recommendation != "recommended" {
		t.Fatalf("stale lock cleanup = %+v, want recommended", got)
	}
}

// deadPID returns the PID of a child process that has exited and been reaped.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot run true: %v", err)
	}
	return cmd.Process.Pid
}

// runningPID starts a child process that stays alive until the test ends.
func runningPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot run sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

func TestPIDAlive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX commands")
	}
	if !pidAlive(os.Getpid()) {
		t.Fatal("current process reported as not running")
	}
	if pidAlive(deadPID(t)) {
		t.Fatal("exited process reported as running")
	}
	if pidAlive(0) || pidAlive(-1) {
		t.Fatal("invalid PID reported as running")
	}
}

func TestRunLockAndUnlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX commands")
	}
	repoPath := newTestRepo(t)
	t.Setenv("GW_WORKTREE_ROOT", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Chdir(repoPath)
	code, stdout, stderr := runGW(t, "new", "task")
	if code != 0 {
		t.Fatalf("gw new returned %d: %s", code, stderr)
	}
	wtPath := strings.TrimSpace(stdout)
	self := strconv.Itoa(os.Getpid())

	if code, _, stderr := runGW(t, "lock", "task", "--pid", strconv.Itoa(deadPID(t))); code != 1 {
		t.Fatalf("lock with exited PID returned %d, want 1: %s", code, stderr)
	}
	if lockStateFor(wtPath) != nil {
		t.Fatal("lock with exited PID was recorded")
	}

	// By branch name.
	if code, _, stderr := runGW(t, "lock", "task", "--pid", self); code != 0 {
		t.Fatalf("lock returned %d: %s", code, stderr)
	}
	if got := lockStateFor(wtPath); got == nil || !got.Alive || got.PID != os.Getpid() {
		t.Fatalf("lock state = %+v", got)
	}
	// Re-locking with the same PID is allowed; another running PID is not.
	if code, _, stderr := runGW(t, "lock", wtPath, "--pid", self); code != 0 {
		t.Fatalf("re-lock with same PID returned %d: %s", code, stderr)
	}
	other := strconv.Itoa(runningPID(t))
	if code, _, _ := runGW(t, "lock", wtPath, "--pid", other); code != 1 {
		t.Fatalf("lock by another running PID returned %d, want 1", code)
	}

	// The lock keeps the worktree out of cleanup.
	result, err := collectResult(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, wt := range result.Worktrees {
		if wt.Path == wtPath && (wt.Cleanup.Recommendation != "keep" || wt.Cleanup.Reasons[0] != "agent_process_running") {
			t.Fatalf("locked worktree cleanup = %+v", wt.Cleanup)
		}
	}

	if code, _, stderr := runGW(t, "unlock", "task"); code != 0 {
		t.Fatalf("unlock returned %d: %s", code, stderr)
	}
	if lockStateFor(wtPath) != nil {
		t.Fatal("lock remains after unlock")
	}
	if code, _, stderr := runGW(t, "unlock", "task"); code != 0 {
		t.Fatalf("second unlock returned %d: %s", code, stderr)
	}

	// A lock whose process has exited is stale and can be replaced.
	if err := writeLock(lockRecord{Path: wtPath, PID: deadPID(t), LockedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if got := lockStateFor(wtPath); got == nil || got.Alive {
		t.Fatalf("stale lock state = %+v, want alive=false", got)
	}
	if code, _, stderr := runGW(t, "lock", "task", "--pid", other); code != 0 {
		t.Fatalf("lock over stale lock returned %d: %s", code, stderr)
	}

	if code, _, _ := runGW(t, "lock", "task"); code != 2 {
		t.Fatalf("lock without --pid returned %d, want 2", code)
	}
	if code, _, _ := runGW(t, "lock", "missing", "--pid", self); code != 1 {
		t.Fatalf("lock on unknown worktree returned %d, want 1", code)
	}
}

func TestMatchTarget(t *testing.T) {
	records := []worktreeRecord{
		{Path: "/tmp/repo", Branch: "main"},
		{Path: "/tmp/a/fix", Branch: "fix-1"},
		{Path: "/tmp/b/fix", Branch: "fix-2"},
		{Path: "/tmp/b/other", Branch: "feature/x"},
	}
	tests := []struct {
		query string
		want  string
	}{
		{query: "/tmp/a/fix", want: "/tmp/a/fix"},
		{query: "feature/x", want: "/tmp/b/other"},
		{query: "other", want: "/tmp/b/other"},
		{query: "main", want: "/tmp/repo"},
	}
	for _, tt := range tests {
		got, err := matchTarget(records, tt.query)
		if err != nil || got.Path != tt.want {
			t.Fatalf("matchTarget(%q) = %+v, %v; want %s", tt.query, got, err, tt.want)
		}
	}
	if _, err := matchTarget(records, "fix"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous directory name error = %v", err)
	}
	if _, err := matchTarget(records, "nope"); err == nil {
		t.Fatal("unknown worktree matched")
	}
}
