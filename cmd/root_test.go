package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/tui"
)

func newRepo(t *testing.T) (dir, mainSHA string) {
	t.Helper()
	dir = t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(c.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "init")
	mainSHA = mustGit(t, dir, "rev-parse", "HEAD")

	mustGit(t, dir, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "feature")
	return dir, mainSHA
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(c.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// chdir switches into dir for the duration of the test and isolates
// SITATAME_HOME so the test never touches the developer's real ~/.sitatame.
// Tests that need to assert on the per-project output root can call
// review.NewPaths(dir, branch) (or ProjectSlug(dir)) to recover the resolved
// location.
//
// t.Chdir (Go 1.24+) installs a process-wide chdir + automatic restore via
// t.Cleanup. It also marks the test as non-parallelisable, which matches the
// semantics we want here — os.Chdir is process-global, so two parallel tests
// chdir'ing to different repos would race. t.Setenv inherits the same
// "cannot t.Parallel()" guarantee, so the two calls reinforce each other.
func chdir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
	t.Setenv("SITATAME_HOME", t.TempDir())
}

func ttyEnv(stdin *os.File, term bool) Env {
	var stdout, stderr bytes.Buffer
	return Env{
		Stdin:      stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return term },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
		},
	}
}

// captureTUIEnv is like ttyEnv but records the TUIOptions passed to RunTUI so
// tests can assert on the resolved base / files.
func captureTUIEnv(stdin *os.File, term bool, captured *TUIOptions) (Env, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return Env{
		Stdin:      stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		IsTerminal: func(uintptr) bool { return term },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			*captured = opts
			return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
		},
	}, stdout, stderr
}

func TestRunRoot_RejectsNonTTY(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return false },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
		},
	}
	code := RunRoot(env, nil)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "not a TTY") {
		t.Errorf("stderr missing TTY message: %q", stderr.String())
	}
}

func TestRunRoot_ResolvesAutoBase(t *testing.T) {
	dir, mainSHA := newRepo(t)
	chdir(t, dir)
	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if captured.Base.Ref != "main" {
		t.Errorf("base.Ref = %q, want main", captured.Base.Ref)
	}
	if captured.Base.SHA != mainSHA {
		t.Errorf("base.SHA = %q, want %q", captured.Base.SHA, mainSHA)
	}
	if captured.Review.Branch != "feature" {
		t.Errorf("branch = %q, want feature", captured.Review.Branch)
	}
}

func TestRunRoot_ExplicitBaseWins(t *testing.T) {
	dir, _ := newRepo(t)
	// rename main so auto would fail; explicit must still work.
	mustGit(t, dir, "branch", "-m", "main", "trunk")
	chdir(t, dir)
	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, []string{"trunk"}); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if captured.Base.Ref != "trunk" {
		t.Errorf("base.Ref = %q, want trunk", captured.Base.Ref)
	}
}

// writeRepoConfig writes <repo>/.sitatame/config.yaml with body. It mirrors
// the path layout config.LoadFromRepo expects, so RunRoot can pick the file
// up via the same code path users will hit.
func writeRepoConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".sitatame"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".sitatame", "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunRoot_ConfigBaseDefault_AffectsAutoDetect covers the issue #24 path:
// when no explicit base is given and base.default is set, the configured ref
// wins over the built-in BaseCandidates. Renaming main to trunk would
// normally cause auto-detect to fail; the config entry "trunk" must rescue
// it.
func TestRunRoot_ConfigBaseDefault_AffectsAutoDetect(t *testing.T) {
	dir, _ := newRepo(t)
	mustGit(t, dir, "branch", "-m", "main", "trunk")
	writeRepoConfig(t, dir, `base:
  default: "trunk"
`)
	chdir(t, dir)
	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if captured.Base.Ref != "trunk" {
		t.Errorf("base.Ref = %q, want trunk (from base.default)", captured.Base.Ref)
	}
}

