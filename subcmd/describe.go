package subcmd

import (
	"flag"
	"os/exec"
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

	mainPath, err := git.MainWorktreePath(g)
	if err != nil {
		output.Errorf("describe: %v", err)
		return 1
	}

	// Determine target worktree path
	var wtPath string
	if fs.NArg() > 0 {
		wtPath, err = worktree.Resolve(g, fs.Arg(0))
		if err != nil {
			output.Errorf("describe: %v", err)
			return 1
		}
	} else {
		wtPath = gitToplevel()
	}

	if *purpose != "" {
		if err := metadata.Set(mainPath, wtPath, "purpose", *purpose); err != nil {
			output.Errorf("describe: failed to set purpose: %v", err)
			return 1
		}
		output.Print(output.Result{
			Messages: []string{"purpose: " + *purpose},
		})
		return 0
	}

	// Show all metadata
	m := metadata.GetAll(mainPath, wtPath)
	branch, _ := g.RunIn(wtPath, "branch", "--show-current")

	var msgs []string
	msgs = append(msgs, "path: "+wtPath)
	msgs = append(msgs, "branch: "+branch)
	msgs = append(msgs, "purpose: "+strDefault(m["purpose"], "(not set)"))
	msgs = append(msgs, "original_branch: "+strDefault(m["original_branch"], "(not set)"))
	msgs = append(msgs, "archived: "+strDefault(m["archived"], "false"))
	if pn := m["pr_number"]; pn != "" {
		msgs = append(msgs, "pr_number: "+pn)
		msgs = append(msgs, "pr_url: "+m["pr_url"])
	}

	output.Print(output.Result{Messages: msgs})
	return 0
}

// gitToplevel returns the git toplevel of the current directory, or ".".
func gitToplevel() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "."
	}
	return strings.TrimSpace(string(out))
}

func strDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
