package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T, fixedTime time.Time) *Store {
	t.Helper()
	// Each test gets its own output root + repo root. Using NewPathsWithRoot
	// keeps these tests independent of $HOME / $SITATAME_HOME.
	outputRoot := t.TempDir()
	repoRoot := t.TempDir()
	s := NewStore(NewPathsWithRoot(outputRoot, repoRoot, "feature/auth"))
	s.Now = func() time.Time { return fixedTime }
	return s
}

func TestSlugifyReviewComment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", "review"},
		{"   \n\n", "review"},
		{"fix-auth", "fix-auth"},
		{"fix auth bug", "fix_auth_bug"},
		{"日本語のみ", "review"}, // all unsafe -> all underscores -> trimmed -> empty -> "review"
		{"first line\nsecond line", "first_line"},
		{"this-is-a-very-long-review-comment-that-exceeds-thirty-two-chars", "this-is-a-very-long-review-comme"},
		{"..", "review"},
		{"...path-trav", "path-trav"},
	}
	for _, c := range cases {
		if got := slugifyReviewComment(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStore_GenerateID_NoCollision(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)
	id, err := s.GenerateID("fix-auth")
	if err != nil {
		t.Fatal(err)
	}
	want := "20260501T152300-fix-auth"
	if id != want {
		t.Errorf("id = %q, want %q", id, want)
	}
}

func TestStore_GenerateID_Collision(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)
	// Pre-create the base id in drafts/.
	if err := os.MkdirAll(s.Paths.DraftsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	base := s.Paths.DraftFile("20260501T152300-fix")
	if err := os.WriteFile(base, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := s.GenerateID("fix")
	if err != nil {
		t.Fatal(err)
	}
	if id != "20260501T152300-fix-1" {
		t.Errorf("id = %q, want suffix -1", id)
	}

	// Pre-create -1 too -> expect -2.
	if err := os.WriteFile(s.Paths.DraftFile("20260501T152300-fix-1"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err = s.GenerateID("fix")
	if err != nil {
		t.Fatal(err)
	}
	if id != "20260501T152300-fix-2" {
		t.Errorf("id = %q, want suffix -2", id)
	}
}

func TestStore_SaveDraftAndPromote(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)
	r := &Review{
		Schema:        1,
		Branch:        "feature/auth",
		Base:          Ref{Ref: "origin/main", SHA: "aaa"},
		Head:          Ref{Ref: "HEAD", SHA: "bbb"},
		CreatedAt:     ts,
		ReviewComment: "fix-auth",
	}
	draft, err := s.SaveDraft(r)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID == "" {
		t.Fatal("ID not assigned")
	}
	if !strings.HasSuffix(draft, filepath.Join("drafts", s.Paths.Slug, r.ID+".md")) {
		t.Errorf("draft path: %q", draft)
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatalf("draft missing: %v", err)
	}

	final, err := s.Promote(draft)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(final, filepath.Join("reviews", s.Paths.Slug, r.ID+".md")) {
		t.Errorf("review path: %q", final)
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatalf("review missing: %v", err)
	}
	if _, err := os.Stat(draft); !os.IsNotExist(err) {
		t.Errorf("draft should be moved, but still exists: %v", err)
	}
}

func TestStore_DetectDraft(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)

	if got, _ := s.DetectDraft(); got != "" {
		t.Errorf("expected no draft, got %q", got)
	}

	r := &Review{
		Schema:    1,
		Branch:    "feature/auth",
		CreatedAt: ts,
		Base:      Ref{Ref: "origin/main", SHA: "aaa"},
		Head:      Ref{Ref: "HEAD", SHA: "bbb"},
	}
	draft, err := s.SaveDraft(r)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.DetectDraft()
	if err != nil {
		t.Fatal(err)
	}
	if got != draft {
		t.Errorf("DetectDraft = %q, want %q", got, draft)
	}
}

func TestStore_LatestReview_PicksNewest(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)
	if err := os.MkdirAll(s.Paths.ReviewsDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	older := s.Paths.ReviewFile("20260101T000000-a")
	newer := s.Paths.ReviewFile("20260201T000000-b")
	if err := os.WriteFile(older, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Force older mtime.
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}

	got, err := s.LatestReview()
	if err != nil {
		t.Fatal(err)
	}
	if got != newer {
		t.Errorf("LatestReview = %q, want %q", got, newer)
	}
}
