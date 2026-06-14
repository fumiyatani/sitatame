package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newMigrationStore builds a Store under an isolated outputRoot, using the
// supplied branch (empty means branch-independent). The fixture clock is fixed
// to ts so .legacy-<YYYYMMDDTHHMMSS> names are deterministic.
func newMigrationStore(t *testing.T, ts time.Time) *Store {
	t.Helper()
	outputRoot := t.TempDir()
	repoRoot := t.TempDir()
	s := NewStore(NewPathsWithRoot(outputRoot, repoRoot, "feature/auth"))
	s.Now = func() time.Time { return ts }
	return s
}

// writeLegacyReview creates <reviewsRoot>/<branchSlug>/<filename> with the
// given content and mtime so latestReviewFile can distinguish multiple files.
func writeLegacyReview(t *testing.T, reviewsRoot, branchSlug, filename, content string, mtime time.Time) string {
	t.Helper()
	dir := filepath.Join(reviewsRoot, branchSlug)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, filename)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestMigrateLegacyLayout_NoCandidates verifies that when neither drafts/ nor
// reviews/ exists under the project root, MigrateLegacyLayout is a no-op.
func TestMigrateLegacyLayout_NoCandidates(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 14, 10, 57, 20, 0, time.UTC)
	s := newMigrationStore(t, ts)

	migrated, legacyDir, err := s.MigrateLegacyLayout()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0 (no legacy layout)", migrated)
	}
	if legacyDir != "" {
		t.Errorf("legacyDir = %q, want empty", legacyDir)
	}
}

// TestMigrateLegacyLayout_BothDraftsAndReviews verifies the full migration path:
// drafts/ + reviews/ are moved to .legacy-<ts>/, and the latest review per
// branch-slug is copied into the new layout.
func TestMigrateLegacyLayout_BothDraftsAndReviews(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 14, 10, 57, 20, 0, time.UTC)
	s := newMigrationStore(t, ts)

	projectRoot := s.Paths.Root()
	draftsRoot := s.Paths.LegacyDraftsRoot()
	reviewsRoot := s.Paths.LegacyReviewsRoot()

	// Seed drafts.
	draftDir := filepath.Join(draftsRoot, "feature--auth")
	if err := os.MkdirAll(draftDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftDir, "draft.md"), []byte("draft"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Seed reviews: two files, the second is newer.
	slug := "feature--auth"
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	writeLegacyReview(t, reviewsRoot, slug, "20260101T000000-review.md", "older content", t1)
	writeLegacyReview(t, reviewsRoot, slug, "20260301T000000-review.md", "newer content", t2)

	migrated, legacyDir, err := s.MigrateLegacyLayout()
	if err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}
	if migrated != 1 {
		t.Errorf("migrated = %d, want 1 (one branch-slug)", migrated)
	}
	wantLegacyDir := filepath.Join(projectRoot, ".legacy-20260614T105720")
	if legacyDir != wantLegacyDir {
		t.Errorf("legacyDir = %q, want %q", legacyDir, wantLegacyDir)
	}

	// Legacy drafts and reviews must be preserved.
	if _, err := os.Stat(filepath.Join(legacyDir, "drafts", slug, "draft.md")); err != nil {
		t.Errorf("legacy draft should be preserved under .legacy-: %v", err)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "reviews", slug, "20260101T000000-review.md")); err != nil {
		t.Errorf("legacy older review should be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "reviews", slug, "20260301T000000-review.md")); err != nil {
		t.Errorf("legacy newer review should be preserved: %v", err)
	}

	// Original drafts/ and reviews/ must no longer exist at old paths.
	if _, err := os.Stat(draftsRoot); !os.IsNotExist(err) {
		t.Errorf("old drafts/ should be gone after migration")
	}
	if _, err := os.Stat(reviewsRoot); !os.IsNotExist(err) {
		t.Errorf("old reviews/ should be gone after migration")
	}

	// New review.md must contain the newest content.
	newReviewFile := filepath.Join(projectRoot, slug, "review.md")
	got, err := os.ReadFile(newReviewFile)
	if err != nil {
		t.Fatalf("new review.md not found at %s: %v", newReviewFile, err)
	}
	if string(got) != "newer content" {
		t.Errorf("new review.md = %q, want %q", got, "newer content")
	}
}

