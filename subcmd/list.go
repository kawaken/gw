package subcmd

import (
	"flag"

	"github.com/kawaken/gw/git"
	"github.com/kawaken/gw/metadata"
	"github.com/kawaken/gw/output"
	"github.com/kawaken/gw/worktree"
)

// List implements `gw list`.
func List(g git.Runner, args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	showPath := fs.Bool("path", false, "show path instead of branch/purpose")
	verbose := fs.Bool("v", false, "verbose (show full path)")
	var showAll bool
	fs.BoolVar(&showAll, "a", false, "include archived worktrees")
	fs.BoolVar(&showAll, "all", false, "include archived worktrees")
	if err := fs.Parse(args); err != nil {
		output.Errorf("list: %v", err)
		return 1
	}

	mainPath, err := git.MainWorktreePath(g)
	if err != nil {
		output.Errorf("list: %v", err)
		return 1
	}

	entries, err := worktree.Sorted(g)
	if err != nil {
		output.Errorf("list: %v", err)
		return 1
	}

	mode := worktree.ModeNormal
	if *showPath {
		mode = worktree.ModePath
	} else if *verbose {
		mode = worktree.ModeVerbose
	}

	var messages []string
	var infos []output.WorktreeInfo

	for _, e := range entries {
		m, err := metadata.GetAll(e.Path)
		if err != nil {
			output.Errorf("list: read metadata for %s: %v", e.Path, err)
			return 1
		}
		archived := m["archived"] == "true"
		if !showAll && archived {
			continue
		}
		label := worktree.MakeLabel(e.Path, mainPath)
		info := worktree.FormatInfo{
			Label:          label,
			Branch:         e.Branch,
			Age:            e.Age,
			Purpose:        m["purpose"],
			OriginalBranch: m["original_branch"],
			Archived:       archived,
			Path:           e.Path,
		}
		messages = append(messages, worktree.FormatLine(info, mode))
		infos = append(infos, output.WorktreeInfo{
			Label:    label,
			Branch:   e.Branch,
			Age:      e.Age,
			Path:     e.Path,
			Archived: archived,
			Purpose:  m["purpose"],
		})
	}

	output.Print(output.Result{
		Messages:  messages,
		Worktrees: infos,
	})
	return 0
}
