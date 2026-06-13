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

// TestRunRoot_AutoLoadDraft is the issue #18 happy path: a draft file for the
// current branch exists, so RunRoot must read it back into the Review handed
// to the TUI. Without auto-load (the pre-fix behaviour) the printed "draft
// exists" notice was misleading because the TUI started with an empty
// Comments slice and the user lost their prior review state on every rerun.
func TestRunRoot_AutoLoadDraft(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}

	paths := review.NewPaths(resolved, "feature")
	if err := os.MkdirAll(paths.DraftsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// Seed a draft with one review-level comment and one inline comment.
	// review-level is enough to prove the merge: it has no anchor metadata
	// for Validate to touch, so it survives load verbatim regardless of the
	// current diff.
	draftBody := `---
schema: 1
id: 20260101T000000-seed
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
review_comment: previous-session-summary
comments:
  - anchor_id: 11111111-1111-1111-1111-111111111111
    state: open
    kind: review
    path: ""
    body: previous-session-overall
---
`
	draftPath := filepath.Join(paths.DraftsDir(), "20260101T000000-seed.md")
	if err := os.WriteFile(draftPath, []byte(draftBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if captured.Review.ID != "20260101T000000-seed" {
		t.Errorf("Review.ID = %q, want %q (so re-save writes back to the same draft file)",
			captured.Review.ID, "20260101T000000-seed")
	}
	if captured.Review.ReviewComment != "previous-session-summary" {
		t.Errorf("ReviewComment = %q, want %q",
			captured.Review.ReviewComment, "previous-session-summary")
	}
	if len(captured.Review.Comments) != 1 {
		t.Fatalf("Comments = %d, want 1 (loaded from draft)", len(captured.Review.Comments))
	}
	if captured.Review.Comments[0].Body != "previous-session-overall\n" &&
		captured.Review.Comments[0].Body != "previous-session-overall" {
		t.Errorf("Comments[0].Body = %q, want previous-session-overall", captured.Review.Comments[0].Body)
	}
	// Files must be re-derived from the current diff, not the (empty) draft Files.
	if len(captured.Files) == 0 {
		t.Errorf("Files must come from the live diff, not the draft (got 0)")
	}
}

// TestRunRoot_NoDraftStartsEmpty is the negative companion: when no draft
// exists for the branch the TUI must receive an empty Comments slice.
// Without this guard a regression where the auto-load path ran on a missing
// file (e.g. swallowed the not-found error and merged zero values) would go
// unnoticed.
func TestRunRoot_NoDraftStartsEmpty(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	var captured TUIOptions
	env, _, stderr := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if len(captured.Review.Comments) != 0 {
		t.Errorf("Comments = %d, want 0 (no draft to load)", len(captured.Review.Comments))
	}
	if captured.Review.ID != "" {
		t.Errorf("Review.ID = %q, want empty (no draft to inherit from)", captured.Review.ID)
	}
	if strings.Contains(stderr.String(), "draft exists:") {
		t.Errorf("stderr should not announce a draft when none was seeded; got %q", stderr.String())
	}
}

// TestRunRoot_LegacyDraftWarning covers the PR #61 path: a draft saved under
// the buggy issue #36 / #19 modal had Side=head pointing at a deleted base
// line. When that draft is auto-loaded on startup, the legacy-anchor detector
// in ValidateWithWarnings must surface the issue on stderr so the user notices
// before re-saving over the bad shape.
func TestRunRoot_LegacyDraftWarning(t *testing.T) {
	dir, _ := newRepo(t)
	// Create a deleted-line diff against main: rewrite "a" with new content so
	// the BaseLine=1 row of `a` (containing "a") becomes a `-` row in the diff.
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("aa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "modify a")
	// Capture the head blob of `a` so the legacy anchor can pin Blob=BlobHead
	// (the exact fingerprint emitLegacyAnchorWarning matches on).
	// `git diff --raw` (which gitx.Diff parses) emits abbreviated SHAs, so we
	// have to ask rev-parse for the same abbreviation. Using the full 40-char
	// SHA here would leave a.Blob != f.BlobHead and silently skip the warning
	// — that is precisely the failure mode that surfaced before we matched
	// the abbreviation length.
	blobHead := mustGit(t, dir, "rev-parse", "--short=7", "HEAD:a")

	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}

	paths := review.NewPaths(resolved, "feature")
	if err := os.MkdirAll(paths.DraftsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// Anchor shape: Side=head, Line=BaseLine (=1, the deleted "a" row),
	// Blob=BlobHead. That is the exact buggy combination PR #61 detects.
	draftBody := `---
schema: 1
id: 20260101T000000-legacy
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
comments:
  - anchor_id: 22222222-2222-2222-2222-222222222222
    state: open
    kind: line
    path: a
    side: head
    blob: ` + blobHead + `
    line: 1
    body: legacy-anchor
---
`
	draftPath := filepath.Join(paths.DraftsDir(), "20260101T000000-legacy.md")
	if err := os.WriteFile(draftPath, []byte(draftBody), 0o644); err != nil {
		t.Fatal(err)
	}

	env, _, stderr := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "legacy head-side anchor") {
		t.Errorf("stderr should contain legacy-anchor warning from ValidateWithWarnings; got %q", stderr.String())
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
