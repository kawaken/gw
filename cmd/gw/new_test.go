package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRunNewAutomaticName(t *testing.T) {
	repo := newTestRepo(t)
	root := t.TempDir()
	t.Setenv("GW_WORKTREE_ROOT", root)
	t.Chdir(repo)
	code, stdout, stderr := runGW(t, "new", "--json")
	if code != 0 {
		t.Fatalf("gw new returned %d: %s", code, stderr)
	}
	var result NewResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(result.Name, "-")
	if len(parts) != 3 || !slices.Contains(worktreeAdjectives[:], parts[0]) || !slices.Contains(worktreeNouns[:], parts[1]) {
		t.Fatalf("unexpected generated name %q", result.Name)
	}
	if suffix, err := hex.DecodeString(parts[2]); err != nil || len(suffix) != 4 {
		t.Fatalf("unexpected hash suffix %q", parts[2])
	}
	wantPath := resolvePath(filepath.Join(root, "myrepo", result.Name, "myrepo"))
	if !result.Created || result.Branch != result.Name || result.Path != wantPath || result.Base != defaultNewBase {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunNewAutomaticRetry(t *testing.T) {
	const first = "quiet-river-12345678"
	const second = "calm-cedar-abcdef01"
	tests := []struct {
		name      string
		collision string
		branch    string
		base      string
		wantCode  int
		wantCalls int
	}{
		{name: "branch conflict", collision: "branch", wantCalls: 2},
		{name: "path conflict", collision: "path", wantCalls: 2},
		{name: "existing worktree is not reused", collision: "existing", wantCalls: 2},
		{name: "only one retry", collision: "both branches", wantCode: exitNewBranchConflict, wantCalls: 2},
		{name: "only one path retry", collision: "both paths", wantCode: exitNewPathConflict, wantCalls: 2},
		{name: "explicit branch conflict", collision: "branch", branch: first, wantCode: exitNewBranchConflict, wantCalls: 1},
		{name: "explicit branch path conflict", collision: "path", branch: "feature/task", wantCalls: 2},
		{name: "invalid branch", branch: "bad branch", wantCode: exitNewInvalidName, wantCalls: 1},
		{name: "missing base", base: "missing", wantCode: exitNewBaseNotFound, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepo(t)
			root := t.TempDir()
			t.Setenv("GW_WORKTREE_ROOT", root)
			t.Chdir(repo)
			switch tt.collision {
			case "branch", "both branches":
				gitTestCommand(t, repo, "branch", first)
				if tt.collision == "both branches" {
					gitTestCommand(t, repo, "branch", second)
				}
			case "path", "both paths":
				if err := os.MkdirAll(filepath.Join(root, "myrepo", first), 0o700); err != nil {
					t.Fatal(err)
				}
				if tt.collision == "both paths" {
					if err := os.MkdirAll(filepath.Join(root, "myrepo", second), 0o700); err != nil {
						t.Fatal(err)
					}
				}
			case "existing":
				if code, _, stderr := runGW(t, "new", first, "--base", "main"); code != 0 {
					t.Fatalf("setup: exit %d: %s", code, stderr)
				}
			}
			originalName, originalWait := makeWorktreeName, waitForNameRetry
			t.Cleanup(func() {
				makeWorktreeName, waitForNameRetry = originalName, originalWait
			})
			calls, waits := 0, 0
			makeWorktreeName = func() string {
				calls++
				if calls > 2 {
					t.Fatal("more than two attempts")
				}
				if waits != calls-1 {
					t.Fatal("name regenerated before waiting")
				}
				return []string{first, second}[calls-1]
			}
			waitForNameRetry = func(delay time.Duration) {
				if delay != time.Second || calls != 1 || waits != 0 {
					t.Fatalf("unexpected wait: delay=%s, calls=%d, waits=%d", delay, calls, waits)
				}
				waits++
			}
			base := tt.base
			if base == "" {
				base = "main"
			}
			code, stdout, stderr := runGW(t, "new", "--branch", tt.branch, "--base", base, "--json")
			if code != tt.wantCode || calls != tt.wantCalls || waits != tt.wantCalls-1 {
				t.Fatalf("exit=%d calls=%d waits=%d, want exit=%d calls=%d: %s", code, calls, waits, tt.wantCode, tt.wantCalls, stderr)
			}
			if code != 0 {
				if stdout != "" {
					t.Fatalf("failed creation emitted stdout: %s", stdout)
				}
				return
			}
			var result NewResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatal(err)
			}
			wantBranch := second
			if tt.branch != "" {
				wantBranch = tt.branch
			}
			if result.Name != second || result.Branch != wantBranch || !result.Created {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestRunNewFromLinkedWorktree(t *testing.T) {
	repo := newTestRepo(t)
	root := t.TempDir()
	t.Setenv("GW_WORKTREE_ROOT", root)
	t.Chdir(repo)
	code, stdout, stderr := runGW(t, "new", "first", "--base", "main")
	if code != 0 {
		t.Fatalf("first worktree: exit %d: %s", code, stderr)
	}
	t.Chdir(strings.TrimSpace(stdout))
	code, stdout, stderr = runGW(t, "new", "second", "--base", "main")
	want := resolvePath(filepath.Join(root, "myrepo", "second", "myrepo"))
	if code != 0 || strings.TrimSpace(stdout) != want {
		t.Fatalf("from linked worktree: exit %d, stdout=%s, stderr=%s", code, stdout, stderr)
	}
}

func TestLegacyAndNewLayoutCommands(t *testing.T) {
	repo, paths := setupCleanRepo(t, nil, "fresh")
	root := os.Getenv("GW_WORKTREE_ROOT")
	legacyPath := resolvePath(filepath.Join(root, "myrepo", "legacy"))
	gitTestCommand(t, repo, "worktree", "add", "--quiet", "-b", "legacy-branch", legacyPath)
	// Names remain usable after an agent switches branches inside a worktree.
	gitTestCommand(t, paths["fresh"], "switch", "-c", "renamed-branch")
	paths["legacy"] = legacyPath
	code, stdout, stderr := runGW(t, "list", "--json")
	if code != 0 {
		t.Fatalf("list: exit %d: %s", code, stderr)
	}
	var listed Result
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Worktrees) != 3 {
		t.Fatalf("listed worktrees: %+v", listed.Worktrees)
	}
	for _, name := range []string{"fresh", "legacy", ""} {
		args := []string{"inspect", "--json"}
		if name != "" {
			args = append(args, name)
		}
		code, stdout, stderr := runGW(t, args...)
		if code != 0 {
			t.Fatalf("inspect %q: exit %d: %s", name, code, stderr)
		}
		var result Result
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatal(err)
		}
		want := paths[name]
		if name == "" {
			want = resolvePath(repo)
		}
		if len(result.Worktrees) != 1 || result.Worktrees[0].Path != want {
			t.Fatalf("inspect %q: %+v, want %s", name, result.Worktrees, want)
		}
	}
	// An old worktree must not become a container for a new one with the same name.
	if code, _, stderr := runGW(t, "new", "legacy", "--branch", "other", "--base", "main"); code != exitNewPathConflict {
		t.Fatalf("legacy path conflict: exit %d: %s", code, stderr)
	}
	for _, name := range []string{"fresh", "legacy"} {
		code, stdout, stderr := runGW(t, "clean", name, "--force", "--json")
		if code != 0 {
			t.Fatalf("clean %s: exit %d: %s", name, code, stderr)
		}
		report := decodeCleanup(t, stdout)
		if len(report.Removed) != 1 || report.Removed[0].Path != paths[name] {
			t.Fatalf("clean %s: %+v", name, report)
		}
		if _, err := os.Lstat(paths[name]); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("worktree %s remains: %v", name, err)
		}
		if name == "fresh" {
			if _, err := os.Lstat(filepath.Dir(paths[name])); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("name directory remains: %v", err)
			}
			if _, err := os.Stat(legacyPath); err != nil {
				t.Fatalf("legacy sibling was removed: %v", err)
			}
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "myrepo")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty repo directory remains: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("worktree root was removed: %v", err)
	}
}

func TestCleanCustomRootPreservesNonemptyParents(t *testing.T) {
	repo, _ := setupCleanRepo(t, nil)
	root := t.TempDir()
	for _, name := range []string{"empty", "with-sibling"} {
		code, stdout, stderr := runGW(t, "new", name, "--root", root, "--base", "main")
		if code != 0 {
			t.Fatalf("new: exit %d: %s", code, stderr)
		}
		path := strings.TrimSpace(stdout)
		parent := filepath.Dir(path)
		marker := filepath.Join(parent, "keep.txt")
		if name == "with-sibling" {
			if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if code, _, stderr := runGW(t, "clean", path, "--force"); code != 0 {
			t.Fatalf("clean: exit %d: %s", code, stderr)
		}
		if name == "empty" {
			for _, dir := range []string{parent, filepath.Dir(parent)} {
				if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("empty parent remains: %s: %v", dir, err)
				}
			}
		} else if content, err := os.ReadFile(marker); err != nil || string(content) != "keep" {
			t.Fatalf("sibling was removed: %q: %v", content, err)
		}
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("custom root was removed: %v", err)
	}
	if _, err := os.Stat(repo); err != nil {
		t.Fatalf("main worktree was removed: %v", err)
	}
}

func TestMatchTargetNestedLayoutAmbiguity(t *testing.T) {
	records := []worktreeRecord{
		{Path: "/repos/myrepo", Branch: "main"},
		{Path: "/worktrees/myrepo/first/myrepo", Branch: "feature/one"},
		{Path: "/other/myrepo/first/myrepo", Branch: "feature/two"},
	}
	if _, err := matchTarget(records, "first"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous name error: %v", err)
	}
	for _, query := range []string{records[1].Path, "feature/one"} {
		got, err := matchTarget(records, query)
		if err != nil || got.Path != records[1].Path {
			t.Fatalf("match %q: %+v, %v", query, got, err)
		}
	}
}

func TestResolveWorktreeMainAfterSorting(t *testing.T) {
	mainPath := "/repos/myrepo"
	worktrees := []Worktree{
		{Path: "/other/myrepo/task/myrepo", Branch: "feature/task"},
		{Path: mainPath, Branch: "main"},
	}
	for _, query := range []string{"", "main"} {
		got, err := resolveWorktree(worktrees, query, mainPath)
		if err != nil || got.Path != mainPath {
			t.Fatalf("resolve %q: %+v, %v", query, got, err)
		}
	}
}
