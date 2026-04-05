package subcmd

import (
	"fmt"
	"os"

	"github.com/kawaken/gw/git"
	"github.com/kawaken/gw/metadata"
	"github.com/kawaken/gw/worktree"
)

// Completion implements `gw __completion <tasks|archived>`.
// Outputs "label:description" lines for zsh _describe.
func Completion(g git.Runner, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gw __completion <tasks|archived>")
		return 1
	}
	mode := args[0]
	if mode != "tasks" && mode != "archived" {
		fmt.Fprintf(os.Stderr, "gw __completion: unknown mode %q (use tasks or archived)\n", mode)
		return 1
	}

	mainPath, err := git.MainWorktreePath(g)
	if err != nil {
		return 1
	}

	entries, err := worktree.Sorted(g)
	if err != nil {
		return 1
	}

	for _, e := range entries {
		label := worktree.MakeLabel(e.Path, mainPath)
		if label == "main" {
			continue
		}

		m, err := metadata.GetAll(e.Path)
		if err != nil {
			return 1
		}
		archived := m["archived"] == "true"

		switch mode {
		case "tasks":
			if archived {
				continue
			}
		case "archived":
			if !archived {
				continue
			}
		}

		desc := m["purpose"]
		if desc == "" {
			desc = e.Branch
		}
		if desc != "" {
			fmt.Printf("%s:%s\n", label, desc)
		} else {
			fmt.Println(label)
		}
	}
	return 0
}
