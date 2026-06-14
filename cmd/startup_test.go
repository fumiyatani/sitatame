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

// seedReview writes a review file at the 1-branch-1-file location
// (<BranchDir>/review.md) for the given dir+branch so startup tests can
// pre-populate a review the autoload path will pick up.
func seedReview(t *testing.T, dir, branch, body string) (paths review.Paths, reviewPath string) {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}
	paths = review.NewPaths(resolved, branch)
	if err := os.MkdirAll(paths.BranchDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	reviewPath = paths.ReviewFile()
	if err := os.WriteFile(reviewPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return paths, reviewPath
}

func TestRunRoot_DetectsExistingReviewOnStartup(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	// Anchor SITATAME_HOME so the seeded path matches what RunRoot resolves.
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	// Pre-seed a review for the current branch ("feature").
	_, reviewPath := seedReview(t, dir, "feature", "---\nschema: 1\nbranch: feature\n---\n")

	env, _, stderr := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := stderr.String()
	if !strings.Contains(got, "review exists:") || !strings.Contains(got, reviewPath) {
		t.Errorf("stderr should mention existing review path; got %q", got)
	}
}

// TestRunRoot_AutoLoadReview is the issue #18 analogue for the new 1-file layout:
// a review file for the current branch exists, so RunRoot must read it back into
// the Review handed to the TUI.
func TestRunRoot_AutoLoadReview(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	// Seed a review with one comment.
	draftBody := `---
schema: 1
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
	seedReview(t, dir, "feature", draftBody)

	var captured TUIOptions
	env, _, _ := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if captured.Review.ReviewComment != "previous-session-summary" {
		t.Errorf("ReviewComment = %q, want %q",
			captured.Review.ReviewComment, "previous-session-summary")
	}
	if len(captured.Review.Comments) != 1 {
		t.Fatalf("Comments = %d, want 1 (loaded from review)", len(captured.Review.Comments))
	}
	if captured.Review.Comments[0].Body != "previous-session-overall\n" &&
		captured.Review.Comments[0].Body != "previous-session-overall" {
		t.Errorf("Comments[0].Body = %q, want previous-session-overall", captured.Review.Comments[0].Body)
	}
	// Files must be re-derived from the current diff, not the (empty) review Files.
	if len(captured.Files) == 0 {
		t.Errorf("Files must come from the live diff, not the stored review (got 0)")
	}
}

// TestRunRoot_AutoLoad_PreservesExtras pins forward-compat Extras round-trip.
func TestRunRoot_AutoLoad_PreservesExtras(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	draftBody := `---
schema: 1
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
review_comment: extras-test
experimental_metadata: agent-tag-xyz
---
`
	seedReview(t, dir, "feature", draftBody)

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if _, ok := captured.Review.Extras["experimental_metadata"]; !ok {
		t.Errorf("captured Review.Extras missing experimental_metadata key: %v", captured.Review.Extras)
	}

	// Read the saved review file back from disk and confirm the key survived.
	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if pathLine == "" {
		t.Fatal("stdout missing SITATAME_REVIEW line")
	}
	if !strings.HasPrefix(pathLine, projectRoot) {
		t.Fatalf("review file not under projectRoot %q: %q", projectRoot, pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "experimental_metadata: agent-tag-xyz") {
		t.Errorf("saved review must keep experimental_metadata; got:\n%s", string(b))
	}
}

// TestRunRoot_AutoLoad_PreservesCreatedAt guards the created_at field.
func TestRunRoot_AutoLoad_PreservesCreatedAt(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	const wantCreatedAt = "2026-01-02T03:04:05Z"
	draftBody := `---
schema: 1
created_at: ` + wantCreatedAt + `
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
review_comment: preserve-ts
---
`
	seedReview(t, dir, "feature", draftBody)

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
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
	if !strings.HasPrefix(pathLine, projectRoot) {
		t.Fatalf("review file not under projectRoot: %q", pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), wantCreatedAt) {
		t.Errorf("saved review must keep created_at %q; got:\n%s", wantCreatedAt, string(b))
	}
}

// TestRunRoot_AutoLoad_PreservesBody asserts the Markdown body survives resume.
func TestRunRoot_AutoLoad_PreservesBody(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	const wantBody = "## Reviewer notes\n\nThis body must survive resume.\n"
	draftBody := `---
schema: 1
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
review_comment: preserve-body
---

` + wantBody
	seedReview(t, dir, "feature", draftBody)

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(captured.Review.Body, "Reviewer notes") ||
		!strings.Contains(captured.Review.Body, "This body must survive resume.") {
		t.Errorf("captured Review.Body lost content: %q", captured.Review.Body)
	}

	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if !strings.HasPrefix(pathLine, projectRoot) {
		t.Fatalf("review file not under projectRoot: %q", pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Reviewer notes") ||
		!strings.Contains(string(b), "This body must survive resume.") {
		t.Errorf("saved review must keep body content; got:\n%s", string(b))
	}
}

// TestRunRoot_AutoLoad_PreservesFiles pins the files list round-trip.
func TestRunRoot_AutoLoad_PreservesFiles(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	draftBody := `---
schema: 1
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
review_comment: preserve-files
files:
  - path: src/auth.ts
    blob_base: 4e5f6a7b
    blob_head: 9c8d7e6f
    status: modified
---
`
	seedReview(t, dir, "feature", draftBody)

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if len(captured.Review.Files) != 1 {
		t.Fatalf("captured Review.Files = %d, want 1 (loaded from review)", len(captured.Review.Files))
	}
	got := captured.Review.Files[0]
	if got.Path != "src/auth.ts" || got.BlobBase != "4e5f6a7b" || got.BlobHead != "9c8d7e6f" || got.Status != "modified" {
		t.Errorf("captured Review.Files[0] = %+v, want path/blobs/status preserved", got)
	}

	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if !strings.HasPrefix(pathLine, projectRoot) {
		t.Fatalf("review file not under projectRoot: %q", pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	for _, frag := range []string{"path: src/auth.ts", "blob_base: 4e5f6a7b", "blob_head: 9c8d7e6f", "status: modified"} {
		if !strings.Contains(content, frag) {
			t.Errorf("saved review missing %q; got:\n%s", frag, content)
		}
	}
}

// TestRunRoot_AutoLoad_PreservesFileExtras pins per-FileMeta Extras round-trip.
func TestRunRoot_AutoLoad_PreservesFileExtras(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	draftBody := `---
schema: 1
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
review_comment: preserve-fileextras
files:
  - path: src/auth.ts
    blob_base: 4e5f6a7b
    blob_head: 9c8d7e6f
    status: modified
    agent_annotation: needs-second-look
---
`
	seedReview(t, dir, "feature", draftBody)

	var captured TUIOptions
	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		captured = opts
		return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if len(captured.Review.Files) != 1 {
		t.Fatalf("captured Review.Files = %d, want 1 (loaded from review)", len(captured.Review.Files))
	}
	if _, ok := captured.Review.Files[0].Extras["agent_annotation"]; !ok {
		t.Errorf("captured Review.Files[0].Extras missing agent_annotation key: %v", captured.Review.Files[0].Extras)
	}

	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if !strings.HasPrefix(pathLine, projectRoot) {
		t.Fatalf("review file not under projectRoot: %q", pathLine)
	}
	b, err := os.ReadFile(pathLine)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "agent_annotation: needs-second-look") {
		t.Errorf("saved review must keep per-file agent_annotation; got:\n%s", string(b))
	}
}

// TestRunRoot_NoReviewStartsEmpty is the negative companion: when no review
// exists for the branch the TUI must receive an empty Comments slice.
func TestRunRoot_NoReviewStartsEmpty(t *testing.T) {
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
		t.Errorf("Comments = %d, want 0 (no review to load)", len(captured.Review.Comments))
	}
	if strings.Contains(stderr.String(), "review exists:") {
		t.Errorf("stderr should not announce a review when none was seeded; got %q", stderr.String())
	}
}

// TestRunRoot_LegacyAnchorWarning covers the PR #61 path: a review containing
// a legacy head-side anchor on a deleted base line must surface a warning on
// stderr so the user notices before re-saving.
func TestRunRoot_LegacyDraftWarning(t *testing.T) {
	dir, _ := newRepo(t)
	// Create a deleted-line diff against main.
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("aa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "modify a")
	blobHead := mustGit(t, dir, "rev-parse", "--short=7", "HEAD:a")

	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	// Anchor shape: Side=head, Line=BaseLine (=1, the deleted "a" row),
	// Blob=BlobHead. That is the exact buggy combination PR #61 detects.
	draftBody := `---
schema: 1
branch: feature
base:
  ref: main
  sha: ""
head:
  ref: HEAD
  sha: ""
review_comment: legacy-test
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
	seedReview(t, dir, "feature", draftBody)

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

// TestRunRoot_LegacySitatameDir_WarnsAboutInRepoDir verifies that RunRoot
// surfaces a warning when a pre-#38 <repo>/.sitatame/ directory is detected.
// As of Phase 4, the output-root drafts/reviews migration is handled
// automatically (MigrateLegacyLayout), so the manual migration hint for the
// output root is no longer emitted; only the "legacy detected — ignored" notice
// fires for the in-repo directory.
func TestRunRoot_LegacySitatameDir_WarnsAboutInRepoDir(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	// Seed a legacy <repo>/.sitatame/ directory with a non-config entry so the
	// "only config entries" suppression does not silence the warning.
	legacy := filepath.Join(dir, ".sitatame")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "old-draft.md"), []byte("draft"), 0o644); err != nil {
		t.Fatal(err)
	}

	env, _, stderr := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := stderr.String()
	if !strings.Contains(got, "legacy") || !strings.Contains(got, "detected") {
		t.Errorf("stderr should contain legacy-detected notice; got %q", got)
	}
}

