package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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

// TUIResult is what the runner hands back to RunRoot after the TUI exits
// normally. Review carries any in-memory edits the user made; Reason tells
// RunRoot which save path to take.
type TUIResult struct {
	Review review.Review
	Reason tui.QuitReason
}

// Env carries the I/O streams, TTY check, and the TUI runner for a single
// invocation. Tests inject substitutes; production uses DefaultEnv.
type Env struct {
	Stdin      *os.File
	Stdout     io.Writer
	Stderr     io.Writer
	IsTerminal func(fd uintptr) bool
	RunTUI     func(env Env, opts TUIOptions) (TUIResult, error)
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

func defaultRunTUI(env Env, opts TUIOptions) (TUIResult, error) {
	model := tui.New(opts.Files, opts.Review)
	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithInput(env.Stdin),
		tea.WithOutput(env.Stdout),
	)
	final, err := p.Run()
	if err != nil {
		return TUIResult{Review: opts.Review, Reason: tui.QuitDraft}, err
	}
	fm, ok := final.(tui.Model)
	if !ok {
		return TUIResult{Review: opts.Review, Reason: tui.QuitDraft}, nil
	}
	return TUIResult{Review: fm.Review, Reason: fm.QuitReason()}, nil
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

	store := review.NewStore(review.NewPaths(repo.Workdir, branch))
	if existing, derr := store.DetectDraft(); derr == nil && existing != "" {
		fmt.Fprintf(env.Stderr, "sitatame: draft exists: %s\n", existing)
	}
	runner := env.RunTUI
	if runner == nil {
		runner = defaultRunTUI
	}
	opts := TUIOptions{Repo: repo, Base: base, Files: files, Review: r}
	result, err := runTUIWithShutdown(runner, env, opts, store)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: tui: %v\n", err)
		return 1
	}

	switch result.Reason {
	case tui.QuitPromote:
		draftPath, err := store.SaveDraft(&result.Review)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sitatame: save draft: %v\n", err)
			return 1
		}
		finalPath, err := store.Promote(draftPath)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sitatame: promote: %v\n", err)
			return 1
		}
		abs, _ := filepath.Abs(finalPath)
		fmt.Fprintf(env.Stdout, "SITATAME_REVIEW=%s\n", abs)
		return 0
	case tui.QuitDraft:
		if _, err := store.SaveDraft(&result.Review); err != nil {
			fmt.Fprintf(env.Stderr, "sitatame: save draft: %v\n", err)
			return 1
		}
		return 1
	}
	// QuitNone or any other value: runner returned without a save signal — exit
	// cleanly without touching the filesystem. In production this only happens
	// if the TUI is short-circuited (e.g. test stubs).
	return 0
}

// runTUIWithShutdown wraps the runner with a defer-based safety net: if the
// runner panics, we still write the in-memory review to drafts/ on a
// best-effort basis (recovering any panic from SaveDraft itself), then re-raise
// the original panic so the caller / test sees it. The terminal restore is
// owned by bubbletea's own panic handler.
func runTUIWithShutdown(runner func(Env, TUIOptions) (TUIResult, error), env Env, opts TUIOptions, store *review.Store) (result TUIResult, runErr error) {
	result = TUIResult{Review: opts.Review, Reason: tui.QuitDraft}
	var panicVal any
	func() {
		defer func() {
			panicVal = recover()
		}()
		result, runErr = runner(env, opts)
	}()
	if panicVal != nil {
		// Best-effort draft save with the last known good state.
		func() {
			defer func() { _ = recover() }()
			_, _ = store.SaveDraft(&result.Review)
		}()
		panic(panicVal)
	}
	return result, runErr
}
