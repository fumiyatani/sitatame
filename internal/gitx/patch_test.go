package gitx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/gitx/internal/parser"
)

func TestParseHunkHeader_WithCounts(t *testing.T) {
	t.Parallel()
	h, err := parser.ParseHunkHeader("@@ -10,5 +12,7 @@ func foo()")
	if err != nil {
		t.Fatal(err)
	}
	if h.BaseStart != 10 || h.BaseLines != 5 || h.HeadStart != 12 || h.HeadLines != 7 {
		t.Errorf("ranges: %+v", h)
	}
	if h.Header != "func foo()" {
		t.Errorf("header trailer = %q", h.Header)
	}
}

func TestParseHunkHeader_OmittedCount(t *testing.T) {
	t.Parallel()
	h, err := parser.ParseHunkHeader("@@ -10 +10 @@")
	if err != nil {
		t.Fatal(err)
	}
	if h.BaseLines != 1 || h.HeadLines != 1 {
		t.Errorf("default counts wrong: %+v", h)
	}
}

func TestParseHunkHeader_Bad(t *testing.T) {
	t.Parallel()
	cases := []string{
		"@@ -10,5 12,7 @@", // missing + sign
		"@@ -10,5 @@",      // missing head
		"@@ -a,b +c,d @@",  // non-numeric
	}
	for _, in := range cases {
		if _, err := parser.ParseHunkHeader(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestParsePatch_Simple(t *testing.T) {
	t.Parallel()
	in := `diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@
 a
-b
+B
+c
 d
`
	got, err := parser.ParsePatch(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	e := got[0]
	if e.APath != "a.go" || e.BPath != "a.go" {
		t.Errorf("paths: %+v", e)
	}
	if e.Binary {
		t.Errorf("binary should be false")
	}
	if len(e.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(e.Hunks))
	}
	h := e.Hunks[0]
	wantPrefixes := []byte{' ', '-', '+', '+', ' '}
	wantTexts := []string{"a", "b", "B", "c", "d"}
	if len(h.Lines) != len(wantPrefixes) {
		t.Fatalf("lines = %d, want %d", len(h.Lines), len(wantPrefixes))
	}
	for i, l := range h.Lines {
		if l.Prefix != wantPrefixes[i] || l.Text != wantTexts[i] {
			t.Errorf("line %d = %+v", i, l)
		}
	}
}

func TestParsePatch_NoNewlineAtEOF(t *testing.T) {
	t.Parallel()
	in := `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
\ No newline at end of file
+new
\ No newline at end of file
`
	got, err := parser.ParsePatch(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Hunks) != 1 {
		t.Fatalf("got = %+v", got)
	}
	lines := got[0].Hunks[0].Lines
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2 (no-newline marker should be skipped)", len(lines))
	}
	if lines[0].Prefix != '-' || lines[0].Text != "old" {
		t.Errorf("line 0 = %+v", lines[0])
	}
	if lines[1].Prefix != '+' || lines[1].Text != "new" {
		t.Errorf("line 1 = %+v", lines[1])
	}
}

func TestParsePatch_BinaryHeader(t *testing.T) {
	t.Parallel()
	in := `diff --git a/img.png b/img.png
index 1111111..2222222 100644
Binary files a/img.png and b/img.png differ
`
	got, err := parser.ParsePatch(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	if !got[0].Binary {
		t.Errorf("binary not flagged: %+v", got[0])
	}
	if len(got[0].Hunks) != 0 {
		t.Errorf("binary should have no hunks: %+v", got[0])
	}
}

func TestParsePatch_MultipleFiles(t *testing.T) {
	t.Parallel()
	in := `diff --git a/a b/a
index 1..2 100644
--- a/a
+++ b/a
@@ -1 +1 @@
-x
+y
diff --git a/b b/b
index 3..4 100644
--- a/b
+++ b/b
@@ -1 +1 @@
-p
+q
`
	got, err := parser.ParsePatch(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %d, want 2", len(got))
	}
	if got[0].BPath != "a" || got[1].BPath != "b" {
		t.Errorf("paths: %+v %+v", got[0], got[1])
	}
}

func TestParsePatch_RenameOnlyNoHunks(t *testing.T) {
	t.Parallel()
	// Pure rename emits no hunks.
	in := `diff --git a/old.go b/new.go
similarity index 100%
rename from old.go
rename to new.go
`
	got, err := parser.ParsePatch(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	if got[0].APath != "old.go" || got[0].BPath != "new.go" {
		t.Errorf("paths: %+v", got[0])
	}
	if len(got[0].Hunks) != 0 {
		t.Errorf("rename-only should have no hunks: %+v", got[0])
	}
}

// --- Diff() integration ---

func TestRepoDiff_Integration(t *testing.T) {
	dir := makeRepo(t)
	writeAndCommit(t, dir, "keep.go", []byte("a\nb\nc\n"), "init")
	writeAndCommit(t, dir, "delme.txt", []byte("bye\n"), "add delme")
	mustGit(t, dir, "checkout", "-q", "-b", "feature")

	if err := os.WriteFile(filepath.Join(dir, "keep.go"), []byte("a\nB\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "added.go"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "delme.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "img.bin"),
		[]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "feature changes")

	repo := &Repo{Workdir: dir}
	files, err := repo.Diff(DiffSpec{Source: SourceRange, Base: "main"})
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string]diffmodel.File{}
	for _, f := range files {
		byPath[f.DisplayPath()] = f
	}

	keep, ok := byPath["keep.go"]
	if !ok {
		t.Fatalf("keep.go missing: %+v", files)
	}
	if keep.Status != diffmodel.StatusModified {
		t.Errorf("keep.go status = %q", keep.Status)
	}
	if len(keep.Hunks) == 0 {
		t.Errorf("keep.go missing hunks: %+v", keep)
	}

	added, ok := byPath["added.go"]
	if !ok || added.Status != diffmodel.StatusAdded {
		t.Errorf("added.go: %+v", added)
	}
	if len(added.Hunks) == 0 {
		t.Errorf("added.go missing hunks")
	}

	del, ok := byPath["delme.txt"]
	if !ok || del.Status != diffmodel.StatusDeleted {
		t.Errorf("delme.txt: %+v", del)
	}

	bin, ok := byPath["img.bin"]
	if !ok {
		t.Fatalf("img.bin missing: %+v", files)
	}
	if !bin.Binary {
		t.Errorf("img.bin not flagged binary: %+v", bin)
	}
	if len(bin.Hunks) != 0 {
		t.Errorf("binary file should have no hunks: %+v", bin)
	}
}
