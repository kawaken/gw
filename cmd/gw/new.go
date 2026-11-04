package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Exit codes for gw new, so callers can tell failures apart without parsing
// stderr. 1 is a general error and 2 is a usage error, as in other commands.
const (
	exitNewInvalidName    = 3
	exitNewBaseNotFound   = 4
	exitNewBranchConflict = 5
	exitNewPathConflict   = 6
)

const defaultNewBase = "origin/main"

// longExecTimeout bounds git calls that touch the network or write a whole
// checkout (fetch, worktree add), which can legitimately take longer than
// execTimeout.
var longExecTimeout = 2 * time.Minute

var (
	makeWorktreeName = randomWorktreeName
	waitForNameRetry = time.Sleep
)

type NewResult struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	Branch        string `json:"branch"`
	Base          string `json:"base,omitempty"`
	Created       bool   `json:"created"`
}

// newError carries the exit code for a gw new failure.
type newError struct {
	code int
	err  error
}

func (e *newError) Error() string { return e.err.Error() }

func newFailure(code int, format string, args ...any) error {
	return &newError{code: code, err: fmt.Errorf(format, args...)}
}

func runNew(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", defaultNewBase, "ref to create the branch from")
	branch := fs.String("branch", "", "branch name (default: the worktree name)")
	root := fs.String("root", "", "directory that holds gw worktrees (default: $GW_WORKTREE_ROOT or ${XDG_DATA_HOME:-~/.local/share}/gw/worktrees)")
	jsonOutput := fs.Bool("json", false, "output JSON")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) > 1 {
		fmt.Fprintln(stderr, "usage: gw new [<name>] [--base <ref>] [--branch <branch>] [--root <dir>] [--json]")
		return 2
	}

	var result NewResult
	if len(positional) == 0 {
		result, err = createAutoWorktree(*branch, *base, *root, stderr)
	} else {
		result, err = createWorktree(positional[0], *branch, *base, *root, stderr)
	}
	if err != nil {
		fmt.Fprintf(stderr, "gw new: %v\n", err)
		var ne *newError
		if errors.As(err, &ne) {
			return ne.code
		}
		return 1
	}
	if *jsonOutput {
		return writeJSON(stdout, result)
	}
	fmt.Fprintln(stdout, result.Path)
	return 0
}

func createAutoWorktree(branch, base, root string, stderr io.Writer) (NewResult, error) {
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			waitForNameRetry(time.Second)
		}
		result, err := createWorktree(makeWorktreeName(), branch, base, root, stderr)
		if err == nil {
			if result.Created {
				return result, nil
			}
			// A generated name must never reuse another caller's worktree.
			err = newFailure(exitNewPathConflict, "%s already exists as a worktree", result.Path)
		}
		var ne *newError
		if attempt == 1 || !errors.As(err, &ne) ||
			(ne.code != exitNewPathConflict && (ne.code != exitNewBranchConflict || branch != "")) {
			return NewResult{}, err
		}
	}
}

func randomWorktreeName() string {
	adjective := worktreeAdjectives[rand.IntN(len(worktreeAdjectives))]
	noun := worktreeNouns[rand.IntN(len(worktreeNouns))]
	words := adjective + "-" + noun
	digest := sha256.Sum256(fmt.Appendf(nil, "%s-%d", words, time.Now().UnixNano()))
	return fmt.Sprintf("%s-%x", words, digest[:4])
}

var worktreeAdjectives = [...]string{
	"amber", "ancient", "autumn", "azure", "bold", "brave", "bright", "brisk",
	"calm", "clever", "cobalt", "cool", "dusky", "cosmic", "crisp", "curious",
	"dawn", "deep", "eager", "early", "fair", "fancy", "fresh", "gentle",
	"golden", "grand", "green", "happy", "hidden", "humble", "ivory", "jolly",
	"keen", "kind", "late", "light", "lively", "lucky", "lunar", "mellow",
	"merry", "misty", "noble", "olive", "pale", "patient", "plain", "polar",
	"proud", "quick", "quiet", "rapid", "rustic", "silent", "silver", "simple",
	"smooth", "solar", "spare", "steady", "sunny", "swift", "tidy", "warm",
}