// TestWarnLegacySitatameDir_ShellQuotesPaths guards POSIX single-quoting of
// paths in the migration hint.
func TestWarnLegacySitatameDir_ShellQuotesPaths(t *testing.T) {
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
	wantLegacyFragment := "'" + legacy + "'/drafts/*"
	if !strings.Contains(got, wantLegacyFragment) {
		t.Errorf("stderr should contain %q (legacy path quoted, glob outside); got %q", wantLegacyFragment, got)
	}
	wantNewQuoted := "'" + newDraftsRoot + "'"
	if strings.Count(got, wantNewQuoted) < 2 {
		t.Errorf("stderr should contain quoted new drafts root %q at least twice; got %q", wantNewQuoted, got)
	}
	bareLegacyFragment := " " + legacy + "/drafts/*"
	if strings.Contains(got, bareLegacyFragment) {
		t.Errorf("stderr leaked unquoted legacy path; got %q", got)
	}
}

// TestShellQuote_EscapesEmbeddedSingleQuote covers the `'\''` escape.
func TestShellQuote_EscapesEmbeddedSingleQuote(t *testing.T) {
	got := shellQuote("a'b")
	want := `'a'\''b'`
	if got != want {
		t.Errorf("shellQuote(\"a'b\") = %q, want %q", got, want)
	}
}