// TestRunRoot_ConfigBaseDefault_LosesToExplicit confirms the priority order:
// the CLI argument still wins over the config-supplied default. Without this
// the config would become an unwelcome override on every invocation.
func TestRunRoot_ConfigBaseDefault_LosesToExplicit(t *testing.T) {
	dir, mainSHA := newRepo(t)
	writeRepoConfig(t, dir, `base:
  default: "feature"
`)
	chdir(t, dir)
	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, []string{"main"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if captured.Base.Ref != "main" {
		t.Errorf("base.Ref = %q, want main (CLI overrides config)", captured.Base.Ref)
	}
	if captured.Base.SHA != mainSHA {
		t.Errorf("base.SHA = %q, want %q", captured.Base.SHA, mainSHA)
	}
}

// TestRunRoot_ConfigBaseCandidates_OverridesChain documents that
// base.candidates fully replaces the built-in BaseCandidates fallback when
// base.default is absent. Renaming main to trunk would normally drop the
// auto-detect chain to empty refs; listing "trunk" in candidates must pick
// it back up.
func TestRunRoot_ConfigBaseCandidates_OverridesChain(t *testing.T) {
	dir, _ := newRepo(t)
	mustGit(t, dir, "branch", "-m", "main", "trunk")
	writeRepoConfig(t, dir, `base:
  candidates:
    - "nonexistent-ref"
    - "trunk"
`)
	chdir(t, dir)
	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if captured.Base.Ref != "trunk" {
		t.Errorf("base.Ref = %q, want trunk (from base.candidates)", captured.Base.Ref)
	}
}

// TestResolveDiffSpec_CandidatesReplaceBuiltin guards the documented
// contract that base.candidates is a *replacement* list, not an addition: a
// repo pinning `candidates: [origin/develop, origin/staging]` must never
// silently fall back to the built-in chain (which would resolve `main`)
// when none of the configured candidates exist. The auto-detect path must
// fail instead so the user notices and fixes their config — silently using
// the wrong base anchors the review against the wrong commits.
//
// The setup deliberately keeps `main` available in the repo: pre-fix
// (when builtins were appended to the user's candidates), this test would
// have resolved `main` and the test would have passed by accident. With
// the fix, builtins are no longer consulted, so RunRoot exits 1.
func TestResolveDiffSpec_CandidatesReplaceBuiltin(t *testing.T) {
	dir, _ := newRepo(t)
	// `main` is still present from newRepo. If builtins were appended to
	// the user's candidates, ResolveBaseWithCandidates would land on
	// `main` here and the assertion below would fail.
	writeRepoConfig(t, dir, `base:
  candidates:
    - "origin/develop"
    - "origin/staging"
`)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	tuiCalled := false
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			tuiCalled = true
			return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
		},
	}
	if code := RunRoot(env, nil); code != 1 {
		t.Errorf("exit = %d, want 1 (auto-detect must fail when only nonexistent candidates are configured)", code)
	}
	if tuiCalled {
		t.Errorf("RunTUI must not be called when base resolution fails")
	}
	// The error message should mention the user's candidates, not main.
	// We don't pin the exact wording but we do guard against the symptom
	// the bug produced: `main` being silently selected.
	if strings.Contains(stderr.String(), "main") && !strings.Contains(stderr.String(), "base not found") {
		t.Errorf("stderr mentions main without a 'base not found' diagnostic; built-in chain may have leaked in: %q", stderr.String())
	}
}

