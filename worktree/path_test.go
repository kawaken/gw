package worktree_test

import (
	"testing"

	"github.com/kawaken/gw/worktree"
)

func TestShortenName(t *testing.T) {
	// Short names pass through unchanged.
	if got := worktree.ShortenName("task-123"); got != "task-123" {
		t.Errorf("ShortenName(short) = %q, want %q", got, "task-123")
	}
	if got := worktree.ShortenName("feature/auth"); got != "feature-auth" {
		t.Errorf("ShortenName(slash) = %q, want %q", got, "feature-auth")
	}

	// Exactly 20 chars: no truncation.
	exact20 := "exactly-twenty-char1"
	if got := worktree.ShortenName(exact20); got != exact20 {
		t.Errorf("ShortenName(20-char) = %q, want %q", got, exact20)
	}

	// Long names: result is always ≤20 chars.
	long := "a-very-long-task-name-that-exceeds-20-chars"
	got := worktree.ShortenName(long)
	if len(got) != 20 {
		t.Errorf("ShortenName(long) len=%d, want 20; got %q", len(got), got)
	}

	// Stability: same input always produces the same output.
	if worktree.ShortenName(long) != got {
		t.Error("ShortenName is not stable")
	}

	// Collision avoidance: names differing only after 12 chars get distinct results.
	a := worktree.ShortenName("feature-my-long-name-A")
	b := worktree.ShortenName("feature-my-long-name-B")
	if a == b {
		t.Errorf("ShortenName collision: %q and %q both produce %q", "feature-my-long-name-A", "feature-my-long-name-B", a)
	}
}

func TestMakeLabel(t *testing.T) {
	cases := []struct {
		wtPath   string
		mainPath string
		want     string
	}{
		{"/repo/myapp", "/repo/myapp", "main"},
		{"/repo/myapp-wt/task-1", "/repo/myapp", "task-1"},
		{"/other/dir/task-1", "/repo/myapp", "task-1"},
	}
	for _, c := range cases {
		got := worktree.MakeLabel(c.wtPath, c.mainPath)
		if got != c.want {
			t.Errorf("MakeLabel(%q, %q) = %q, want %q", c.wtPath, c.mainPath, got, c.want)
		}
	}
}

func TestPath(t *testing.T) {
	cases := []struct {
		task string
		want string
	}{
		{"", "/repo/myapp"},
		{"main", "/repo/myapp"},
		{"task-1", "/repo/myapp-wt/task-1"},
		{"feature/auth", "/repo/myapp-wt/feature-auth"},
	}
	for _, c := range cases {
		got := worktree.Path("/repo/myapp", "", c.task)
		if got != c.want {
			t.Errorf("Path(%q) = %q, want %q", c.task, got, c.want)
		}
	}
}