// TestMigrateLegacyLayout_ReviewsOnly verifies that when only reviews/ exists
// (no drafts/), migration still works.
func TestMigrateLegacyLayout_ReviewsOnly(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 14, 10, 57, 20, 0, time.UTC)
	s := newMigrationStore(t, ts)

	projectRoot := s.Paths.Root()
	reviewsRoot := s.Paths.LegacyReviewsRoot()

	slug := "main"
	t1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	writeLegacyReview(t, reviewsRoot, slug, "20260201T000000-review.md", "main review", t1)

	migrated, legacyDir, err := s.MigrateLegacyLayout()
	if err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}
	if migrated != 1 {
		t.Errorf("migrated = %d, want 1", migrated)
	}
	if legacyDir == "" {
		t.Error("legacyDir should not be empty")
	}

	// drafts/ was never present; verify no drafts dir was created in legacy.
	legacyDrafts := filepath.Join(legacyDir, "drafts")
	if _, err := os.Stat(legacyDrafts); !os.IsNotExist(err) {
		t.Errorf("legacy drafts/ should not exist when none was present")
	}

	// New review.md must exist.
	newReviewFile := filepath.Join(projectRoot, slug, "review.md")
	got, err := os.ReadFile(newReviewFile)
	if err != nil {
		t.Fatalf("new review.md missing: %v", err)
	}
	if string(got) != "main review" {
		t.Errorf("new review.md = %q, want %q", got, "main review")
	}
}

// TestMigrateLegacyLayout_DraftsOnly verifies that when only drafts/ exists
// (no reviews/), the directory is moved to legacy but no new review.md is
// created (there is nothing to copy from).
func TestMigrateLegacyLayout_DraftsOnly(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 14, 10, 57, 20, 0, time.UTC)
	s := newMigrationStore(t, ts)

	projectRoot := s.Paths.Root()
	draftsRoot := s.Paths.LegacyDraftsRoot()

	draftDir := filepath.Join(draftsRoot, "feature--x")
	if err := os.MkdirAll(draftDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftDir, "draft.md"), []byte("wip"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrated, legacyDir, err := s.MigrateLegacyLayout()
	if err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}
	// No reviews to copy -> migrated == 0.
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0 (no reviews to copy)", migrated)
	}
	if legacyDir == "" {
		t.Error("legacyDir should not be empty even when no reviews to copy")
	}

	// Draft preserved in legacy.
	legacyDraft := filepath.Join(legacyDir, "drafts", "feature--x", "draft.md")
	if _, err := os.Stat(legacyDraft); err != nil {
		t.Errorf("legacy draft not found: %v", err)
	}

	// No new review.md should be created under the project root for "feature--x".
	newReviewFile := filepath.Join(projectRoot, "feature--x", "review.md")
	if _, err := os.Stat(newReviewFile); !os.IsNotExist(err) {
		t.Errorf("new review.md should NOT be created when there are no legacy reviews")
	}
}

// TestMigrateLegacyLayout_LatestSelected verifies that when multiple review
// files exist for a single branch-slug, only the most recently modified one
// is copied to the new layout.
func TestMigrateLegacyLayout_LatestSelected(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 14, 10, 57, 20, 0, time.UTC)
	s := newMigrationStore(t, ts)

	projectRoot := s.Paths.Root()
	reviewsRoot := s.Paths.LegacyReviewsRoot()

	slug := "bugfix--auth"
	// Write three files with distinct mtimes.
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	writeLegacyReview(t, reviewsRoot, slug, "a.md", "january", t1)
	writeLegacyReview(t, reviewsRoot, slug, "b.md", "february", t2)
	writeLegacyReview(t, reviewsRoot, slug, "c.md", "may (latest)", t3)

	migrated, _, err := s.MigrateLegacyLayout()
	if err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}
	if migrated != 1 {
		t.Errorf("migrated = %d, want 1", migrated)
	}

	got, err := os.ReadFile(filepath.Join(projectRoot, slug, "review.md"))
	if err != nil {
		t.Fatalf("new review.md missing: %v", err)
	}
	if string(got) != "may (latest)" {
		t.Errorf("new review.md = %q, want %q", got, "may (latest)")
	}
}

