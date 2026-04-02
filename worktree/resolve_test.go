package worktree_test

import (
	"testing"

	"github.com/kawaken/gw/git"
	"github.com/kawaken/gw/worktree"
)

const porcelainResolve = `worktree /repo/myapp
HEAD aaa
branch refs/heads/main

worktree /repo/myapp-wt/task-1
HEAD bbb
branch refs/heads/task-1

worktree /repo/myapp-wt/PR-42
HEAD ccc
detached
`

func newResolveRunner() git.Runner {
	return &git.FakeRunner{
		Responses: map[string]string{
			"worktree list --porcelain": porcelainResolve,
		},
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		query   string
		want    string
		wantErr bool
	}{
		{"", "/repo/myapp", false},
		{"main", "/repo/myapp", false},
		{"task-1", "/repo/myapp-wt/task-1", false},
		{"PR-42", "/repo/myapp-wt/PR-42", false},
		{"42", "/repo/myapp-wt/PR-42", false},
		{"#42", "/repo/myapp-wt/PR-42", false},
		{"unknown", "", true},
	}
	g := newResolveRunner()
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			got, err := worktree.Resolve(g, c.query)
			if c.wantErr {
				if err == nil {
					t.Errorf("expected error for query %q, got %q", c.query, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("Resolve(%q) = %q, want %q", c.query, got, c.want)
			}
		})
	}
}
