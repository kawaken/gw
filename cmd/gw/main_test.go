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
