package worktree_test

import (
	"testing"

	"github.com/kawaken/gw/worktree"
)

func TestShortenName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"task-123", "task-123"},
		{"feature/auth", "feature-auth"},
		{"a-very-long-task-name-that-exceeds-20-chars", "a-very-long-task-n__"}, // [:18]+"__"
		{"exactly-twenty-char1", "exactly-twenty-char1"},                        // 20 chars: no truncation
		{"exactly-twenty-chars!", "exactly-twenty-cha__"},                       // 21 chars: truncate
	}
	for _, c := range cases {
		got := worktree.ShortenName(c.in)
		if got != c.want {
			t.Errorf("ShortenName(%q) = %q, want %q", c.in, got, c.want)
		}
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
