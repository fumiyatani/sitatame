package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/gitx"
	"github.com/fumiyatani/sitatame/internal/review"
	"github.com/fumiyatani/sitatame/internal/termcheck"
	"github.com/fumiyatani/sitatame/internal/tui"
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
	// LookPath resolves a binary name like exec.LookPath. Tests stub it to
	// force the search fallback path even when the host has ripgrep installed.
	LookPath func(name string) (string, error)
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
		LookPath:   exec.LookPath,
	}
}

func defaultRunTUI(env Env, opts TUIOptions) (TUIResult, error) {
	model := tui.New(opts.Files, opts.Review)
	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
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

// rootOpts is the parsed form of the positional + flag arguments to
// `sitatame [base]` / `sitatame --staged` / `sitatame --working`.
type rootOpts struct {
	Staged  bool
	Working bool
	BaseArg string
}

// parseRootArgs splits args into the rootOpts shape, enforcing the rule that
// --staged and --working are mutually exclusive and cannot be combined with a
// positional base argument.
func parseRootArgs(args []string) (rootOpts, error) {
	var opts rootOpts
	for _, a := range args {
		switch {
		case a == "--staged":
			opts.Staged = true
		case a == "--working":
			opts.Working = true
		case len(a) > 0 && a[0] == '-':
			return rootOpts{}, fmt.Errorf("unknown flag: %s", a)
		default:
			if opts.BaseArg != "" {
				return rootOpts{}, fmt.Errorf("unexpected extra argument: %s", a)
			}
			opts.BaseArg = a
		}
	}
	if opts.Staged && opts.Working {
		return rootOpts{}, fmt.Errorf("--staged and --working are mutually exclusive")
	}
	if (opts.Staged || opts.Working) && opts.BaseArg != "" {
		return rootOpts{}, fmt.Errorf("--staged/--working cannot be combined with an explicit base")
	}
	return opts, nil
}

// RunRoot implements `sitatame [base]`: validate the entry conditions
// (TTY + repo + base resolution), build the diff model, and hand off to the
// TUI runner.
func RunRoot(env Env, args []string) int {
	opts, err := parseRootArgs(args)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: %v\n", err)
		return 2
	}

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

	spec, base, err := resolveDiffSpec(repo, opts)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: %v\n", err)
		return 1
	}

	files, err := repo.Diff(spec)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: diff: %v\n", err)
		return 1
	}

	if (opts.Staged || opts.Working) && len(files) == 0 {
		fmt.Fprintf(env.Stderr, "sitatame: %s\n", emptyDiffMessage(spec))
		return 0
	}

	branch, _ := repo.CurrentBranch()
	headSHA, _ := repo.HeadSHA()
	headRef := "HEAD"
	headRefSHA := headSHA
	switch spec.Source {
	case gitx.SourceStaged:
		headRef = "INDEX"
		headRefSHA = ""
	case gitx.SourceWorking:
		headRef = "WORKTREE"
		headRefSHA = ""
	}
	r := review.Review{
		Schema: 1,
		Branch: branch,
		Base:   review.Ref{Ref: base.Ref, SHA: base.SHA},
		Head:   review.Ref{Ref: headRef, SHA: headRefSHA},
	}

	store := review.NewStore(review.NewPaths(repo.Workdir, branch))
	if existing, derr := store.DetectDraft(); derr == nil && existing != "" {
		fmt.Fprintf(env.Stderr, "sitatame: draft exists: %s\n", existing)
	}
	runner := env.RunTUI
	if runner == nil {
		runner = defaultRunTUI
	}
	tuiOpts := TUIOptions{Repo: repo, Base: base, Files: files, Review: r}
	result, err := runTUIWithShutdown(runner, env, tuiOpts, store)
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

// resolveDiffSpec turns the parsed CLI options into a DiffSpec plus the
// review.Ref-shaped Base record that goes into the review YAML. For
// --staged/--working the "base" is HEAD itself; for the default mode the base
// goes through the auto-detect / explicit chain.
func resolveDiffSpec(repo *gitx.Repo, opts rootOpts) (gitx.DiffSpec, gitx.Base, error) {
	switch {
	case opts.Staged:
		sha, err := repo.HeadSHA()
		if err != nil {
			return gitx.DiffSpec{}, gitx.Base{}, err
		}
		return gitx.DiffSpec{Source: gitx.SourceStaged}, gitx.Base{Ref: "HEAD", SHA: sha}, nil
	case opts.Working:
		sha, err := repo.HeadSHA()
		if err != nil {
			return gitx.DiffSpec{}, gitx.Base{}, err
		}
		return gitx.DiffSpec{Source: gitx.SourceWorking}, gitx.Base{Ref: "HEAD", SHA: sha}, nil
	default:
		base, err := gitx.ResolveBase(repo, opts.BaseArg)
		if err != nil {
			return gitx.DiffSpec{}, gitx.Base{}, err
		}
		return gitx.DiffSpec{Source: gitx.SourceRange, Base: base.Ref}, base, nil
	}
}

func emptyDiffMessage(spec gitx.DiffSpec) string {
	switch spec.Source {
	case gitx.SourceStaged:
		return "no staged changes"
	case gitx.SourceWorking:
		return "no working-tree changes"
	}
	panic(fmt.Sprintf("emptyDiffMessage: unexpected Source %d", spec.Source))
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
