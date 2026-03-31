package worktree_test

import (
	"testing"

	"github.com/kawaken/gw/worktree"
)

func TestFormatLine(t *testing.T) {
	cases := []struct {
		name           string
		label          string
		branch         string
		age            string
		purpose        string
		originalBranch string
		archived       bool
		wtPath         string
		mode           worktree.FormatMode
		want           string
	}{
		{
			name:   "simple main no extra info",
			label:  "main",
			branch: "[main]",
			want:   "main",
		},
		{
			name:   "task with age",
			label:  "task-1",
			branch: "[task-1]",
			age:    "2 days",
			want:   "task-1 (2 days)",
		},
		{
			name:    "task with purpose",
			label:   "task-1",
			branch:  "[task-1]",
			purpose: "fix auth",
			want:    "task-1\tfix auth",
		},
		{
			name:           "branch differs from label",
			label:          "task-1",
			branch:         "[feature/auth]",
			originalBranch: "feature/auth",
			want:           "task-1",
		},
		{
			name:     "archived",
			label:    "task-1",
			branch:   "[task-1]",
			archived: true,
			want:     "task-1\t[Archived]",
		},
		{
			name:   "verbose shows path",
			label:  "task-1",
			branch: "[task-1]",
			wtPath: "/repo/myapp-wt/task-1",
			mode:   worktree.ModeVerbose,
			want:   "task-1\t/repo/myapp-wt/task-1",
		},
		{
			name:   "path mode",
			label:  "task-1",
			age:    "1 day",
			wtPath: "/repo/myapp-wt/task-1",
			mode:   worktree.ModePath,
			want:   "task-1 (1 day)\t/repo/myapp-wt/task-1",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := worktree.FormatLine(c.label, c.branch, c.age, c.purpose, c.originalBranch, c.archived, c.wtPath, c.mode)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