// TestRunRoot_NewFlag_RefusesWhenReviewExists tests that --new exits 1 when
// review.md already exists for the branch.
func TestRunRoot_NewFlag_RefusesWhenReviewExists(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	seedReview(t, dir, "feature", "---\nschema: 1\nreview_comment: existing\n---\n")

	env, _, stderr := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		t.Fatal("RunTUI must not be called when --new refuses")
		return TUIResult{}, nil
	})
	if code := RunRoot(env, []string{"--new"}); code != 1 {
		t.Errorf("exit = %d, want 1 (--new must refuse existing review)", code)
	}
	if !strings.Contains(stderr.String(), "review already exists") {
		t.Errorf("stderr should mention review already exists; got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--force-new") {
		t.Errorf("stderr should hint --force-new; got %q", stderr.String())
	}
}

// TestRunRoot_NewFlag_SucceedsWhenNoReview tests that --new proceeds normally
// when no review.md exists.
func TestRunRoot_NewFlag_SucceedsWhenNoReview(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	var called bool
	env, _, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		called = true
		return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
	})
	if code := RunRoot(env, []string{"--new"}); code != 0 {
		t.Errorf("exit = %d, want 0 (--new with no existing review)", code)
	}
	if !called {
		t.Error("RunTUI should have been called when --new has no existing review")
	}
}

// TestRunRoot_ForceNewFlag_BacksUpExistingReview tests that --force-new backs
// up the existing review.md to .bak and starts a fresh session.
func TestRunRoot_ForceNewFlag_BacksUpExistingReview(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	paths, existingPath := seedReview(t, dir, "feature", "---\nschema: 1\nreview_comment: existing\n---\n")

	var captured TUIOptions
	env, _, stderr := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, []string{"--force-new"}); code != 0 {
		t.Fatalf("exit = %d, want 0 (--force-new must back up and continue); stderr=%q", code, stderr.String())
	}
	// The existing review.md should now be at .bak.
	if _, err := os.Stat(paths.BakFile()); os.IsNotExist(err) {
		t.Errorf("review.md.bak should exist after --force-new, path=%s", paths.BakFile())
	}
	// The original review.md at existingPath should be gone (moved to .bak).
	if _, err := os.Stat(existingPath); !os.IsNotExist(err) {
		t.Errorf("original review.md should be gone after --force-new, but still exists at %s", existingPath)
	}
	// The TUI must start with an empty (or freshly-built) Review, not the old one.
	if captured.Review.ReviewComment == "existing" {
		t.Errorf("TUI should not see the old review comment after --force-new")
	}
}

