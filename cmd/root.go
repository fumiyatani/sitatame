package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/tanifumiya/sitatame/internal/gitx"
	"github.com/tanifumiya/sitatame/internal/termcheck"
)

// Env carries the I/O streams and TTY check for a single invocation, so tests
// can inject substitutes.
type Env struct {
	Stdin      *os.File
	Stdout     io.Writer
	Stderr     io.Writer
	IsTerminal func(fd uintptr) bool
}

// DefaultEnv binds the process streams and the platform TTY check.
func DefaultEnv() Env {
	return Env{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		IsTerminal: termcheck.IsTerminal,
	}
}

// RunRoot implements `sitatame [base]`. The TUI itself ships in a later task;
// for now we validate the entry conditions (TTY + repo + base resolution) and
// print a placeholder line.
func RunRoot(env Env, args []string) int {
	if !env.IsTerminal(env.Stdin.Fd()) {
		fmt.Fprintln(env.Stderr, "sitatame: stdin is not a TTY; sitatame requires an interactive terminal")
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: cwd: %v\n", err)
		return 1
	}
	repo, err := gitx.Discover(cwd)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: %v\n", err)
		return 1
	}

	var explicit string
	if len(args) > 0 {
		explicit = args[0]
	}
	base, err := gitx.ResolveBase(repo, explicit)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: %v\n", err)
		return 1
	}

	// Placeholder until the TUI lands; downstream tasks replace this with the
	// bubbletea program.
	fmt.Fprintf(env.Stderr, "sitatame: base=%s sha=%s (TUI unimplemented)\n", base.Ref, base.SHA)
	return 2
}
