package review

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchSlug_PRDExample(t *testing.T) {
	t.Parallel()

	branch := "feature/auth-refactor"
	got := BranchSlug(branch)

	sum := sha1.Sum([]byte(branch))
	wantHash := hex.EncodeToString(sum[:])[:8]
	want := "feature_auth-refactor__" + wantHash
	if got != want {
		t.Errorf("BranchSlug(%q) = %q, want %q", branch, got, want)
	}
	if !strings.HasPrefix(got, "feature_auth-refactor__") {
		t.Errorf("expected unsafe '/' replaced with '_': %q", got)
	}
}

func TestBranchSlug_TruncatesTo32(t *testing.T) {
	t.Parallel()

	branch := strings.Repeat("a", 50)
	got := BranchSlug(branch)
	parts := strings.Split(got, "__")
	if len(parts) != 2 {
		t.Fatalf("expected one '__' separator, got %q", got)
	}
	if len(parts[0]) != 32 {
		t.Errorf("prefix length = %d, want 32", len(parts[0]))
	}
	if len(parts[1]) != 8 {
		t.Errorf("hash length = %d, want 8", len(parts[1]))
	}
}

func TestBranchSlug_AllUnsafeFallsBackToBranch(t *testing.T) {
	t.Parallel()

	branch := "////"
	got := BranchSlug(branch)
	if !strings.HasPrefix(got, "branch__") {
		t.Errorf("expected 'branch__' prefix, got %q", got)
	}
}

func TestBranchSlug_EmptyBranch(t *testing.T) {
	t.Parallel()

	got := BranchSlug("")
	sum := sha1.Sum(nil)
	want := "branch__" + hex.EncodeToString(sum[:])[:8]
	if got != want {
		t.Errorf("BranchSlug(\"\") = %q, want %q", got, want)
	}
}

func TestBranchSlug_DeterministicAcrossCalls(t *testing.T) {
	t.Parallel()
	if BranchSlug("dev") != BranchSlug("dev") {
		t.Error("BranchSlug should be deterministic")
	}
	if BranchSlug("dev") == BranchSlug("Dev") {
		t.Error("BranchSlug should differ for different branches")
	}
}

func TestPaths(t *testing.T) {
	t.Parallel()

	// Use NewPathsWithRoot so the test is independent of $HOME and
	// $SITATAME_HOME. The output root is treated as opaque — what matters is
	// that everything resolves under <root>/<project-slug>/{reviews,drafts}.
	p := NewPathsWithRoot("/out", "/repo", "feature/x")
	if p.Slug == "" || !strings.Contains(p.Slug, "__") {
		t.Errorf("unexpected branch slug: %q", p.Slug)
	}
	if p.ProjectSlug == "" || !strings.Contains(p.ProjectSlug, "__") {
		t.Errorf("unexpected project slug: %q", p.ProjectSlug)
	}
	projectRoot := filepath.Join("/out", p.ProjectSlug)
	want := filepath.Join(projectRoot, "reviews", p.Slug)
	if got := p.ReviewsDir(); got != want {
		t.Errorf("ReviewsDir() = %q, want %q", got, want)
	}
	want = filepath.Join(projectRoot, "drafts", p.Slug)
	if got := p.DraftsDir(); got != want {
		t.Errorf("DraftsDir() = %q, want %q", got, want)
	}
	want = filepath.Join(p.ReviewsDir(), "20260501T000000-x.md")
	if got := p.ReviewFile("20260501T000000-x"); got != want {
		t.Errorf("ReviewFile() = %q, want %q", got, want)
	}
	want = filepath.Join(p.DraftsDir(), "20260501T000000-x.md")
	if got := p.DraftFile("20260501T000000-x"); got != want {
		t.Errorf("DraftFile() = %q, want %q", got, want)
	}
}

// TestNewPathsWithRoot_EmptyBranchUsesDetachedSlug guards against regressing
// the detached-HEAD fix: when RunRoot encounters `branch, _ := CurrentBranch()`
// returning "" (detached HEAD), the per-branch helpers must still resolve to a
// branch-scoped directory so SaveDraft / DetectDraft / Promote don't share
// state across unrelated sessions in the same repo. BranchSlug("") returns the
// deterministic "branch__da39a3ee", which is what we expect to land in Slug.
func TestNewPathsWithRoot_EmptyBranchUsesDetachedSlug(t *testing.T) {
	t.Parallel()

	p := NewPathsWithRoot("/out", "/repo", "")
	const detachedSlug = "branch__da39a3ee"
	if p.Slug != detachedSlug {
		t.Errorf("Slug = %q, want %q (BranchSlug(\"\"))", p.Slug, detachedSlug)
	}
	wantDrafts := filepath.Join("/out", p.ProjectSlug, "drafts", detachedSlug)
	if got := p.DraftsDir(); got != wantDrafts {
		t.Errorf("DraftsDir() = %q, want %q", got, wantDrafts)
	}
	wantReviews := filepath.Join("/out", p.ProjectSlug, "reviews", detachedSlug)
	if got := p.ReviewsDir(); got != wantReviews {
		t.Errorf("ReviewsDir() = %q, want %q", got, wantReviews)
	}
}

func TestProjectSlug_Deterministic(t *testing.T) {
	t.Parallel()
	a := ProjectSlug("/Users/me/code/sitatame")
	b := ProjectSlug("/Users/me/code/sitatame")
	if a != b {
		t.Errorf("ProjectSlug should be deterministic: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "sitatame__") {
		t.Errorf("expected sitatame__ prefix: %q", a)
	}
}

func TestProjectSlug_DistinguishesCheckouts(t *testing.T) {
	t.Parallel()
	// Two checkouts with the same basename but different absolute paths
	// (e.g. a worktree and the primary checkout) must not collide.
	a := ProjectSlug("/Users/me/code/sitatame")
	b := ProjectSlug("/Users/me/work/sitatame")
	if a == b {
		t.Errorf("expected different slugs for distinct paths, got %q == %q", a, b)
	}
}

func TestProjectSlug_UnsafeBasenameFallsBack(t *testing.T) {
	t.Parallel()
	// Unsafe basename (all '/' would be impossible since filepath.Base
	// strips them, so test with non-ASCII).
	got := ProjectSlug("/Users/me/日本語")
	if !strings.HasPrefix(got, "project__") && !strings.HasPrefix(got, "____") {
		// Either fallback to "project" or all-underscores-from-safePrefix
		// are acceptable; just make sure we get a slug with the hash.
		if !strings.Contains(got, "__") {
			t.Errorf("expected slug to contain '__': %q", got)
		}
	}
}

func TestNewPaths_HonoursSITATAMEHOME(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvOutputRoot, dir)
	p := NewPaths("/repo", "feature")
	if p.OutputRoot != dir {
		t.Errorf("OutputRoot = %q, want %q", p.OutputRoot, dir)
	}
	if !strings.HasPrefix(p.Root(), dir+string(filepath.Separator)) {
		t.Errorf("Root() %q should live under %q", p.Root(), dir)
	}
}
