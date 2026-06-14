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

// TestStore_SaveReview_Normal verifies that SaveReview writes review.md and
// returns its path.
func TestStore_SaveReview_Normal(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)
	r := &Review{
		Schema:        1,
		Branch:        "feature/auth",
		Base:          Ref{Ref: "origin/main", SHA: "aaa"},
		Head:          Ref{Ref: "HEAD", SHA: "bbb"},
		ReviewComment: "fix-auth",
	}
	path, err := s.SaveReview(r)
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("SaveReview returned empty path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("review file missing: %v", err)
	}
	// Must be under BranchDir as review.md.
	if got := filepath.Base(path); got != "review.md" {
		t.Errorf("filename = %q, want review.md", got)
	}
	if !strings.HasPrefix(path, s.Paths.BranchDir()) {
		t.Errorf("path %q not under BranchDir %q", path, s.Paths.BranchDir())
	}
}

// TestStore_SaveReview_EmptyIsNoop verifies that an empty Review (no comments,
// blank review_comment) results in no file creation and ("", nil) return.
func TestStore_SaveReview_EmptyIsNoop(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)
	r := &Review{
		Schema: 1,
		Branch: "feature/auth",
		// Empty: no comments, no review_comment.
	}
	path, err := s.SaveReview(r)
	if err != nil {
		t.Fatalf("unexpected error on empty review: %v", err)
	}
	if path != "" {
		t.Errorf("path = %q, want empty (no file should be created)", path)
	}
	if _, err := os.Stat(s.Paths.ReviewFile()); !os.IsNotExist(err) {
		t.Errorf("review.md should not exist after empty save, but stat returned: %v", err)
	}
}

// TestStore_SaveReview_CreatesBackup verifies that when review.md already
// exists, a second SaveReview creates review.md.bak with the previous content.
func TestStore_SaveReview_CreatesBackup(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)

	r := &Review{
		Schema:        1,
		Branch:        "feature/auth",
		ReviewComment: "first",
	}
	firstPath, err := s.SaveReview(r)
	if err != nil {
		t.Fatal(err)
	}
	firstContent, _ := os.ReadFile(firstPath)

	r.ReviewComment = "second"
	if _, err := s.SaveReview(r); err != nil {
		t.Fatal(err)
	}

	// .bak must exist and contain the first content.
	bakContent, err := os.ReadFile(s.Paths.BakFile())
	if err != nil {
		t.Fatalf("review.md.bak missing: %v", err)
	}
	if string(bakContent) != string(firstContent) {
		t.Errorf("bak content differs from first write")
	}

	// review.md must have the second content.
	newContent, _ := os.ReadFile(s.Paths.ReviewFile())
	if string(newContent) == string(firstContent) {
		t.Errorf("review.md still has first content after second save")
	}
}

// TestStore_SaveReview_BakIsOverwritten verifies that a third SaveReview
// overwrites the existing .bak (only 1 generation kept).
func TestStore_SaveReview_BakIsOverwritten(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)

	for i, comment := range []string{"first", "second", "third"} {
		r := &Review{Schema: 1, Branch: "feature/auth", ReviewComment: comment}
		if _, err := s.SaveReview(r); err != nil {
			t.Fatalf("save #%d: %v", i+1, err)
		}
	}
	// .bak must exist and contain the second write (not the first).
	bakContent, err := os.ReadFile(s.Paths.BakFile())
	if err != nil {
		t.Fatalf("review.md.bak missing: %v", err)
	}
	if !strings.Contains(string(bakContent), "second") {
		t.Errorf("bak should contain second content; got:\n%s", bakContent)
	}
	if strings.Contains(string(bakContent), "first") {
		t.Errorf("bak should NOT contain first content (overwritten); got:\n%s", bakContent)
	}
}

