package search

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWalk_FindsHits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := map[string]string{
		"a/foo.md":     "hello world\nbody refers to bug #42\n",
		"a/bar.md":     "no match here\nstill nothing\n",
		"b/baz.md":     "another bug here\n",
		"ignore.txt":   "bug should be skipped (not .md)\n",
		"a/empty.md":   "",
	}
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	re := regexp.MustCompile(`bug`)
	hits, err := Walk(dir, re)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2; got %+v", len(hits), hits)
	}
	// Confirm both hits cite a .md file (not the .txt) and have non-zero line numbers.
	for _, h := range hits {
		if !strings.HasSuffix(h.Path, ".md") {
			t.Errorf("hit on non-md file: %s", h.Path)
		}
		if h.Line < 1 {
			t.Errorf("hit has zero line: %+v", h)
		}
		if !strings.Contains(h.Text, "bug") {
			t.Errorf("hit text missing 'bug': %q", h.Text)
		}
	}
}

func TestWalk_RegexpMatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.md"),
		[]byte("TODO check rate limit\nDONE check it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?i)^todo`)
	hits, _ := Walk(dir, re)
	if len(hits) != 1 || hits[0].Line != 1 {
		t.Fatalf("expected one hit on line 1, got %+v", hits)
	}
}

func TestWalk_EmptyRegexp(t *testing.T) {
	t.Parallel()
	hits, err := Walk(t.TempDir(), nil)
	if err != nil || hits != nil {
		t.Errorf("nil regexp must short-circuit: hits=%v err=%v", hits, err)
	}
}

func TestWalk_MissingDir(t *testing.T) {
	t.Parallel()
	hits, err := Walk(filepath.Join(t.TempDir(), "does-not-exist"), regexp.MustCompile(`x`))
	if err != nil || len(hits) != 0 {
		t.Errorf("missing dir should return zero hits and nil err; got hits=%v err=%v", hits, err)
	}
}

// TestWalk_ExcludesLegacyDirs verifies that directories with a .legacy- prefix
// are skipped entirely so migrated data does not surface in search results.
func TestWalk_ExcludesLegacyDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// File in a normal directory: should match.
	normalDir := filepath.Join(dir, "feature--auth")
	if err := os.MkdirAll(normalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(normalDir, "review.md"),
		[]byte("TODO: check legacy exclusion\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// File in a .legacy-<ts>/ directory: must NOT match.
	legacyDir := filepath.Join(dir, ".legacy-20260614T105720", "reviews", "feature--auth")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "20260101T000000-review.md"),
		[]byte("TODO: this is in legacy and must not appear\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	re := regexp.MustCompile(`TODO`)
	hits, err := Walk(dir, re)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1 (only the non-legacy file); got %+v", len(hits), hits)
	}
	if strings.Contains(hits[0].Path, ".legacy-") {
		t.Errorf("hit path contains .legacy- directory: %s", hits[0].Path)
	}
}
