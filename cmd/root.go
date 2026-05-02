package cmd

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tanifumiya/sitatame/internal/diffmodel"
	"github.com/tanifumiya/sitatame/internal/gitx"
	"github.com/tanifumiya/sitatame/internal/review"
	"github.com/tanifumiya/sitatame/internal/termcheck"
	"github.com/tanifumiya/sitatame/internal/tui"
)

// TUIOptions is the bag of values handed to the TUI runner. Tests can swap
// the runner via Env.RunTUI to assert resolution without launching bubbletea.
type TUIOptions struct {
	Repo   *gitx.Repo
	Base   gitx.Base
	Files  []diffmodel.File
	Review review.Review
}

// Env carries the I/O streams, TTY check, and the TUI runner for a single
// invocation. Tests inject substitutes; production uses DefaultEnv.
type Env struct {
	Stdin      *os.File
	Stdout     io.Writer
	Stderr     io.Writer
	IsTerminal func(fd uintptr) bool
	RunTUI     func(env Env, opts TUIOptions) error
}

// DefaultEnv binds the process streams, the platform TTY check, and the real
// bubbletea-based TUI runner.
func DefaultEnv() Env {
	return Env{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		IsTerminal: termcheck.IsTerminal,
		RunTUI:     defaultRunTUI,
	}
}

func defaultRunTUI(env Env, opts TUIOptions) error {
	model := tui.New(opts.Files, opts.Review)
	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithInput(env.Stdin),
		tea.WithOutput(env.Stdout),
	)
	_, err := p.Run()
	return err
}

// RunRoot implements `sitatame [base]`: validate the entry conditions
// (TTY + repo + base resolution), build the diff model, and hand off to the
// TUI runner.
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

	files, err := repo.Diff(base.Ref)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: diff: %v\n", err)
		return 1
	}

	branch, _ := repo.CurrentBranch()
	headSHA, _ := repo.HeadSHA()
	r := review.Review{
		Schema: 1,
		Branch: branch,
		Base:   review.Ref{Ref: base.Ref, SHA: base.SHA},
		Head:   review.Ref{Ref: "HEAD", SHA: headSHA},
	}

	runner := env.RunTUI
	if runner == nil {
		runner = defaultRunTUI
	}
	if err := runner(env, TUIOptions{Repo: repo, Base: base, Files: files, Review: r}); err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: tui: %v\n", err)
		return 1
	}
	return 0
}