var worktreeNouns = [...]string{
	"anchor", "aspen", "badger", "beacon", "birch", "breeze", "brook", "canyon",
	"cedar", "cliff", "cloud", "comet", "coral", "crane", "creek", "delta",
	"dune", "ember", "falcon", "fern", "field", "finch", "fjord", "forest",
	"fox", "garden", "glacier", "grove", "harbor", "hawk", "heron", "hill",
	"island", "lake", "lantern", "maple", "meadow", "mesa", "moon", "moss",
	"oak", "orchid", "otter", "owl", "pebble", "pine", "planet", "pond",
	"prairie", "rain", "raven", "reef", "ridge", "river", "robin", "sparrow",
	"spruce", "star", "stone", "summit", "thistle", "tide", "valley", "willow",
}

func createWorktree(name, branch, base, root string, stderr io.Writer) (NewResult, error) {
	if err := validateWorktreeName(name); err != nil {
		return NewResult{}, &newError{code: exitNewInvalidName, err: err}
	}
	if branch == "" {
		branch = name
	}
	if base == "" {
		base = defaultNewBase
	}

	cwdRepo, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return NewResult{}, fmt.Errorf("not inside a Git repository: %w", err)
	}
	cwdRepo = mustAbs(strings.TrimSpace(cwdRepo))
	if err := gitRun(cwdRepo, "check-ref-format", "--branch", branch); err != nil {
		return NewResult{}, newFailure(exitNewInvalidName, "invalid branch name %q", branch)
	}
	records, err := gitWorktrees(cwdRepo)
	if err != nil {
		return NewResult{}, err
	}
	if len(records) == 0 {
		return NewResult{}, errors.New("no worktrees found")
	}
	// The first entry of `git worktree list` is the main worktree, so the
	// repository name is the same no matter which worktree gw runs from.
	mainPath := records[0].Path
	rootDir, err := worktreeRoot(root)
	if err != nil {
		return NewResult{}, err
	}
	// Git records worktree paths with symlinks resolved, so resolve ours the
	// same way before comparing (e.g. /var vs /private/var on macOS).
	repoName := filepath.Base(mainPath)
	nameDir := resolvePath(filepath.Join(rootDir, repoName, name))
	target := filepath.Join(nameDir, repoName)
	rootDir = resolvePath(rootDir)

	for _, record := range records {
		if record.Path != target {
			continue
		}
		if _, err := os.Stat(target); err != nil {
			return NewResult{}, newFailure(exitNewPathConflict, "%s is registered as a worktree but its directory is missing (run git worktree prune)", target)
		}
		if record.Branch != branch {
			return NewResult{}, newFailure(exitNewPathConflict, "%s already exists as a worktree for branch %q", target, valueOr(record.Branch, "(detached)"))
		}
		return NewResult{SchemaVersion: schemaVersion, Name: name, Path: target, Branch: branch, Created: false}, nil
	}
	if _, err := os.Lstat(nameDir); err == nil {
		return NewResult{}, newFailure(exitNewPathConflict, "%s already exists and is not the requested worktree", nameDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return NewResult{}, err
	}
	for _, record := range records {
		if record.Branch == branch {
			return NewResult{}, newFailure(exitNewBranchConflict, "branch %q is already checked out at %s", branch, record.Path)
		}
	}
	if _, err := gitOutputFrom(cwdRepo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return NewResult{}, newFailure(exitNewBranchConflict, "branch %q already exists", branch)
	}

	if remote := remoteOfRef(cwdRepo, base); remote != "" {
		ref := strings.TrimPrefix(base, remote+"/")
		if err := gitRunTimeout(cwdRepo, longExecTimeout, "fetch", "--quiet", remote, ref); err != nil {
			fmt.Fprintf(stderr, "gw new: warning: fetch %s %s failed, using the local ref: %v\n", remote, ref, err)
		}
	}
	if _, err := gitOutputFrom(cwdRepo, "rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
		return NewResult{}, newFailure(exitNewBaseNotFound, "base %q not found", base)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return NewResult{}, fmt.Errorf("create worktree directory: %w", err)
	}
	// --no-track keeps the new branch from tracking the base (e.g. origin/main),
	// so the first push sets its own upstream.
	if err := gitRunTimeout(cwdRepo, longExecTimeout, "worktree", "add", "--quiet", "--no-track", "-b", branch, target, base); err != nil {
		removeEmptyRepoDir(rootDir, target)
		// Another caller may have created the same branch or path after our checks.
		if _, pathErr := os.Lstat(target); pathErr == nil {
			return NewResult{}, newFailure(exitNewPathConflict, "git worktree add: %v", err)
		}
		if _, branchErr := gitOutputFrom(cwdRepo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); branchErr == nil {
			return NewResult{}, newFailure(exitNewBranchConflict, "git worktree add: %v", err)
		}
		return NewResult{}, fmt.Errorf("git worktree add: %w", err)
	}
	return NewResult{SchemaVersion: schemaVersion, Name: name, Path: target, Branch: branch, Base: base, Created: true}, nil
}

