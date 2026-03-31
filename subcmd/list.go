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
	showAll := fs.Bool("a", false, "include archived worktrees")
	fs.Bool("all", false, "include archived worktrees") // alias
	if err := fs.Parse(args); err != nil {
		output.Errorf("list: %v", err)
		return 1
	}
	// re-check --all
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "all" {
			*showAll = true
		}
	})

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
		archived := metadata.Get(mainPath, e.Path, "archived") == "true"
		if !*showAll && archived {
			continue
		}
		label := worktree.MakeLabel(e.Path, mainPath)
		purpose := metadata.Get(mainPath, e.Path, "purpose")
		origBranch := metadata.Get(mainPath, e.Path, "original_branch")

		line := worktree.FormatLine(label, e.Branch, e.Age, purpose, origBranch, archived, e.Path, mode)
		messages = append(messages, line)

		infos = append(infos, output.WorktreeInfo{
			Label:    label,
			Branch:   e.Branch,
			Age:      e.Age,
			Path:     e.Path,
			Archived: archived,
			Purpose:  purpose,
		})
	}

	output.Print(output.Result{
		Messages:  messages,
		Worktrees: infos,
	})
	return 0
}
