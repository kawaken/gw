package subcmd

import (
	"fmt"
	"os"
)

const shellWrapper = `function gw() {
    local result
    result=$(command gw "$@")
    local exit_code=$?
    printf '%s\n' "$result" | command gw __fmt messages
    local cd_path
    cd_path=$(printf '%s\n' "$result" | command gw __fmt cd)
    if [[ $exit_code -eq 0 ]] && [[ -n "$cd_path" ]]; then
        export GW_PREVIOUS_WORKTREE="$(git rev-parse --show-toplevel 2>/dev/null)"
        cd "$cd_path"
    fi
    return $exit_code
}

_gw() {
    local -a subcommands
    subcommands=(
        'create:Create new worktree'
        'list:List worktrees'
        'remove:Remove worktree'
        'switch:Switch to worktree'
        'describe:Show/set metadata'
        'review:Create PR review worktree'
        'prune:Bulk remove worktrees'
        'archive:Archive worktree'
        'activate:Activate archived worktree'
        'sync:Sync files to worktree'
        'sync-edit:Edit sync config'
        'init:Generate shell setup script'
    )
    _arguments -C '1: :->command' '*: :->args'
    case $state in
        command) _describe -t commands 'gw subcommand' subcommands ;;
        args)
            case $words[2] in
                switch|sw|remove|rm|describe|desc)
                    local -a tasks
                    tasks=(${(f)"$(command gw __completion tasks)"})
                    _describe -V -t tasks 'worktree' tasks ;;
                archive)
                    local -a tasks
                    tasks=(${(f)"$(command gw __completion tasks)"})
                    _describe -V -t tasks 'worktree' tasks ;;
                activate)
                    local -a tasks
                    tasks=(${(f)"$(command gw __completion archived)"})
                    _describe -V -t tasks 'worktree' tasks ;;
            esac ;;
    esac
}
compdef _gw gw
`

// Init implements `gw init <shell>`.
// Currently only "zsh" is supported.
// Output is raw shell script (not JSON).
func Init(args []string) int {
	shell := "zsh"
	if len(args) > 0 {
		shell = args[0]
	}
	switch shell {
	case "zsh":
		if _, err := os.Stdout.WriteString(shellWrapper); err != nil {
			fmt.Fprintf(os.Stderr, "gw init: write: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "gw init: unsupported shell %q (supported: zsh)\n", shell)
		return 1
	}
}