// validateWorktreeName rejects names that would escape or nest inside the
// repository directory under the worktree root.
func validateWorktreeName(name string) error {
	switch {
	case name == "":
		return errors.New("name must not be empty")
	case name == ".":
		return fmt.Errorf("invalid name %q", name)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("invalid name %q: must not contain a path separator", name)
	case strings.Contains(name, ".."):
		return fmt.Errorf("invalid name %q: must not contain ..", name)
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("invalid name %q: must not start with -", name)
	case strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return fmt.Errorf("invalid name %q: must not contain control characters", name)
	}
	return nil
}

// worktreeRoot returns the directory that holds worktrees created by gw new:
// the --root flag, then $GW_WORKTREE_ROOT, then
// ${XDG_DATA_HOME:-~/.local/share}/gw/worktrees.
func worktreeRoot(flagValue string) (string, error) {
	if flagValue != "" {
		return mustAbs(flagValue), nil
	}
	if env := os.Getenv("GW_WORKTREE_ROOT"); env != "" {
		return mustAbs(env), nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(mustAbs(base), "gw", "worktrees"), nil
}

// remoteOfRef returns the remote name when ref has the form <remote>/<branch>
// for a configured remote, otherwise "".
func remoteOfRef(repoPath, ref string) string {
	out, err := gitOutputFrom(repoPath, "remote")
	if err != nil {
		return ""
	}
	best := ""
	for _, remote := range strings.Fields(out) {
		if strings.HasPrefix(ref, remote+"/") && len(remote) > len(best) {
			best = remote
		}
	}
	return best
}

// removeEmptyRepoDir removes empty parents of either <root>/<repo>/<name>/<repo>
// or the legacy <root>/<repo>/<name>. It never removes root or nonempty parents.
func removeEmptyRepoDir(root, worktreePath string) {
	if root == "" {
		return
	}
	root = resolvePath(root)
	parent := resolvePath(filepath.Dir(mustAbs(worktreePath)))
	repoDir := parent
	if filepath.Dir(repoDir) != root {
		repoDir = filepath.Dir(parent)
		if filepath.Dir(repoDir) != root || filepath.Base(worktreePath) != filepath.Base(repoDir) {
			return
		}
		if err := os.Remove(parent); err != nil {
			return
		}
	}
	_ = os.Remove(repoDir)
}

// resolvePath returns path with symlinks resolved for the part of it that
// exists; the missing rest is appended unchanged.
func resolvePath(path string) string {
	path = mustAbs(path)
	rest := ""
	for current := path; ; {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		rest = filepath.Join(filepath.Base(current), rest)
		current = parent
	}
}

// parseInterspersed parses flags that may appear before or after positional
// arguments (flag.FlagSet stops at the first positional argument).
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		consumed := len(args) - len(rest)
		if consumed > 0 && args[consumed-1] == "--" {
			return append(positional, rest...), nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}
