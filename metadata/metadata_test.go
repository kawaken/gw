package metadata_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kawaken/gw/metadata"
)

func setupLinkedWorktree(t *testing.T, adminDirName, wtPath string) (mainRepo string) {
	t.Helper()
	dir := t.TempDir()
	mainRepo = filepath.Join(dir, "myapp")
	adminDir := filepath.Join(mainRepo, ".git", "worktrees", adminDirName)

	if err := os.MkdirAll(adminDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtPath, 0o750); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(wtPath, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+adminDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return mainRepo
}

func TestGetSet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	wt := filepath.Join(dir, "myapp-wt", "mytask")
	_ = setupLinkedWorktree(t, "mytask", wt)

	got, err := metadata.Get(wt, "purpose")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	if err := metadata.Set(wt, "purpose", "fix auth bug"); err != nil {
		t.Fatal(err)
	}
	got, err = metadata.Get(wt, "purpose")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fix auth bug" {
		t.Errorf("got %q, want %q", got, "fix auth bug")
	}

	if err := metadata.Set(wt, "purpose", "new purpose"); err != nil {
		t.Fatal(err)
	}
	got, err = metadata.Get(wt, "purpose")
	if err != nil {
		t.Fatal(err)
	}
	if got != "new purpose" {
		t.Errorf("got %q, want %q", got, "new purpose")
	}

	if err := metadata.Set(wt, "archived", "true"); err != nil {
		t.Fatal(err)
	}
	got, err = metadata.Get(wt, "purpose")
	if err != nil {
		t.Fatal(err)
	}
	if got != "new purpose" {
		t.Errorf("purpose changed: %q", got)
	}
	got, err = metadata.Get(wt, "archived")
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Errorf("archived: %q", got)
	}
}

func TestGetAll(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	wt := filepath.Join(dir, "myapp-wt", "mytask")
	_ = setupLinkedWorktree(t, "mytask", wt)

	if err := metadata.Set(wt, "purpose", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := metadata.Set(wt, "archived", "true"); err != nil {
		t.Fatal(err)
	}

	m, err := metadata.GetAll(wt)
	if err != nil {
		t.Fatal(err)
	}
	if m["purpose"] != "hello" {
		t.Errorf("purpose: %q", m["purpose"])
	}
	if m["archived"] != "true" {
		t.Errorf("archived: %q", m["archived"])
	}
}

func TestUsesGitdirWhenAdminDirDiffersFromBasename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	wt := filepath.Join(dir, "b", "same")
	main := setupLinkedWorktree(t, "same1", wt)

	if err := metadata.Set(wt, "purpose", "collision-safe"); err != nil {
		t.Fatal(err)
	}

	got, err := metadata.Get(wt, "purpose")
	if err != nil {
		t.Fatal(err)
	}
	if got != "collision-safe" {
		t.Fatalf("got %q, want %q", got, "collision-safe")
	}

	data, err := os.ReadFile(filepath.Join(main, ".git", "worktrees", "same1", "gw_metadata"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "purpose=collision-safe\n" {
		t.Fatalf("unexpected metadata content: %q", string(data))
	}

	if _, err := os.Stat(filepath.Join(main, ".git", "worktrees", "same", "gw_metadata")); !os.IsNotExist(err) {
		t.Fatalf("unexpected legacy metadata file created: %v", err)
	}
}

func TestGetAllReturnsErrorWhenAdminDirCannotBeResolved(t *testing.T) {
	t.Parallel()

	wt := filepath.Join(t.TempDir(), "broken-worktree")
	if err := os.MkdirAll(wt, 0o750); err != nil {
		t.Fatal(err)
	}

	if _, err := metadata.GetAll(wt); err == nil {
		t.Fatal("expected error, got nil")
	}
}
