package worktree_test

import (
	"testing"

	"github.com/kawaken/gw/git"
	"github.com/kawaken/gw/worktree"
)

const porcelainTwo = `worktree /repo/myapp
HEAD aaa
branch refs/heads/main

worktree /repo/myapp-wt/task-1
HEAD bbb
branch refs/heads/task-1

worktree /repo/myapp-wt/task-2
HEAD ccc
branch refs/heads/task-2
`

func TestSorted(t *testing.T) {
	t.Parallel()

	g := &git.FakeRunner{
		Responses: map[string]string{
			"worktree list --porcelain": porcelainTwo,
			// task-2 is newest (ts=200), task-1 is middle (ts=100), main is oldest (ts=50)
			"log -1 --format=%ct %cr": "50 3 weeks ago",
		},
	}
	// Per-worktree overrides via RunIn (FakeRunner uses Run for all)
	// To test ordering, we use a custom fake that varies by dir.
	gOrdered := &orderedFake{}

	entries, err := worktree.Sorted(gOrdered)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Newest first: task-2 (200), task-1 (100), main (50)
	want := []string{"/repo/myapp-wt/task-2", "/repo/myapp-wt/task-1", "/repo/myapp"}
	for i, e := range entries {
		if e.Path != want[i] {
			t.Errorf("entries[%d].Path = %q, want %q", i, e.Path, want[i])
		}
	}
	_ = g // suppress unused warning
}

func TestSortedMissingLog(t *testing.T) {
	t.Parallel()

	// Worktrees with no commits get timestamp=0 and sort last.
	g := &git.FakeRunner{
		Responses: map[string]string{
			"worktree list --porcelain": `worktree /repo/myapp
HEAD aaa
branch refs/heads/main

worktree /repo/myapp-wt/empty
HEAD 000
branch refs/heads/empty
`,
			"log -1 --format=%ct %cr": "100 1 day ago",
		},
	}
	// empty worktree returns no log output (simulated by returning "" for that path)
	gMixed := &mixedLogFake{
		base: g,
		overrides: map[string]string{
			"/repo/myapp-wt/empty": "",
		},
	}

	entries, err := worktree.Sorted(gMixed)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}
	// main (ts=100) should come before empty (ts=0)
	if entries[0].Path != "/repo/myapp" {
		t.Errorf("expected main first, got %q", entries[0].Path)
	}
	if entries[1].Path != "/repo/myapp-wt/empty" {
		t.Errorf("expected empty last, got %q", entries[1].Path)
	}
}

// orderedFake returns different timestamps per worktree path.
type orderedFake struct{}

func (f *orderedFake) Run(args ...string) (string, error) {
	if len(args) == 3 && args[0] == "worktree" {
		return porcelainTwo, nil
	}
	return "", nil
}

func (f *orderedFake) RunIn(dir string, _ ...string) (string, error) {
	ts := map[string]string{
		"/repo/myapp":           "50 3 weeks ago",
		"/repo/myapp-wt/task-1": "100 1 day ago",
		"/repo/myapp-wt/task-2": "200 2 hours ago",
	}
	if v, ok := ts[dir]; ok {
		return v, nil
	}
	return "", nil
}

func (f *orderedFake) Toplevel() (string, error) { return "/repo/myapp", nil }

// mixedLogFake overrides RunIn for specific paths.
type mixedLogFake struct {
	base      *git.FakeRunner
	overrides map[string]string
}

func (f *mixedLogFake) Run(args ...string) (string, error) { return f.base.Run(args...) }
func (f *mixedLogFake) Toplevel() (string, error)          { return f.base.Toplevel() }
func (f *mixedLogFake) RunIn(dir string, args ...string) (string, error) {
	if v, ok := f.overrides[dir]; ok {
		return v, nil
	}
	return f.base.Run(args...)
}
