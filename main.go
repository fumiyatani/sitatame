package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `sitatame - TUI git diff review tool

Usage:
  sitatame [base]              Launch TUI review against <base> (auto-detect if omitted)
  sitatame search <pattern>    Search saved reviews

Flags:
  -h, --help                   Show this help
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			fmt.Fprint(stdout, usage)
			return 0
		case "search":
			return runSearch(args[1:], stdout, stderr)
		}
	}
	return runRoot(args, stdout, stderr)
}

func runRoot(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "sitatame: unimplemented (root TUI)")
	fmt.Fprint(stderr, usage)
	return 2
}

func runSearch(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "sitatame search: unimplemented")
	return 2
}
