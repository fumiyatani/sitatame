package main

import (
	"fmt"
	"os"

	"github.com/fumiyatani/sitatame/cmd"
)

const usage = `sitatame - TUI git diff review tool

Usage:
  sitatame [base]              Launch TUI review against <base>..HEAD
                               (auto-detect if omitted)
  sitatame --staged            Review staged changes (index vs HEAD)
  sitatame --working           Review all uncommitted changes
                               (worktree vs HEAD; staged + unstaged)
  sitatame search [flags] <pattern>  Search saved reviews (regexp)

Flags:
  --staged                     Review the index against HEAD
  --working                    Review the working tree against HEAD
  --new                        Refuse to start if review.md already exists for
                               the current branch (use --force-new to overwrite)
  --force-new                  Back up the existing review.md to review.md.bak
                               and start a fresh session
  --no-clipboard               Do not copy the review path to the clipboard
                               after saving (also: SITATAME_NO_CLIPBOARD=1)
  -h, --help                   Show this help

Search flags (sitatame search):
  --project <slug>             Limit search to one project directory
  --branch <slug>              Limit search to one branch directory
  --state open|resolved|stale|all  Filter by comment state (default: all)
  --json                       Emit JSON array (for machine consumption)
  --root <path>                Override SITATAME_HOME for this search

Notes:
  --staged and --working are mutually exclusive and cannot be combined
  with an explicit base argument.
  --new and --force-new are mutually exclusive.
`

func main() {
	os.Exit(dispatch(os.Args[1:], cmd.DefaultEnv()))
}

func dispatch(args []string, env cmd.Env) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			fmt.Fprint(env.Stdout, usage)
			return 0
		case "search":
			return cmd.RunSearch(env, args[1:])
		}
	}
	return cmd.RunRoot(env, args)
}