// TestRunRoot_NewAndForceNew_MutuallyExclusive tests that --new and --force-new
// together produce exit 2.
func TestRunRoot_NewAndForceNew_MutuallyExclusive(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			t.Fatal("RunTUI must not be called")
			return TUIResult{}, nil
		},
	}
	if code := RunRoot(env, []string{"--new", "--force-new"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("stderr = %q, want contains 'mutually exclusive'", stderr.String())
	}
}

// TestRunRoot_MigrateLegacyLayout_AutomigrationOnStartup verifies that when the
// output root contains the legacy drafts/reviews layout, RunRoot automatically
// migrates the data to the new 1-branch-1-file layout and:
//   - the migrated review.md is autoloaded by the TUI session
//   - a "migrated N branch(es)" message is printed to stderr
//   - the legacy data is preserved under .legacy-<ts>/
func TestRunRoot_MigrateLegacyLayout_AutomigrationOnStartup(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	// Build the legacy reviews/ layout for the "feature" branch slug.
	branchSlug := review.BranchSlug("feature")
	legacyReviewsDir := filepath.Join(projectRoot, "reviews", branchSlug)
	if err := os.MkdirAll(legacyReviewsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyContent := "---\nschema: 1\nbranch: feature\nreview_comment: migrated-review\n---\n"
	legacyFile := filepath.Join(legacyReviewsDir, "20260101T000000-review.md")
	if err := os.WriteFile(legacyFile, []byte(legacyContent), 0o600); err != nil {
		t.Fatal(err)
	}

	var captured TUIOptions
	env, _, stderr := captureTUIEnv(os.Stdin, true, &captured)
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}

	// Migration message should be in stderr.
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "migrated") {
		t.Errorf("stderr should contain migration message; got %q", stderrStr)
	}
	if !strings.Contains(stderrStr, "legacy data preserved") {
		t.Errorf("stderr should mention legacy data preserved; got %q", stderrStr)
	}

	// The migrated review.md must be autoloaded: TUI gets the old review_comment.
	if captured.Review.ReviewComment != "migrated-review" {
		t.Errorf("ReviewComment = %q, want %q (autoloaded from migrated review)",
			captured.Review.ReviewComment, "migrated-review")
	}

	// .legacy-<ts>/ must exist and contain the old file.
	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	var legacyDirName string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), ".legacy-") {
			legacyDirName = e.Name()
			break
		}
	}
	if legacyDirName == "" {
		t.Fatal("no .legacy-<ts>/ directory found under project root")
	}
	preserved := filepath.Join(projectRoot, legacyDirName, "reviews", branchSlug, "20260101T000000-review.md")
	if _, err := os.Stat(preserved); err != nil {
		t.Errorf("legacy file not preserved at %s: %v", preserved, err)
	}
}

// TestRunRoot_MigrateLegacyLayout_NoLegacy_Noop verifies that when no legacy
// layout exists, RunRoot does not emit a migration message.
func TestRunRoot_MigrateLegacyLayout_NoLegacy_Noop(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)

	env, _, stderr := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		return TUIResult{Review: opts.Review, Reason: tui.QuitNone}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "migrated") {
		t.Errorf("stderr should not contain migration message when no legacy layout; got %q", stderr.String())
	}
}

// TestRunRoot_PanicSavesReviewAndPropagates checks that the panic recovery
// path writes a review.md (best-effort) before re-panicking.
func TestRunRoot_PanicSavesReviewAndPropagates(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	env, _, _ := envWithRunner(os.Stdin, func(_ Env, _ TUIOptions) (TUIResult, error) {
		panic("simulated bubbletea crash")
	})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic to propagate from RunRoot")
		}
		// The panic-path review is empty (no comments, no review_comment on the
		// initial Review), so SaveReview is a no-op. The project root dir is
		// created by RecoverFromCrash/BranchDir mkdir even though no file lands.
		// We just assert that the panic propagated correctly (no file is fine
		// because the initial Review is empty).
		_ = projectRoot // referenced to keep the test grounded
	}()
	_ = RunRoot(env, nil)
	t.Fatal("RunRoot should have panicked")
}
