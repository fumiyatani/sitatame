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

	p := NewPaths("/repo", "feature/x")
	if p.Slug == "" || !strings.Contains(p.Slug, "__") {
		t.Errorf("unexpected slug: %q", p.Slug)
	}
	want := filepath.Join("/repo", ".sitatame", "reviews", p.Slug)
	if got := p.ReviewsDir(); got != want {
		t.Errorf("ReviewsDir() = %q, want %q", got, want)
	}
	want = filepath.Join("/repo", ".sitatame", "drafts", p.Slug)
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
