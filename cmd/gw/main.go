package main

import (
	"fmt"
	"os"

	"github.com/kawaken/gw/git"
	"github.com/kawaken/gw/output"
	"github.com/kawaken/gw/subcmd"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}

	cmd := resolveAlias(args[0])
	rest := args[1:]

	// Internal commands that don't need git
	switch cmd {
	case "__fmt":
		return subcmd.Fmt(rest)
	case "init":
		return subcmd.Init(rest)
	}

	g := git.New()

	switch cmd {
	case "list", "ls":
		return subcmd.List(g, rest)
	case "describe", "desc":
		return subcmd.Describe(g, rest)
	case "__completion":
		return subcmd.Completion(g, rest)
	// Phase 2 stubs – not yet implemented
	case "create", "cr",
		"remove", "rm",
		"switch", "sw",
		"review", "pr",
		"prune",
		"archive",
		"activate",
		"sync",
		"sync-edit":
		output.Errorf("%s: not yet implemented", cmd)
		return 1
	default:
		output.Errorf("unknown subcommand: %s", cmd)
		printUsage()
		return 1
	}
}

func resolveAlias(cmd string) string {
	switch cmd {
	case "ls":
		return "list"
	case "cr":
		return "create"
	case "rm":
		return "remove"
	case "sw":
		return "switch"
	case "pr":
		return "review"
	case "desc":
		return "describe"
	}
	return cmd
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `usage: gw <subcommand> [args]

subcommands:
  create (cr)     Create new worktree
  list (ls)       List worktrees
  remove (rm)     Remove worktree
  switch (sw)     Switch to worktree
  describe (desc) Show/set metadata
  review (pr)     Create PR review worktree
  prune           Bulk remove worktrees
  archive         Archive worktree
  activate        Activate archived worktree
  sync            Sync files to worktree
  sync-edit       Edit sync config
  init            Generate shell setup script`)
}
