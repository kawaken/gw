package subcmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/kawaken/gw/git"
	"github.com/kawaken/gw/metadata"
	"github.com/kawaken/gw/output"
	"github.com/kawaken/gw/worktree"
)

// Describe implements `gw describe [--purpose <text>] [task]`.
func Describe(g git.Runner, args []string) int {
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	purpose := fs.String("purpose", "", "set purpose/description")
	if err := fs.Parse(args); err != nil {
		output.Errorf("describe: %v", err)
		return 1
	}

	_, err := git.MainWorktreePath(g)
	if err != nil {
		output.Errorf("describe: %v", err)
		return 1
	}

	if fs.NArg() > 1 {
		output.Errorf("describe: too many arguments")
		return 1
	}

	var wtPath string
	if fs.NArg() > 0 {
		wtPath, err = worktree.Resolve(g, fs.Arg(0))
		if err != nil {
			output.Errorf("describe: %v", err)
			return 1
		}
	} else {
		wtPath, err = g.Toplevel()
		if err != nil {
			output.Errorf("describe: %v", err)
			return 1
		}
	}

	if *purpose != "" {
		if err := metadata.Set(wtPath, "purpose", *purpose); err != nil {
			output.Errorf("describe: failed to set purpose: %v", err)
			return 1
		}
		output.Print(output.Result{
			Messages: []string{"purpose: " + *purpose},
		})
		return 0
	}

	m, err := metadata.GetAll(wtPath)
	if err != nil {
		output.Errorf("describe: failed to read metadata: %v", err)
		return 1
	}
	branch, err := g.RunIn(wtPath, "branch", "--show-current")
	branch = strings.TrimSpace(branch)
	if err != nil {
		branch = fmt.Sprintf("(unknown: %v)", err)
	} else if branch == "" {
		branch = "(detached)"
	}

	msgs := []string{
		"path: " + wtPath,
		"branch: " + branch,
		"purpose: " + strDefault(m["purpose"], "(not set)"),
		"original_branch: " + strDefault(m["original_branch"], "(not set)"),
		"archived: " + strDefault(m["archived"], "false"),
	}
	if pn := m["pr_number"]; pn != "" {
		msgs = append(msgs, "pr_number: "+pn)
		if prURL := m["pr_url"]; prURL != "" {
			msgs = append(msgs, "pr_url: "+prURL)
		}
	}

	output.Print(output.Result{Messages: msgs})
	return 0
}

func strDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