// TestResolveDiffSpec_DefaultIsPrependedToBuiltinWhenCandidatesEmpty
// asserts the second axis of the contract: when only base.default is set
// (no candidates), the configured ref is *prepended* to the built-in
// BaseCandidates rather than replacing it. This keeps "I just want to
// default to origin/release, but please still find main if release isn't
// here" working without forcing the user to spell out every fallback.
//
// We use a default that does not resolve (`origin/release` — no such
// remote in the test repo) so the test isolates the "tail still runs"
// behavior: if the built-in chain were dropped, RunRoot would exit 1; the
// fact that it picks `main` proves the chain is intact.
func TestResolveDiffSpec_DefaultIsPrependedToBuiltinWhenCandidatesEmpty(t *testing.T) {
	dir, mainSHA := newRepo(t)
	writeRepoConfig(t, dir, `base:
  default: "origin/release"
`)
	chdir(t, dir)
	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0 (built-in fallback should still run after an unresolvable default)", code)
	}
	if captured.Base.Ref != "main" {
		t.Errorf("base.Ref = %q, want main (built-in BaseCandidates should be appended after default)", captured.Base.Ref)
	}
	if captured.Base.SHA != mainSHA {
		t.Errorf("base.SHA = %q, want %q", captured.Base.SHA, mainSHA)
	}
}

// TestResolveDiffSpec_EmptyCandidatesUsesOnlyDefault is the PR #60 round 3
// regression test. An explicit `candidates: []` together with a configured
// `default` must restrict the auto-detect chain to *only* the default — the
// built-in fallback (origin/main / main / …) must not silently sneak in.
//
// Pre-fix, mergeBaseCandidates collapsed "key omitted" and "key set to []"
// onto the same len(slice) == 0 branch, so an explicit empty list would
// silently re-enable the built-in chain and `main` would resolve. The fix
// adds CandidatesPresent to BaseConfig and routes the empty-list case
// through the replacement branch. We pin that behavior by configuring a
// default that does not resolve in the test repo (`origin/release`) while
// leaving `main` reachable — pre-fix this would have landed on `main` and
// returned 0; post-fix the auto-detect chain is `[origin/release]` only and
// RunRoot exits 1.
func TestResolveDiffSpec_EmptyCandidatesUsesOnlyDefault(t *testing.T) {
	dir, _ := newRepo(t)
	// `main` is still present from newRepo. Pre-fix this is the ref the
	// built-in fallback would have silently selected.
	writeRepoConfig(t, dir, `base:
  default: "origin/release"
  candidates: []
`)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	tuiCalled := false
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			tuiCalled = true
			return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
		},
	}
	if code := RunRoot(env, nil); code != 1 {
		t.Errorf("exit = %d, want 1 (auto-detect must fail when default is unreachable and candidates list is explicitly empty)", code)
	}
	if tuiCalled {
		t.Errorf("RunTUI must not be called when base resolution fails")
	}
	// Guard the symptom of the pre-fix bug: `main` silently selected.
	if strings.Contains(stderr.String(), "main") && !strings.Contains(stderr.String(), "base not found") {
		t.Errorf("stderr mentions main without a 'base not found' diagnostic; built-in chain may have leaked in: %q", stderr.String())
	}
}

// TestResolveDiffSpec_NoCandidatesKeyUsesBuiltin is the symmetric guard:
// when `candidates:` is omitted entirely (only `default:` is set), the
// built-in BaseCandidates chain must still follow the default. This is the
// "default + built-in fallback" workflow documented in docs/config.md and
// the case CandidatesPresent must keep working — collapsing all
// len(slice) == 0 cases onto the replacement branch would regress it the
// other way.
//
// We use a default that does not resolve (`origin/release` — no such
// remote in the test repo) so the test isolates the "tail still runs"
// behavior: if the built-in chain were dropped, RunRoot would exit 1; the
// fact that it picks `main` proves the chain is intact.
func TestResolveDiffSpec_NoCandidatesKeyUsesBuiltin(t *testing.T) {
	dir, mainSHA := newRepo(t)
	writeRepoConfig(t, dir, `base:
  default: "origin/release"
`)
	chdir(t, dir)
	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0 (built-in fallback should still run when candidates key is omitted)", code)
	}
	if captured.Base.Ref != "main" {
		t.Errorf("base.Ref = %q, want main (built-in BaseCandidates should follow default)", captured.Base.Ref)
	}
	if captured.Base.SHA != mainSHA {
		t.Errorf("base.SHA = %q, want %q", captured.Base.SHA, mainSHA)
	}
}

