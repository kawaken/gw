package worktree_test

import (
	"testing"

	"github.com/kawaken/gw/worktree"
)

func TestFormatLine(t *testing.T) {
	cases := []struct {
		name string
		info worktree.FormatInfo
		mode worktree.FormatMode
		want string
	}{
		{
			name: "simple main no extra info",
			info: worktree.FormatInfo{Label: "main", Branch: "[main]"},
			want: "main",
		},
		{
			name: "task with age",
			info: worktree.FormatInfo{Label: "task-1", Branch: "[task-1]", Age: "2 days"},
			want: "task-1 (2 days)",
		},
		{
			name: "task with purpose",
			info: worktree.FormatInfo{Label: "task-1", Branch: "[task-1]", Purpose: "fix auth"},
			want: "task-1\tfix auth",
		},
		{
			name: "branch differs from label",
			info: worktree.FormatInfo{Label: "task-1", Branch: "[feature/auth]", OriginalBranch: "feature/auth"},
			want: "task-1",
		},
		{
			name: "archived",
			info: worktree.FormatInfo{Label: "task-1", Branch: "[task-1]", Archived: true},
			want: "task-1\t[Archived]",
		},
		{
			name: "verbose shows path",
			info: worktree.FormatInfo{Label: "task-1", Branch: "[task-1]", Path: "/repo/myapp-wt/task-1"},
			mode: worktree.ModeVerbose,
			want: "task-1\t/repo/myapp-wt/task-1",
		},
		{
			name: "path mode",
			info: worktree.FormatInfo{Label: "task-1", Age: "1 day", Path: "/repo/myapp-wt/task-1"},
			mode: worktree.ModePath,
			want: "task-1 (1 day)\t/repo/myapp-wt/task-1",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := worktree.FormatLine(c.info, c.mode)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
