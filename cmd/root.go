package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/config"
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
	// Clipboard copies text to the system clipboard. When nil, the real
	// clipboard.Copy implementation is used. Tests inject a stub to verify
	// clipboard interaction without spawning real processes.
	Clipboard func(text string) error
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
	Staged      bool
	Working     bool
	BaseArg     string
	New         bool // --new: refuse if review.md already exists
	ForceNew    bool // --force-new: back up review.md and start fresh
	NoClipboard bool // --no-clipboard: skip copying the review path to clipboard
}

// parseRootArgs splits args into the rootOpts shape, enforcing the rule that
// --staged and --working are mutually exclusive and cannot be combined with a
// positional base argument. --new and --force-new are mutually exclusive.
func parseRootArgs(args []string) (rootOpts, error) {
	var opts rootOpts
	for _, a := range args {
		switch {
		case a == "--staged":
			opts.Staged = true
		case a == "--working":
			opts.Working = true
		case a == "--new":
			opts.New = true
		case a == "--force-new":
			opts.ForceNew = true
		case a == "--no-clipboard":
			opts.NoClipboard = true
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
	if opts.New && opts.ForceNew {
		return rootOpts{}, fmt.Errorf("--new and --force-new are mutually exclusive")
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

	// Per-repo config (.sitatame/config.yaml) is best-effort: a missing file
	// is the common case, and a malformed file must not block the TUI from
	// launching. We surface the parse error as a warning and fall through
	// with the zero Config, which preserves the historical base auto-detect
	// behavior.
	//
	// --staged / --working force the base to HEAD and never consult the
	// config (see resolveDiffSpec and docs/config.md). Loading + warning on
	// a malformed config in those modes would surface noise the user cannot
	// act on for this invocation, so we skip the load entirely. The cfg ==
	// nil path is the same one mergeBaseCandidates already takes when no
	// file is present, so downstream code does not need a special case.
	var cfg *config.Config
	if !opts.Staged && !opts.Working {
		var cfgErr error
		cfg, cfgErr = config.LoadFromRepo(repo.Workdir, env.Stderr)
		if cfgErr != nil {
			fmt.Fprintf(env.Stderr, "sitatame: config: %v (ignoring file)\n", cfgErr)
		}
	}

	spec, base, err := resolveDiffSpec(repo, opts, cfg)
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
	// In detached HEAD CurrentBranch returns "" and review.BranchSlug("")
	// collapses every detached session in this repo onto the same
	// "branch__da39a3ee" directory — two concurrent detached sessions would
	// share state. Normalising into "detached/<sha[:12]>" gives each detached
	// HEAD its own per-SHA slug; if HeadSHA also fails (pathological /
	// unborn-HEAD case) we fall back to the empty-branch slug, matching the
	// previous behaviour.
	if branch == "" && len(headSHA) >= 12 {
		branch = "detached/" + headSHA[:12]
	}
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
	// Start from a fresh Review and overlay any existing draft on top so
	// that resuming a session preserves every field the draft owned
	// (Extras / CreatedAt / Body / Files etc.) while still reflecting the
	// *current* diff in Branch / Base / Head.
	r := review.Review{
		Schema: 1,
		Branch: branch,
		Base:   review.Ref{Ref: base.Ref, SHA: base.SHA},
		Head:   review.Ref{Ref: headRef, SHA: headRefSHA},
	}

	paths := review.NewPaths(repo.Workdir, branch)
	// Only warn about the repo-local .sitatame/ legacy directory (pre-#38
	// in-repo storage). The output-root drafts/reviews layout (pre-#76) is
	// handled automatically by MigrateLegacyLayout below, so we pass an
	// empty newDraftsRoot to suppress the now-redundant migration hint.
	warnLegacySitatameDir(env, paths.LegacyRoot(), "")
	store := review.NewStore(paths)

	// Crash recovery: if review.md was lost mid-write but review.md.bak
	// exists, restore it before trying to load. Orphaned .tmp files are
	// also cleaned up here. Best-effort: a recovery failure must not block
	// startup.
	if rerr := store.RecoverFromCrash(); rerr != nil {
		fmt.Fprintf(env.Stderr, "sitatame: crash recovery failed: %v (continuing)\n", rerr)
	}

	// Phase 4: one-time migration from drafts/reviews to 1-branch-1-file layout.
	// Must run after RecoverFromCrash (so BranchDir is clean) and before
	// autoload (so the migrated review.md is visible to DetectReview).
	// Migration failure is non-fatal: the user can still operate on the new layout.
	if migrated, legacyDir, merr := store.MigrateLegacyLayout(); merr != nil {
		fmt.Fprintf(env.Stderr, "sitatame: migration warning: %v\n", merr)
	} else if migrated > 0 {
		fmt.Fprintf(env.Stderr,
			"sitatame: migrated %d branch(es) to new layout; legacy data preserved in %s\n",
			migrated, legacyDir)
	}

	// --new: refuse if review.md already exists.
	if opts.New {
		if existing, _ := store.DetectReview(); existing != "" {
			fmt.Fprintf(env.Stderr,
				"sitatame: review already exists for branch: %s; use --force-new to overwrite\n", existing)
			return 1
		}
	}

	// --force-new: back up existing review.md and start fresh.
	if opts.ForceNew {
		if existing, _ := store.DetectReview(); existing != "" {
			if err := os.Rename(existing, paths.BakFile()); err != nil {
				fmt.Fprintf(env.Stderr, "sitatame: --force-new: backup review: %v\n", err)
				return 1
			}
			fmt.Fprintf(env.Stderr, "sitatame: --force-new: backed up %s to %s\n", existing, paths.BakFile())
		}
	}

	if existing, derr := store.DetectReview(); derr == nil && existing != "" {
		fmt.Fprintf(env.Stderr, "sitatame: review exists: %s\n", existing)
		// Auto-load the previously-saved review so a re-run on the same
		// branch surfaces the user's prior comments instead of starting
		// from an empty Review.
		//
		// Failures here are best-effort: a corrupt or unreadable review
		// should not block a fresh session, so we surface the reason on
		// stderr and continue with the empty Review.
		//
		// Why we start from `loaded` and only overwrite the
		// diff-derived fields, rather than copying a handful of fields
		// off the loaded review onto a freshly-built Review:
		//
		//   * PR #65 introduced top-level / file / comment `Extras`
		//     maps that hold YAML keys we don't model — AI agents lean
		//     on this forward-compat hook. A field-by-field copy drops
		//     those keys on the floor.
		//   * `CreatedAt` (documented by PR #65 / PR #69) and the raw
		//     Markdown `Body` are the same story: dropping them on
		//     resume mutates the very file we then re-save.
		//   * Branch / Base / Head must reflect the *current* diff
		//     snapshot, not the one the review was saved against, so we
		//     unconditionally overwrite them after the value copy.
		//
		// Files is preserved as-loaded by design (PR #70 round-2 P2
		// fix): the on-disk review is the only source of per-FileMeta
		// `Extras` (forward-compat keys AI agents stash there) and of
		// the original diff snapshot.
		if loaded, lerr := loadReviewForResume(existing); lerr != nil {
			fmt.Fprintf(env.Stderr, "sitatame: review load failed: %v (starting empty)\n", lerr)
		} else {
			r = loaded
			r.Branch = branch
			r.Base = review.Ref{Ref: base.Ref, SHA: base.SHA}
			r.Head = review.Ref{Ref: headRef, SHA: headRefSHA}
			if r.Schema == 0 {
				r.Schema = 1
			}
			// Run validation with stderr warnings so the PR #61 legacy
			// anchor detector flags reviews saved before issue #36 / #19
			// were fixed. Validate also re-classifies comment state vs.
			// the freshly-loaded diff (open / stale).
			review.ValidateWithWarnings(&r, files, env.Stderr)
		}
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

	noClipboard := opts.NoClipboard || os.Getenv("SITATAME_NO_CLIPBOARD") != ""
	return finalizeReview(env, store, result, noClipboard)
}

// loadReviewForResume reads `path` and decodes it as a review.Review. The
// caller adopts the returned value wholesale (preserving Extras / CreatedAt /
// Body / Files etc.) and only overwrites the fields tied to the *current*
// diff snapshot (Branch / Base / Head). Files is kept as-loaded so per-
// FileMeta Extras survive resume -> save; refreshing Files against the live
// diff is left as a follow-up.
//
// Returned errors are surfaced on stderr by the caller and the session
// continues with an empty Review — a corrupt or unreadable review must not
// block startup.
func loadReviewForResume(path string) (review.Review, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return review.Review{}, fmt.Errorf("read review: %w", err)
	}
	r, err := review.Decode(b)
	if err != nil {
		return review.Review{}, fmt.Errorf("decode review: %w", err)
	}
	return r, nil
}

// resolveDiffSpec turns the parsed CLI options into a DiffSpec plus the
// review.Ref-shaped Base record that goes into the review YAML. For
// --staged/--working the "base" is HEAD itself; for the default mode the base
// goes through the auto-detect / explicit chain.
//
// cfg, when non-nil, contributes to the auto-detect path only: an explicit
// CLI base argument still wins, and --staged / --working both ignore the
// config because their "base" is HEAD by definition. See mergeBaseCandidates
// for how cfg.Base.Default and cfg.Base.Candidates are layered on top of the
// built-in fallback chain.
func resolveDiffSpec(repo *gitx.Repo, opts rootOpts, cfg *config.Config) (gitx.DiffSpec, gitx.Base, error) {
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
		candidates := mergeBaseCandidates(cfg)
		base, err := gitx.ResolveBaseWithCandidates(repo, opts.BaseArg, candidates)
		if err != nil {
			return gitx.DiffSpec{}, gitx.Base{}, err
		}
		return gitx.DiffSpec{Source: gitx.SourceRange, Base: base.Ref}, base, nil
	}
}

// mergeBaseCandidates assembles the candidate list that drives
// ResolveBaseWithCandidates' auto-detect path. The two config fields have
// deliberately asymmetric semantics:
//
//   - cfg.Base.Candidates is a *replacement* list. When the YAML
//     `candidates:` key is present (cfg.Base.CandidatesPresent == true) it is
//     the entire chain we try — gitx.BaseCandidates is NOT appended, even if
//     the user wrote `candidates: []`. This is the contract documented in
//     docs/config.md: a repo that pins `candidates: [origin/develop]` (or
//     `candidates: []` with a default) must never silently fall back to
//     `origin/main` / `main`, because the auto-resolved base is what every
//     review is anchored against and a mismatched base produces a misleading
//     review with no warning. The CandidatesPresent flag is what
//     distinguishes "key omitted" (use built-in fallback) from "key set to
//     []" (refuse to fall back); collapsing them on len(slice) == 0 would
//     silently re-enable the chain users were trying to opt out of.
//   - cfg.Base.Default is *additive*. It is shorthand for "try this ref
//     first" and is prepended to whichever chain follows — either the
//     replacement Candidates list or the built-in BaseCandidates fallback.
//
// Concretely:
//
//	default="", candidates omitted             -> nil  (use gitx.BaseCandidates)
//	default="X", candidates omitted            -> [X, ...gitx.BaseCandidates]
//	default="", candidates=[]                  -> nil  (no candidates configured;
//	                                              safety net: fall back to built-in
//	                                              with a stderr note via the caller)
//	default="X", candidates=[]                 -> [X]  (only the configured default;
//	                                              built-in chain stays out)
//	default="",  candidates=[A, B]             -> [A, B]
//	default="X", candidates=[A, B]             -> [X, A, B]
//
// Returning nil for the no-config and "empty list with no default" cases is
// load-bearing: ResolveBaseWithCandidates falls back to gitx.BaseCandidates
// when its candidates argument is nil/empty, so the existing
// TestResolveBase_FallsBackToMain et al keep hitting the built-in chain
// unchanged. The "empty list with no default" case is a misconfiguration —
// the user opted out of the built-in chain without providing any
// replacement — so we fall back to the built-in as a safety net rather than
// guaranteeing an auto-detect failure. The CandidatesPresent + len(Default)
// path is the one that actually enforces the opt-out.
//
// Duplicates are collapsed so the failure message in
// ResolveBaseWithCandidates does not list the same ref twice (e.g. if the
// user puts "main" in both Default and the built-in fallback path).
func mergeBaseCandidates(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	hasDefault := cfg.Base.Default != ""
	candidatesPresent := cfg.Base.CandidatesPresent

	// Pick the tail chain.
	//   - candidates key explicitly present -> use its contents (even if
	//     empty) as the replacement list. Built-in chain stays out.
	//   - candidates key omitted -> built-in is the fallback so a
	//     default-only (or no-config) invocation still has somewhere to
	//     land.
	var tail []string
	switch {
	case candidatesPresent:
		tail = cfg.Base.Candidates
		// If both candidates and default are absent in this branch
		// (candidates: [] with no default), there is nothing to try.
		// Returning nil lets ResolveBaseWithCandidates apply its
		// built-in fallback as a safety net rather than guaranteeing a
		// failure for a likely-misconfigured file.
		if !hasDefault && len(tail) == 0 {
			return nil
		}
	case hasDefault:
		tail = gitx.BaseCandidates
	default:
		return nil
	}

	seen := make(map[string]bool)
	out := make([]string, 0, 1+len(tail))
	add := func(c string) {
		if c == "" || seen[c] {
			return
		}
		seen[c] = true
		out = append(out, c)
	}
	add(cfg.Base.Default)
	for _, c := range tail {
		add(c)
	}
	return out
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

// warnLegacySitatameDir prints two one-line stderr notices when a pre-#38
// <repo>/.sitatame/ directory is still present: one that says the directory is
// ignored, and one with a copy-pasteable migration command for the user's
// actual new drafts root. We intentionally do not auto-migrate: users may
// have stale drafts they want to review or commit before deleting, and a
// silent move could clobber work. Empty legacyDir or a missing path is a
// no-op.
//
// The migration line bundles `mkdir -p <newDraftsRoot> && mv ...` because on
// a first upgrade the new drafts root does not exist yet; a bare `mv` would
// fail and leave the user guessing. newDraftsRoot is `paths.DraftsRoot()`
// from the caller — i.e. the resolved `<output-root>/<project-slug>/drafts`
// for this checkout — and is fed into the message so users do not have to
// compute the project slug by hand.
//
// Paths are POSIX single-quote wrapped via shellQuote so users with spaces or
// shell metacharacters (`$`, `*`, `(`, ...) in their checkout / SITATAME_HOME
// can paste the hint as-is. The `/drafts/*` glob in the mv source is left
// outside the quotes so the shell still expands it; quoting it would turn the
// `*` into a literal and the `mv` would fail with "no such file".
func warnLegacySitatameDir(env Env, legacyDir, newDraftsRoot string) {
	if legacyDir == "" {
		return
	}
	if _, err := os.Stat(legacyDir); err != nil {
		return
	}
	// Issue #24 introduced <repo>/.sitatame/config.yaml as a legitimate
	// in-repo file. If the directory contains nothing but config-file
	// entries from the allowlist, treat it as a pure config directory and
	// stay silent — the legacy warning is meant to flag stale review
	// artifacts (drafts/, reviews/), not the new config file.
	if onlyConfigEntries(legacyDir) {
		return
	}
	fmt.Fprintf(env.Stderr,
		"sitatame: legacy %s/ detected — ignored.\n",
		legacyDir,
	)
	if newDraftsRoot != "" {
		abs, err := filepath.Abs(newDraftsRoot)
		if err != nil || abs == "" {
			abs = newDraftsRoot
		}
		fmt.Fprintf(env.Stderr,
			"sitatame: To migrate drafts: mkdir -p %s && mv %s/drafts/* %s/\n",
			shellQuote(abs), shellQuote(legacyDir), shellQuote(abs),
		)
	}
}

// configEntryAllowlist names the files inside <repo>/.sitatame/ that are
// considered legitimate config artifacts as of issue #24, not legacy review
// state. When the directory contains nothing outside this set,
// warnLegacySitatameDir suppresses the legacy notice.
//
// Kept tight on purpose: only files Sitatame itself owns belong here. If a
// future release adds another in-repo config file (e.g. a per-repo schema
// version marker), extend this set in the same commit so users do not see
// the legacy warning regress.
var configEntryAllowlist = map[string]bool{
	config.FileName: true,
}

// onlyConfigEntries reports whether dir exists and contains *only*
// allowlisted config files (no subdirectories, no unknown files, at least
// one allowlisted file present). Empty directories return false because
// pre-#24 sitatame installs left .sitatame/ on disk even after the user
// cleared drafts/reviews, and that is still a legitimate signal that the
// legacy notice should fire. Read errors also fall through to false so the
// warning still triggers on permission denials rather than silently
// swallowing a real legacy directory.
func onlyConfigEntries(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	if len(entries) == 0 {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			return false
		}
		if !configEntryAllowlist[e.Name()] {
			return false
		}
	}
	return true
}

// shellQuote wraps s in POSIX single quotes, escaping embedded single quotes
// with the standard `'\''` trick. Used so paths printed into copy-paste shell
// snippets survive spaces and metacharacters (`$`, `*`, `(`, backticks, ...).
// We don't use strconv.Quote because that produces Go-style double-quoted
// strings, which would re-expand `$VAR` and friends under sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// runTUIWithShutdown wraps the runner with a defer-based safety net: if the
// runner panics, we still write the in-memory review to drafts/ on a
// best-effort basis (recovering any panic from SaveDraft itself), then re-raise
// the original panic so the caller / test sees it. The terminal restore is
// owned by bubbletea's own panic handler.
func runTUIWithShutdown(runner func(Env, TUIOptions) (TUIResult, error), env Env, opts TUIOptions, store *review.Store) (result TUIResult, runErr error) {
	result = TUIResult{Review: opts.Review, Reason: tui.QuitDiscard}
	var panicVal any
	func() {
		defer func() {
			panicVal = recover()
		}()
		result, runErr = runner(env, opts)
	}()
	if panicVal != nil {
		// Best-effort review save with the last known good state so the
		// user doesn't lose their work on an unexpected crash.
		func() {
			defer func() { _ = recover() }()
			_, _ = store.SaveReview(&result.Review)
		}()
		panic(panicVal)
	}
	return result, runErr
}
