package review

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPaths_BranchDir(t *testing.T) {
	t.Parallel()
	p := NewPathsWithRoot("/home/me/.sitatame", "/repo", "feature/auth")
	slug := BranchSlug("feature/auth")
	want := filepath.Join("/home/me/.sitatame", ProjectSlug("/repo"), slug)
	if got := p.BranchDir(); got != want {
		t.Errorf("BranchDir() = %q, want %q", got, want)
	}
}

func TestPaths_ReviewFile(t *testing.T) {
	t.Parallel()
	p := NewPathsWithRoot("/home/me/.sitatame", "/repo", "feature/auth")
	got := p.ReviewFile()
	if !strings.HasSuffix(got, "review.md") {
		t.Errorf("ReviewFile() = %q, want suffix review.md", got)
	}
	if !strings.Contains(got, p.BranchDir()) {
		t.Errorf("ReviewFile() = %q, want under BranchDir %q", got, p.BranchDir())
	}
}

func TestPaths_BakFile(t *testing.T) {
	t.Parallel()
	p := NewPathsWithRoot("/home/me/.sitatame", "/repo", "feature/auth")
	got := p.BakFile()
	if !strings.HasSuffix(got, "review.md.bak") {
		t.Errorf("BakFile() = %q, want suffix review.md.bak", got)
	}
	if !strings.Contains(got, p.BranchDir()) {
		t.Errorf("BakFile() = %q, want under BranchDir %q", got, p.BranchDir())
	}
}

func TestPaths_RescueFilePattern(t *testing.T) {
	t.Parallel()
	p := NewPathsWithRoot("/home/me/.sitatame", "/repo", "feature/auth")
	got := p.RescueFilePattern()
	if !strings.HasSuffix(got, "review.md.rescue.*.json") {
		t.Errorf("RescueFilePattern() = %q, want suffix review.md.rescue.*.json", got)
	}
	if !strings.Contains(got, p.BranchDir()) {
		t.Errorf("RescueFilePattern() = %q, want under BranchDir %q", got, p.BranchDir())
	}
}

func TestPaths_LegacyHelpers(t *testing.T) {
	t.Parallel()
	p := NewPathsWithRoot("/home/me/.sitatame", "/repo", "feature/auth")
	if got := p.LegacyReviewsRoot(); !strings.HasSuffix(got, "reviews") {
		t.Errorf("LegacyReviewsRoot() = %q, want suffix reviews", got)
	}
	if got := p.LegacyDraftsRoot(); !strings.HasSuffix(got, "drafts") {
		t.Errorf("LegacyDraftsRoot() = %q, want suffix drafts", got)
	}
}