// TestRunRoot_MalformedConfig_DegradesToAutoDetect guards the graceful
// degradation path: a config file that fails to parse must not block the TUI
// — the built-in BaseCandidates chain still resolves and the user sees a
// warning on stderr.
func TestRunRoot_MalformedConfig_DegradesToAutoDetect(t *testing.T) {
	dir, mainSHA := newRepo(t)
	writeRepoConfig(t, dir, "base: [unterminated\n")
	chdir(t, dir)
	var captured TUIOptions
	env, _, stderr := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if captured.Base.Ref != "main" {
		t.Errorf("base.Ref = %q, want main (auto-detect fallback)", captured.Base.Ref)
	}
	if captured.Base.SHA != mainSHA {
		t.Errorf("base.SHA = %q, want %q", captured.Base.SHA, mainSHA)
	}
	if !strings.Contains(stderr.String(), "sitatame:") {
		t.Errorf("expected config warning on stderr; got %q", stderr.String())
	}
}

// TestRunRoot_ConfigOnlyDir_DoesNotTriggerLegacyWarning is the issue #24
// allowlist case: a <repo>/.sitatame/ directory containing only config.yaml
// is the new legitimate state, not the legacy review-storage layout, so the
// warning must stay silent.
func TestRunRoot_ConfigOnlyDir_DoesNotTriggerLegacyWarning(t *testing.T) {
	dir, _ := newRepo(t)
	writeRepoConfig(t, dir, `base:
  default: "main"
`)
	chdir(t, dir)
	var captured TUIOptions
	env, _, stderr := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := stderr.String()
	if strings.Contains(got, "legacy") {
		t.Errorf("config-only .sitatame/ must not trigger legacy warning; got %q", got)
	}
	if strings.Contains(got, "To migrate drafts:") {
		t.Errorf("config-only .sitatame/ must not print migration hint; got %q", got)
	}
}

// TestRunRoot_ConfigPlusLegacyContents_StillWarns confirms that the
// allowlist is "strictly equal to" rather than "contains": if the directory
// has config.yaml *plus* leftover drafts/, the user still needs the legacy
// notice.
func TestRunRoot_ConfigPlusLegacyContents_StillWarns(t *testing.T) {
	dir, _ := newRepo(t)
	writeRepoConfig(t, dir, `base:
  default: "main"
`)
	// Pretend there are still legacy drafts on disk.
	if err := os.MkdirAll(filepath.Join(dir, ".sitatame", "drafts"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	var captured TUIOptions
	env, _, stderr := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "legacy") {
		t.Errorf("expected legacy warning with leftover drafts/; got %q", stderr.String())
	}
}

func TestRunRoot_BaseAutoFails(t *testing.T) {
	dir, _ := newRepo(t)
	mustGit(t, dir, "branch", "-m", "main", "trunk")
	chdir(t, dir)
	env := ttyEnv(os.Stdin, true)
	code := RunRoot(env, nil)
	if code != 1 {
		t.Errorf("expected exit 1 when auto base fails, got %d", code)
	}
}

func TestRunRoot_Staged_ResolvesAndDiffs(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "staged.txt")

	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, []string{"--staged"}); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if len(captured.Files) == 0 {
		t.Fatalf("no files captured")
	}
	if captured.Review.Head.Ref != "INDEX" {
		t.Errorf("Head.Ref = %q, want INDEX", captured.Review.Head.Ref)
	}
	if captured.Review.Head.SHA != "" {
		t.Errorf("Head.SHA = %q, want empty", captured.Review.Head.SHA)
	}
	if captured.Review.Base.Ref != "HEAD" {
		t.Errorf("Base.Ref = %q, want HEAD", captured.Review.Base.Ref)
	}
}

