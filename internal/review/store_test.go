package review

import (
	"encoding/json"
	"errors"
	"fmt"
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

func TestSaveDraft_SetsCreatedAtIfZero(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)
	r := &Review{
		Schema:        1,
		Branch:        "feature/auth",
		Base:          Ref{Ref: "origin/main", SHA: "aaa"},
		Head:          Ref{Ref: "HEAD", SHA: "bbb"},
		ReviewComment: "fix-auth",
		// CreatedAt deliberately left zero.
	}
	if _, err := s.SaveDraft(r); err != nil {
		t.Fatal(err)
	}
	if !r.CreatedAt.Equal(ts) {
		t.Errorf("in-memory CreatedAt = %v, want %v", r.CreatedAt, ts)
	}
	// Verify the persisted YAML carries the same timestamp so external tools
	// can sort/filter without seeing Go zero time.
	body, err := os.ReadFile(s.Paths.DraftFile(r.ID))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(body)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("persisted created_at = %v, want %v", got.CreatedAt, ts)
	}
}

func TestSaveDraft_PreservesExistingCreatedAt(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	s := newTestStore(t, now)
	original := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := &Review{
		Schema:        1,
		Branch:        "feature/auth",
		Base:          Ref{Ref: "origin/main", SHA: "aaa"},
		Head:          Ref{Ref: "HEAD", SHA: "bbb"},
		ReviewComment: "fix-auth",
		CreatedAt:     original,
	}
	if _, err := s.SaveDraft(r); err != nil {
		t.Fatal(err)
	}
	if !r.CreatedAt.Equal(original) {
		t.Errorf("CreatedAt overwritten: got %v, want %v", r.CreatedAt, original)
	}
	body, err := os.ReadFile(s.Paths.DraftFile(r.ID))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(body)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(original) {
		t.Errorf("persisted created_at = %v, want %v", got.CreatedAt, original)
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

// TestSaveDraft_RescueOnEncodeFailure verifies that when Encode fails,
// SaveDraft writes a rescue JSON file under DraftsDir and returns a
// RescueError containing the rescue path. The draft file itself must NOT
// be created; the rescue file must contain the full Review as JSON.
func TestSaveDraft_RescueOnEncodeFailure(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 14, 10, 57, 20, 0, time.UTC)
	s := newTestStore(t, ts)

	// Inject a broken encode function to simulate the #76 failure mode.
	encodeErr := fmt.Errorf("yaml re-decode: did not find expected key at line 49")
	s.encodeFunc = func(r Review) ([]byte, error) {
		return nil, encodeErr
	}

	r := &Review{
		Schema:        1,
		Branch:        "feature/auth",
		Base:          Ref{Ref: "origin/main", SHA: "aaa"},
		Head:          Ref{Ref: "HEAD", SHA: "bbb"},
		ReviewComment: "fix-auth",
		Comments: []Comment{
			{
				Anchor: Anchor{
					AnchorID: "11111111-1111-1111-1111-111111111111",
					Kind:     KindLine,
					Path:     "src/main.go",
					Side:     SideHead,
					Blob:     "bbb",
					Line:     10,
				},
				State: StateOpen,
				Body:  "「重要」このコメントを確認してください。\n: colon at line start\n",
			},
		},
	}

	draftPath, err := s.SaveDraft(r)

	// Must return an error — no draft path.
	if draftPath != "" {
		t.Errorf("draftPath = %q, want empty (draft must not be created on encode failure)", draftPath)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// The error must be a RescueError.
	var re *RescueError
	if !errors.As(err, &re) {
		t.Fatalf("error is %T (%v), want *RescueError", err, err)
	}
	if re.RescuePath == "" {
		t.Fatal("RescueError.RescuePath is empty")
	}

	// Unwrapped error must chain to the original encode error.
	if !errors.Is(re, encodeErr) {
		t.Errorf("RescueError does not wrap original encode error: %v", re)
	}

	// The rescue file must exist and contain valid JSON with the expected schema.
	raw, err := os.ReadFile(re.RescuePath)
	if err != nil {
		t.Fatalf("rescue file not found at %s: %v", re.RescuePath, err)
	}
	var payload rescuePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("rescue file is not valid JSON: %v\ncontent: %s", err, raw)
	}
	if payload.Schema != "rescue/1" {
		t.Errorf("schema = %q, want rescue/1", payload.Schema)
	}
	if payload.OriginalEncodeError == "" {
		t.Error("original_encode_error is empty")
	}
	if payload.Review == nil {
		t.Fatal("review field is nil in rescue payload")
	}
	if len(payload.Review.Comments) != 1 {
		t.Errorf("rescue review.comments count = %d, want 1", len(payload.Review.Comments))
	}

	// The rescue file must be under DraftsDir.
	if !strings.HasPrefix(re.RescuePath, s.Paths.DraftsDir()) {
		t.Errorf("rescue path %q is not under DraftsDir %q", re.RescuePath, s.Paths.DraftsDir())
	}

	// The rescue filename must match the expected pattern.
	base := filepath.Base(re.RescuePath)
	if !strings.HasPrefix(base, "review.md.rescue.") || !strings.HasSuffix(base, ".json") {
		t.Errorf("rescue filename %q does not match pattern review.md.rescue.<timestamp>.json", base)
	}

	// The draft file must NOT exist: rescue is the only output.
	draftGlob := filepath.Join(s.Paths.DraftsDir(), "*.md")
	matches, _ := filepath.Glob(draftGlob)
	if len(matches) > 0 {
		t.Errorf("draft files found when none expected: %v", matches)
	}
}
