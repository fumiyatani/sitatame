package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/review"
	"github.com/fumiyatani/sitatame/internal/tui"
)

func TestRunRoot_DetectsExistingDraftOnStartup(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	// Anchor SITATAME_HOME so the seeded path matches what RunRoot resolves.
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)
	// Mirror the resolved path that gitx.Discover hands to NewPaths inside
	// RunRoot, otherwise the seeded draft lands under a different
	// project-slug and DetectDraft won't see it.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}

	// Pre-seed a draft for the current branch ("feature").
	paths := review.NewPaths(resolved, "feature")
	if err := os.MkdirAll(paths.DraftsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	draftPath := filepath.Join(paths.DraftsDir(), "20260101T000000-seed.md")
	if err := os.WriteFile(draftPath, []byte("---\nschema: 1\nid: 20260101T000000-seed\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env, _, stderr := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := stderr.String()
	if !strings.Contains(got, "draft exists:") || !strings.Contains(got, draftPath) {
		t.Errorf("stderr should mention existing draft path; got %q", got)
	}
}

func TestRunRoot_LegacySitatameDir_HintIncludesMkdir(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	// Seed a legacy <repo>/.sitatame/ directory so warnLegacySitatameDir
	// fires. We don't need drafts inside it — the warning is keyed on the
	// directory existing at all.
	legacy := filepath.Join(dir, ".sitatame")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}

	env, _, stderr := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := stderr.String()
	if !strings.Contains(got, "To migrate drafts:") {
		t.Fatalf("stderr should contain migration hint; got %q", got)
	}
	// The hint must bundle `mkdir -p` so the first upgrade does not fail on
	// a `mv` into a not-yet-created drafts root.
	if !strings.Contains(got, "mkdir -p ") {
		t.Errorf("migration hint should include `mkdir -p ` to create the new drafts root; got %q", got)
	}
	if !strings.Contains(got, " && mv ") {
		t.Errorf("migration hint should chain mkdir and mv with `&&`; got %q", got)
	}
}

// TestWarnLegacySitatameDir_ShellQuotesPaths guards the PR #42 round-4 P2 fix:
// the migration hint is meant to be copy-pasted into a shell, so paths must be
// POSIX single-quoted to survive spaces and shell metacharacters. The mv glob
// `/drafts/*` must stay *outside* the closing quote of the legacy path so the
// shell still expands it — otherwise a literal `*` would be passed to mv and
// the command would fail with "no such file".
func TestWarnLegacySitatameDir_ShellQuotesPaths(t *testing.T) {
	// Build a legacy directory under a path with a space — this is the
	// canonical case that an unquoted hint would split on.
	base := t.TempDir()
	legacy := filepath.Join(base, "with space", ".sitatame")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	newDraftsRoot := filepath.Join(base, "out put", "slug", "drafts")

	var stderr bytes.Buffer
	env := Env{Stderr: &stderr}
	warnLegacySitatameDir(env, legacy, newDraftsRoot)

	got := stderr.String()
	// The legacy path must appear single-quoted, with `/drafts/*` left outside
	// the quotes so the shell still globs.
	wantLegacyFragment := "'" + legacy + "'/drafts/*"
	if !strings.Contains(got, wantLegacyFragment) {
		t.Errorf("stderr should contain %q (legacy path quoted, glob outside); got %q", wantLegacyFragment, got)
	}
	// The new drafts root appears twice (mkdir + mv target), both quoted.
	wantNewQuoted := "'" + newDraftsRoot + "'"
	if strings.Count(got, wantNewQuoted) < 2 {
		t.Errorf("stderr should contain quoted new drafts root %q at least twice; got %q", wantNewQuoted, got)
	}
	// Sanity: the unquoted form must not appear on its own (would mean a path
	// leaked through without quoting). We check the legacy fragment specifically
	// because a bare space-containing path with no surrounding quote is the
	// failure mode we're guarding against.
	bareLegacyFragment := " " + legacy + "/drafts/*"
	if strings.Contains(got, bareLegacyFragment) {
		t.Errorf("stderr leaked unquoted legacy path; got %q", got)
	}
}

// TestShellQuote_EscapesEmbeddedSingleQuote covers the one tricky case in the
// `'\''` escape sequence: a path containing a literal single quote must be
// split into two quoted segments separated by an escaped quote.
func TestShellQuote_EscapesEmbeddedSingleQuote(t *testing.T) {
	got := shellQuote("a'b")
	want := `'a'\''b'`
	if got != want {
		t.Errorf("shellQuote(\"a'b\") = %q, want %q", got, want)
	}
}

func TestRunRoot_BaseAutoFails_HintsExplicitArg(t *testing.T) {
	dir, _ := newRepo(t)
	// Rename main so no auto-base candidate resolves.
	mustGit(t, dir, "branch", "-m", "main", "trunk")
	chdir(t, dir)

	env := ttyEnv(os.Stdin, true)
	stderr := env.Stderr.(interface{ String() string })
	if code := RunRoot(env, nil); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	msg := stderr.String()
	if !strings.Contains(msg, "base not found") {
		t.Errorf("stderr missing base-not-found phrase: %q", msg)
	}
	if !strings.Contains(msg, "sitatame <base>") {
		t.Errorf("stderr should hint explicit base argument: %q", msg)
	}
}