func TestRunRoot_Working_ResolvesAndDiffs(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	// Modify a tracked file (committed in newRepo) — this should appear under
	// --working without needing `git add`.
	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("b\nworking\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, []string{"--working"}); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if len(captured.Files) == 0 {
		t.Fatalf("no files captured")
	}
	if captured.Review.Head.Ref != "WORKTREE" {
		t.Errorf("Head.Ref = %q, want WORKTREE", captured.Review.Head.Ref)
	}
	if captured.Review.Head.SHA != "" {
		t.Errorf("Head.SHA = %q, want empty", captured.Review.Head.SHA)
	}
}

func TestRunRoot_FlagConflict_StagedAndWorking(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			t.Fatalf("RunTUI must not be called")
			return TUIResult{}, nil
		},
	}
	if code := RunRoot(env, []string{"--staged", "--working"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("stderr = %q, want contains 'mutually exclusive'", stderr.String())
	}
}

func TestRunRoot_FlagConflict_StagedAndBase(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			t.Fatalf("RunTUI must not be called")
			return TUIResult{}, nil
		},
	}
	if code := RunRoot(env, []string{"--staged", "main"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "explicit base") {
		t.Errorf("stderr = %q, want contains 'explicit base'", stderr.String())
	}
}

func TestRunRoot_UnknownFlag(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			t.Fatalf("RunTUI must not be called")
			return TUIResult{}, nil
		},
	}
	if code := RunRoot(env, []string{"--bogus"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown flag") {
		t.Errorf("stderr = %q, want contains 'unknown flag'", stderr.String())
	}
}

func TestRunRoot_Staged_NoChanges(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	// Clean index — no staged files.
	var stdout, stderr bytes.Buffer
	tuiCalled := false
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			tuiCalled = true
			return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
		},
	}
	if code := RunRoot(env, []string{"--staged"}); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if tuiCalled {
		t.Errorf("RunTUI must not be called when staged diff is empty")
	}
	if !strings.Contains(stderr.String(), "no staged changes") {
		t.Errorf("stderr = %q, want contains 'no staged changes'", stderr.String())
	}
}

// TestRunRoot_DetachedHEAD_UsesShaInBranchSlug guards the PR #42 P1 fix:
// when the repo is in detached HEAD, RunRoot must synthesise a branch label
// of "detached/<sha[:12]>" so the per-branch slug stays unique per detached
// session instead of collapsing onto the empty-branch fallback
// (BranchSlug("") = "branch__da39a3ee") and letting two unrelated detached
// sessions share state.
func TestRunRoot_DetachedHEAD_UsesShaInBranchSlug(t *testing.T) {
	dir, mainSHA := newRepo(t)
	// Detach HEAD at the main commit — base auto-detection will still resolve
	// `main` as the base ref.
	mustGit(t, dir, "checkout", "-q", "--detach", mainSHA)
	// newRepo left us on `feature` (one commit ahead). Replace that commit so
	// the detached HEAD points at a fresh, distinct SHA. We just write a new
	// file and commit it.
	if err := os.WriteFile(filepath.Join(dir, "detached-marker"), []byte("d\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "detached")
	detachedSHA := mustGit(t, dir, "rev-parse", "HEAD")

	chdir(t, dir)
	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	wantBranch := "detached/" + detachedSHA[:12]
	if captured.Review.Branch != wantBranch {
		t.Errorf("Review.Branch = %q, want %q", captured.Review.Branch, wantBranch)
	}
}

func TestDispatchHelp(t *testing.T) {
	// dispatch lives in main, not cmd; this test just checks RunSearch wiring.
	env := ttyEnv(os.Stdin, true)
	if got := RunSearch(env, nil); got != 2 {
		t.Errorf("RunSearch exit = %d, want 2 on missing pattern", got)
	}
}
