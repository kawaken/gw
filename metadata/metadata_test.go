package metadata_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kawaken/gw/metadata"
)

func setup(t *testing.T) (mainRepo, wtPath string) {
	t.Helper()
	dir := t.TempDir()
	// Simulate: mainRepo/.git/worktrees/mytask/
	mainRepo = filepath.Join(dir, "myapp")
	wtPath = filepath.Join(dir, "myapp-wt", "mytask")
	gitWorktreesDir := filepath.Join(mainRepo, ".git", "worktrees", "mytask")
	if err := os.MkdirAll(gitWorktreesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return
}

func TestGetSet(t *testing.T) {
	main, wt := setup(t)

	// Get on missing key returns ""
	if got := metadata.Get(main, wt, "purpose"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	// Set and Get
	if err := metadata.Set(main, wt, "purpose", "fix auth bug"); err != nil {
		t.Fatal(err)
	}
	if got := metadata.Get(main, wt, "purpose"); got != "fix auth bug" {
		t.Errorf("got %q, want %q", got, "fix auth bug")
	}

	// Overwrite
	if err := metadata.Set(main, wt, "purpose", "new purpose"); err != nil {
		t.Fatal(err)
	}
	if got := metadata.Get(main, wt, "purpose"); got != "new purpose" {
		t.Errorf("got %q, want %q", got, "new purpose")
	}

	// Other keys untouched
	if err := metadata.Set(main, wt, "archived", "true"); err != nil {
		t.Fatal(err)
	}
	if got := metadata.Get(main, wt, "purpose"); got != "new purpose" {
		t.Errorf("purpose changed: %q", got)
	}
	if got := metadata.Get(main, wt, "archived"); got != "true" {
		t.Errorf("archived: %q", got)
	}
}

func TestGetAll(t *testing.T) {
	main, wt := setup(t)
	if err := metadata.Set(main, wt, "purpose", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := metadata.Set(main, wt, "archived", "true"); err != nil {
		t.Fatal(err)
	}

	m := metadata.GetAll(main, wt)
	if m["purpose"] != "hello" {
		t.Errorf("purpose: %q", m["purpose"])
	}
	if m["archived"] != "true" {
		t.Errorf("archived: %q", m["archived"])
	}
}
