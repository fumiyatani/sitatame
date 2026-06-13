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

// TestRunRoot_AutoLoad_PreservesExtras pins the PR #70 codex P2 fix: the
// auto-load → re-save cycle must round-trip unknown top-level YAML keys
// (Extras). AI agents lean on PR #65's forward-compat mechanism to attach
// experimental metadata that sitatame itself does not model; if RunRoot
// re-builds the Review from a hand-picked subset of fields, those keys are
// silently dropped on the next save. We assert both:
//
//  1. the captured TUI Review carries the Extras forward, and
//  2. the final review file (post-Promote) still contains the key on disk.
func TestRunRoot_AutoLoad_PreservesExtras(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}

	paths := review.NewPaths(resolved, "feature")
	if err := os.MkdirAll(paths.DraftsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	draftBody := `---
schema: 1
id: 20260101T000000-extras
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
experimental_metadata: agent-tag-xyz
---
`
	draftPath := filepath.Join(paths.DraftsDir(), "20260101T000000-extras.md")
	if err := os.WriteFile(draftPath, []byte(draftBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitPromote}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if _, ok := captured.Review.Extras["experimental_metadata"]; !ok {
		t.Errorf("captured Review.Extras missing experimental_metadata key: %v", captured.Review.Extras)
	}

	// Read the promoted review file back from disk and confirm the key
	// survived the encode roundtrip.
	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if !strings.HasPrefix(pathLine, filepath.Join(projectRoot, "reviews")) {
		t.Fatalf("review file not under reviews/: %q", pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "experimental_metadata: agent-tag-xyz") {
		t.Errorf("promoted review must keep experimental_metadata; got:\n%s", string(b))
	}
}

// TestRunRoot_AutoLoad_PreservesCreatedAt guards the documented `created_at`
// field (PR #65 schema docs / PR #69). The pre-fix RunRoot copied only ID /
// Comments / ReviewComment off the loaded draft, which zeroed CreatedAt on the
// next save and silently rewrote the file with a different timestamp story
// than the user originally captured.
func TestRunRoot_AutoLoad_PreservesCreatedAt(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}

	paths := review.NewPaths(resolved, "feature")
	if err := os.MkdirAll(paths.DraftsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	const wantCreatedAt = "2026-01-02T03:04:05Z"
	draftBody := `---
schema: 1
id: 20260101T000000-createdat
created_at: ` + wantCreatedAt + `
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
---
`
	draftPath := filepath.Join(paths.DraftsDir(), "20260101T000000-createdat.md")
	if err := os.WriteFile(draftPath, []byte(draftBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitPromote}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if captured.Review.CreatedAt.IsZero() {
		t.Errorf("captured Review.CreatedAt is zero; want loaded timestamp")
	}
	if got := captured.Review.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"); got != wantCreatedAt {
		t.Errorf("captured CreatedAt = %q, want %q", got, wantCreatedAt)
	}

	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if !strings.HasPrefix(pathLine, filepath.Join(projectRoot, "reviews")) {
		t.Fatalf("review file not under reviews/: %q", pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), wantCreatedAt) {
		t.Errorf("promoted review must keep created_at %q; got:\n%s", wantCreatedAt, string(b))
	}
}

// TestRunRoot_AutoLoad_PreservesBody asserts that the Markdown body following
// the YAML front matter survives the resume → re-save cycle. The pre-fix
// RunRoot rebuilt a fresh Review and never copied Body, so any handwritten
// narrative the user attached to the draft was wiped on the next launch.
func TestRunRoot_AutoLoad_PreservesBody(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}

	paths := review.NewPaths(resolved, "feature")
	if err := os.MkdirAll(paths.DraftsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	const wantBody = "## Reviewer notes\n\nThis body must survive resume.\n"
	draftBody := `---
schema: 1
id: 20260101T000000-body
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
---

` + wantBody
	draftPath := filepath.Join(paths.DraftsDir(), "20260101T000000-body.md")
	if err := os.WriteFile(draftPath, []byte(draftBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitPromote}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(captured.Review.Body, "Reviewer notes") ||
		!strings.Contains(captured.Review.Body, "This body must survive resume.") {
		t.Errorf("captured Review.Body lost content: %q", captured.Review.Body)
	}

	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if !strings.HasPrefix(pathLine, filepath.Join(projectRoot, "reviews")) {
		t.Fatalf("review file not under reviews/: %q", pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Reviewer notes") ||
		!strings.Contains(string(b), "This body must survive resume.") {
		t.Errorf("promoted review must keep body content; got:\n%s", string(b))
	}
}

// TestRunRoot_AutoLoad_PreservesFiles pins the PR #70 round-2 codex P2 fix:
// the auto-load → re-save cycle must round-trip the loaded draft's `files:`
// list (FileMeta entries). The pre-fix RunRoot wiped `r.Files = nil` before
// handing the Review to the TUI, and `SaveDraft → Encode` then serialised an
// empty `files:` list, so every resume → save silently dropped the original
// diff-snapshot metadata recorded at draft creation time.
//
// We assert on both the captured TUI Review (the in-memory carry) and the
// promoted file on disk (the encode round-trip), mirroring the structure of
// TestRunRoot_AutoLoad_PreservesExtras.
func TestRunRoot_AutoLoad_PreservesFiles(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}

	paths := review.NewPaths(resolved, "feature")
	if err := os.MkdirAll(paths.DraftsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// The path here ("src/auth.ts") deliberately does NOT exist in the live
	// diff (newRepo only commits `a` / `b`). That is the point: the loaded
	// Files snapshot is *historical* metadata from when the draft was first
	// saved, and we want to prove it survives resume even when the current
	// diff has nothing to say about it. A diff refresh / merge is tracked as
	// a follow-up.
	draftBody := `---
schema: 1
id: 20260101T000000-files
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
files:
  - path: src/auth.ts
    blob_base: 4e5f6a7b
    blob_head: 9c8d7e6f
    status: modified
---
`
	draftPath := filepath.Join(paths.DraftsDir(), "20260101T000000-files.md")
	if err := os.WriteFile(draftPath, []byte(draftBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitPromote}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if len(captured.Review.Files) != 1 {
		t.Fatalf("captured Review.Files = %d, want 1 (loaded from draft)", len(captured.Review.Files))
	}
	got := captured.Review.Files[0]
	if got.Path != "src/auth.ts" || got.BlobBase != "4e5f6a7b" || got.BlobHead != "9c8d7e6f" || got.Status != "modified" {
		t.Errorf("captured Review.Files[0] = %+v, want path/blobs/status preserved", got)
	}

	// Confirm the promoted file on disk still carries the files block.
	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if !strings.HasPrefix(pathLine, filepath.Join(projectRoot, "reviews")) {
		t.Fatalf("review file not under reviews/: %q", pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	for _, frag := range []string{"path: src/auth.ts", "blob_base: 4e5f6a7b", "blob_head: 9c8d7e6f", "status: modified"} {
		if !strings.Contains(content, frag) {
			t.Errorf("promoted review missing %q; got:\n%s", frag, content)
		}
	}
}

// TestRunRoot_AutoLoad_PreservesFileExtras is the per-FileMeta Extras analogue
// of TestRunRoot_AutoLoad_PreservesExtras. AI agents stash forward-compat
// keys on individual file entries (PR #65 FileMeta.Extras). Wiping Files on
// resume silently dropped those keys; this test pins that they now survive
// the resume → save round-trip both in memory and on disk.
func TestRunRoot_AutoLoad_PreservesFileExtras(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}

	paths := review.NewPaths(resolved, "feature")
	if err := os.MkdirAll(paths.DraftsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	draftBody := `---
schema: 1
id: 20260101T000000-fileextras
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
files:
  - path: src/auth.ts
    blob_base: 4e5f6a7b
    blob_head: 9c8d7e6f
    status: modified
    agent_annotation: needs-second-look
---
`
	draftPath := filepath.Join(paths.DraftsDir(), "20260101T000000-fileextras.md")
	if err := os.WriteFile(draftPath, []byte(draftBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitPromote}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if len(captured.Review.Files) != 1 {
		t.Fatalf("captured Review.Files = %d, want 1 (loaded from draft)", len(captured.Review.Files))
	}
	if _, ok := captured.Review.Files[0].Extras["agent_annotation"]; !ok {
		t.Errorf("captured Review.Files[0].Extras missing agent_annotation key: %v", captured.Review.Files[0].Extras)
	}

	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if !strings.HasPrefix(pathLine, filepath.Join(projectRoot, "reviews")) {
		t.Fatalf("review file not under reviews/: %q", pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "agent_annotation: needs-second-look") {
		t.Errorf("promoted review must keep per-file agent_annotation; got:\n%s", string(b))
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
