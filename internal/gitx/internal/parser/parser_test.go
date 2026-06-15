package parser_test

import (
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/gitx/internal/parser"
)

func TestParseRawZ_Modified(t *testing.T) {
	t.Parallel()
	in := ":100644 100644 1111111 2222222 M\x00src/a.go\x00"
	got, err := parser.ParseRawZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	want := parser.RawEntry{
		SrcMode: "100644", DstMode: "100644",
		SrcSHA: "1111111", DstSHA: "2222222",
		Status:  diffmodel.StatusModified,
		PrePath: "src/a.go", PostPath: "src/a.go",
	}
	if got[0] != want {
		t.Errorf("got %+v\nwant %+v", got[0], want)
	}
}

func TestParseRawZ_AddedDeleted(t *testing.T) {
	t.Parallel()
	in := ":000000 100644 0000000 abcdef0 A\x00new.go\x00" +
		":100644 000000 1111111 0000000 D\x00old.go\x00"
	got, err := parser.ParseRawZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].Status != diffmodel.StatusAdded || got[0].PostPath != "new.go" || got[0].PrePath != "" {
		t.Errorf("added entry wrong: %+v", got[0])
	}
	if got[1].Status != diffmodel.StatusDeleted || got[1].PrePath != "old.go" || got[1].PostPath != "" {
		t.Errorf("deleted entry wrong: %+v", got[1])
	}
}

func TestParseRawZ_RenameWithSimilarity(t *testing.T) {
	t.Parallel()
	in := ":100644 100644 1111111 2222222 R100\x00old/a.go\x00new/a.go\x00"
	got, err := parser.ParseRawZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	if got[0].Status != diffmodel.StatusRenamed || got[0].Similarity != 100 {
		t.Errorf("got %+v", got[0])
	}
}

func TestParseNumstatZ_TextAndBinary(t *testing.T) {
	t.Parallel()
	in := "3\t1\tsrc/a.go\x00" +
		"-\t-\timg.png\x00"
	got, err := parser.ParseNumstatZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0] != (parser.NumstatEntry{Added: 3, Deleted: 1, PostPath: "src/a.go"}) {
		t.Errorf("text entry: %+v", got[0])
	}
	if got[1] != (parser.NumstatEntry{Binary: true, PostPath: "img.png"}) {
		t.Errorf("binary entry: %+v", got[1])
	}
}

func TestJoinRawAndNumstat_BinaryFlag(t *testing.T) {
	t.Parallel()
	raw := []parser.RawEntry{
		{SrcMode: "100644", DstMode: "100644", SrcSHA: "a", DstSHA: "b", Status: diffmodel.StatusAdded, PostPath: "img.bin"},
	}
	num := []parser.NumstatEntry{
		{Binary: true, PostPath: "img.bin"},
	}
	files := parser.JoinRawAndNumstat(raw, num)
	if len(files) != 1 {
		t.Fatalf("len=%d, want 1", len(files))
	}
	if !files[0].Binary {
		t.Errorf("img.bin not flagged binary: %+v", files[0])
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
		t.Fatalf("entries=%d, want 1", len(got))
	}
	e := got[0]
	if e.APath != "a.go" || e.BPath != "a.go" {
		t.Errorf("paths: %+v", e)
	}
	if len(e.Hunks) != 1 {
		t.Fatalf("hunks=%d, want 1", len(e.Hunks))
	}
}

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