// TestStore_SaveReview_RescueOnEncodeFailure verifies that when Encode fails,
// SaveReview writes a rescue JSON file under BranchDir and returns a
// RescueError. The review.md file must NOT be created.
func TestStore_SaveReview_RescueOnEncodeFailure(t *testing.T) {
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

	reviewPath, err := s.SaveReview(r)

	if reviewPath != "" {
		t.Errorf("reviewPath = %q, want empty (review.md must not be created on encode failure)", reviewPath)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var re *RescueError
	if !errors.As(err, &re) {
		t.Fatalf("error is %T (%v), want *RescueError", err, err)
	}
	if re.RescuePath == "" {
		t.Fatal("RescueError.RescuePath is empty")
	}
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

	// The rescue file must be under BranchDir.
	if !strings.HasPrefix(re.RescuePath, s.Paths.BranchDir()) {
		t.Errorf("rescue path %q is not under BranchDir %q", re.RescuePath, s.Paths.BranchDir())
	}

	// The rescue filename must match the expected pattern.
	base := filepath.Base(re.RescuePath)
	if !strings.HasPrefix(base, "review.md.rescue.") || !strings.HasSuffix(base, ".json") {
		t.Errorf("rescue filename %q does not match pattern review.md.rescue.<timestamp>.json", base)
	}

	// review.md must NOT exist.
	if _, err := os.Stat(s.Paths.ReviewFile()); !os.IsNotExist(err) {
		t.Errorf("review.md should not exist after encode failure")
	}
}

// TestStore_DetectReview verifies DetectReview returns the path when review.md
// exists and "" when it does not.
func TestStore_DetectReview(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)

	// No file yet.
	if got, _ := s.DetectReview(); got != "" {
		t.Errorf("DetectReview (no file) = %q, want empty", got)
	}

	r := &Review{
		Schema:        1,
		Branch:        "feature/auth",
		ReviewComment: "fix-auth",
	}
	path, err := s.SaveReview(r)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.DetectReview()
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Errorf("DetectReview = %q, want %q", got, path)
	}
}

// TestStore_RecoverFromCrash_RestoresBak verifies that when review.md is absent
// but review.md.bak exists, RecoverFromCrash restores the backup.
func TestStore_RecoverFromCrash_RestoresBak(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)

	// Simulate a crash: create .bak without review.md.
	branchDir := s.Paths.BranchDir()
	if err := os.MkdirAll(branchDir, 0o700); err != nil {
		t.Fatal(err)
	}
	bakContent := []byte("bak content")
	if err := os.WriteFile(s.Paths.BakFile(), bakContent, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.RecoverFromCrash(); err != nil {
		t.Fatalf("RecoverFromCrash: %v", err)
	}

	// review.md must now exist with bak content.
	got, err := os.ReadFile(s.Paths.ReviewFile())
	if err != nil {
		t.Fatalf("review.md missing after recovery: %v", err)
	}
	if string(got) != string(bakContent) {
		t.Errorf("recovered content = %q, want %q", got, bakContent)
	}
}

// TestStore_RecoverFromCrash_NoopWhenBothExist verifies that RecoverFromCrash
// is a no-op when review.md already exists (normal state).
func TestStore_RecoverFromCrash_NoopWhenBothExist(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)

	// Normal state: both review.md and .bak exist.
	branchDir := s.Paths.BranchDir()
	if err := os.MkdirAll(branchDir, 0o700); err != nil {
		t.Fatal(err)
	}
	finalContent := []byte("final content")
	bakContent := []byte("bak content")
	if err := os.WriteFile(s.Paths.ReviewFile(), finalContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Paths.BakFile(), bakContent, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.RecoverFromCrash(); err != nil {
		t.Fatalf("RecoverFromCrash: %v", err)
	}

	// review.md must be unchanged.
	got, _ := os.ReadFile(s.Paths.ReviewFile())
	if string(got) != string(finalContent) {
		t.Errorf("review.md content changed unexpectedly")
	}
}

// TestStore_RecoverFromCrash_CleansTmpFiles verifies that orphaned .tmp files
// are cleaned up by RecoverFromCrash.
func TestStore_RecoverFromCrash_CleansTmpFiles(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 5, 1, 15, 23, 0, 0, time.UTC)
	s := newTestStore(t, ts)

	branchDir := s.Paths.BranchDir()
	if err := os.MkdirAll(branchDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Create an orphaned tmp file.
	tmpPath := filepath.Join(branchDir, ".review.12345.tmp")
	if err := os.WriteFile(tmpPath, []byte("orphaned"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.RecoverFromCrash(); err != nil {
		t.Fatalf("RecoverFromCrash: %v", err)
	}

	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("orphaned tmp file should be removed, but still exists")
	}
}