// TestMigrateLegacyLayout_ExistingNewReviewSkipped verifies that when a new
// review.md already exists at the new layout path, migration does not
// overwrite it.
func TestMigrateLegacyLayout_ExistingNewReviewSkipped(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 14, 10, 57, 20, 0, time.UTC)
	s := newMigrationStore(t, ts)

	projectRoot := s.Paths.Root()
	reviewsRoot := s.Paths.LegacyReviewsRoot()

	slug := "feature--auth"
	t1 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	writeLegacyReview(t, reviewsRoot, slug, "20260401T000000-review.md", "legacy content", t1)

	// Pre-create the new review.md so migration should skip overwriting it.
	newBranchDir := filepath.Join(projectRoot, slug)
	if err := os.MkdirAll(newBranchDir, 0o700); err != nil {
		t.Fatal(err)
	}
	newReviewFile := filepath.Join(newBranchDir, "review.md")
	existingContent := "existing new content"
	if err := os.WriteFile(newReviewFile, []byte(existingContent), 0o600); err != nil {
		t.Fatal(err)
	}

	migrated, _, err := s.MigrateLegacyLayout()
	if err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}
	// Skip means 0 migrated for this slug.
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0 (existing new review must not be overwritten)", migrated)
	}

	// Verify the new review.md was not overwritten.
	got, err := os.ReadFile(newReviewFile)
	if err != nil {
		t.Fatalf("new review.md should still exist: %v", err)
	}
	if string(got) != existingContent {
		t.Errorf("new review.md = %q, want %q (must not be overwritten)", got, existingContent)
	}
}

// TestMigrateLegacyLayout_EmptyBranchSlugSkipped verifies that a branch-slug
// directory that is empty (no .md files) does not count as migrated.
func TestMigrateLegacyLayout_EmptyBranchSlugSkipped(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 14, 10, 57, 20, 0, time.UTC)
	s := newMigrationStore(t, ts)

	projectRoot := s.Paths.Root()
	reviewsRoot := s.Paths.LegacyReviewsRoot()

	// Create empty branch-slug dir.
	emptySlug := "empty--branch"
	if err := os.MkdirAll(filepath.Join(reviewsRoot, emptySlug), 0o700); err != nil {
		t.Fatal(err)
	}

	migrated, legacyDir, err := s.MigrateLegacyLayout()
	if err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0 (empty dir should be skipped)", migrated)
	}
	if legacyDir == "" {
		t.Error("legacyDir should not be empty (mv still happened)")
	}

	// No new review.md created.
	newReviewFile := filepath.Join(projectRoot, emptySlug, "review.md")
	if _, err := os.Stat(newReviewFile); !os.IsNotExist(err) {
		t.Errorf("review.md should not be created for empty branch-slug dir")
	}

	// Legacy reviews dir preserved.
	legacyReviews := filepath.Join(legacyDir, "reviews", emptySlug)
	if _, err := os.Stat(legacyReviews); err != nil {
		t.Errorf("legacy empty dir should be in .legacy-: %v", err)
	}
}

// TestMigrateLegacyLayout_MultipleBranches verifies that multiple branch-slugs
// are all migrated and each gets its own review.md in the new layout.
func TestMigrateLegacyLayout_MultipleBranches(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 14, 10, 57, 20, 0, time.UTC)
	s := newMigrationStore(t, ts)

	projectRoot := s.Paths.Root()
	reviewsRoot := s.Paths.LegacyReviewsRoot()

	slugs := []string{"alpha", "beta", "gamma"}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, slug := range slugs {
		writeLegacyReview(t, reviewsRoot, slug, "r.md",
			"content for "+slug, base.Add(time.Duration(i)*24*time.Hour))
	}

	migrated, _, err := s.MigrateLegacyLayout()
	if err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}
	if migrated != len(slugs) {
		t.Errorf("migrated = %d, want %d", migrated, len(slugs))
	}

	for _, slug := range slugs {
		got, err := os.ReadFile(filepath.Join(projectRoot, slug, "review.md"))
		if err != nil {
			t.Errorf("review.md missing for slug %q: %v", slug, err)
			continue
		}
		if !strings.Contains(string(got), slug) {
			t.Errorf("review.md for slug %q = %q, want to contain slug name", slug, got)
		}
	}
}
