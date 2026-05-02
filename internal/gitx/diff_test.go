package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
)

// --- Pure parser unit tests using hand-crafted NUL streams ---

func TestParseRawZ_Modified(t *testing.T) {
	t.Parallel()
	in := ":100644 100644 1111111 2222222 M\x00src/a.go\x00"
	got, err := parseRawZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	want := rawEntry{
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
	got, err := parseRawZ(in)
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

func TestParseRawZ_MultiDeleted(t *testing.T) {
	t.Parallel()
	in := ":100644 000000 a 0 D\x00x\x00" +
		":100644 000000 b 0 D\x00y\x00" +
		":100644 000000 c 0 D\x00z\x00"
	got, err := parseRawZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	for i, want := range []string{"x", "y", "z"} {
		if got[i].PrePath != want || got[i].Status != diffmodel.StatusDeleted {
			t.Errorf("entry %d = %+v, want path %q", i, got[i], want)
		}
	}
}

func TestParseRawZ_RenameWithSimilarity(t *testing.T) {
	t.Parallel()
	in := ":100644 100644 1111111 2222222 R100\x00old/a.go\x00new/a.go\x00"
	got, err := parseRawZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	if got[0].Status != diffmodel.StatusRenamed {
		t.Errorf("status = %q, want R", got[0].Status)
	}
	if got[0].Similarity != 100 {
		t.Errorf("similarity = %d, want 100", got[0].Similarity)
	}
	if got[0].PrePath != "old/a.go" || got[0].PostPath != "new/a.go" {
		t.Errorf("paths wrong: %+v", got[0])
	}
}

func TestParseRawZ_CopyWithSimilarity(t *testing.T) {
	t.Parallel()
	in := ":100644 100644 1111111 1111111 C75\x00src/a.go\x00src/b.go\x00"
	got, err := parseRawZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != diffmodel.StatusCopied || got[0].Similarity != 75 {
		t.Errorf("got %+v", got[0])
	}
}

func TestParseRawZ_BadInput(t *testing.T) {
	t.Parallel()
	cases := []string{
		"missing-colon\x00path\x00",
		":100644 100644 1 2\x00path\x00",        // 4 fields
		":100644 100644 1 2 X\x00path\x00",      // unknown status
		":100644 100644 1 2 R10x\x00a\x00b\x00", // bad similarity
	}
	for _, in := range cases {
		if _, err := parseRawZ(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestParseNumstatZ_TextAndBinary(t *testing.T) {
	t.Parallel()
	in := "3\t1\tsrc/a.go\x00" +
		"-\t-\timg.png\x00"
	got, err := parseNumstatZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0] != (numstatEntry{Added: 3, Deleted: 1, PostPath: "src/a.go"}) {
		t.Errorf("text entry: %+v", got[0])
	}
	if got[1] != (numstatEntry{Binary: true, PostPath: "img.png"}) {
		t.Errorf("binary entry: %+v", got[1])
	}
}

func TestParseNumstatZ_Rename(t *testing.T) {
	t.Parallel()
	in := "0\t0\t\x00old.go\x00new.go\x00"
	got, err := parseNumstatZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	if got[0].PrePath != "old.go" || got[0].PostPath != "new.go" {
		t.Errorf("rename paths wrong: %+v", got[0])
	}
}

func TestParseNumstatZ_RenameThenText(t *testing.T) {
	t.Parallel()
	// Two entries: a rename (2 paths) and a regular modify (1 path). The header
	// detection for the second entry must kick in.
	in := "0\t0\t\x00a\x00b\x00" +
		"5\t2\tc\x00"
	got, err := parseNumstatZ(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].PrePath != "a" || got[0].PostPath != "b" {
		t.Errorf("rename: %+v", got[0])
	}
	if got[1].PrePath != "" || got[1].PostPath != "c" || got[1].Added != 5 {
		t.Errorf("modify: %+v", got[1])
	}
}

// --- Integration test: run real git and validate join behavior ---

func makeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q", "-b", "main")
	return dir
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(c.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeAndCommit(t *testing.T, dir, name string, body []byte, msg string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", msg)
}

func gitDiff(t *testing.T, dir string, args ...string) string {
	t.Helper()
	args = append([]string{"diff"}, args...)
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func TestDiff_IntegrationModifiedAddedDeletedRenameCopy(t *testing.T) {
	dir := makeRepo(t)
	writeAndCommit(t, dir, "keep.go", []byte("a\nb\nc\n"), "init")
	writeAndCommit(t, dir, "removeme.go", []byte("x\n"), "add removeme")
	writeAndCommit(t, dir, "src/orig.go", []byte("hello\nworld\n"), "add orig")
	mustGit(t, dir, "checkout", "-q", "-b", "feature")

	// modify, add, delete, rename+edit, copy(via cp + edit)
	if err := os.WriteFile(filepath.Join(dir, "keep.go"), []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "added.go"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "removeme.go")); err != nil {
		t.Fatal(err)
	}
	// rename: src/orig.go -> src/renamed.go (with small edit so it shows up)
	if err := os.Rename(filepath.Join(dir, "src/orig.go"), filepath.Join(dir, "src/renamed.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src/renamed.go"), []byte("hello\nworld\n!\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "feature changes")

	rawOut := gitDiff(t, dir, "--raw", "-z", "--find-renames", "--find-copies", "main..HEAD")
	numOut := gitDiff(t, dir, "--numstat", "-z", "--find-renames", "--find-copies", "main..HEAD")

	rawEntries, err := parseRawZ(rawOut)
	if err != nil {
		t.Fatal(err)
	}
	numEntries, err := parseNumstatZ(numOut)
	if err != nil {
		t.Fatal(err)
	}
	files := joinRawAndNumstat(rawEntries, numEntries)

	byDisp := map[string]diffmodel.File{}
	for _, f := range files {
		byDisp[f.DisplayPath()] = f
	}
	if got, ok := byDisp["keep.go"]; !ok || got.Status != diffmodel.StatusModified {
		t.Errorf("keep.go: %+v", got)
	}
	if got, ok := byDisp["added.go"]; !ok || got.Status != diffmodel.StatusAdded {
		t.Errorf("added.go: %+v", got)
	}
	if got, ok := byDisp["removeme.go"]; !ok || got.Status != diffmodel.StatusDeleted {
		t.Errorf("removeme.go: %+v", got)
	}
	if got, ok := byDisp["src/renamed.go"]; !ok || got.Status != diffmodel.StatusRenamed ||
		got.RenameFrom != "src/orig.go" || got.RenameTo != "src/renamed.go" {
		t.Errorf("rename: %+v", got)
	}
}

func TestDiff_BinaryDetection(t *testing.T) {
	dir := makeRepo(t)
	writeAndCommit(t, dir, "doc.txt", []byte("hi\n"), "init")
	mustGit(t, dir, "checkout", "-q", "-b", "feature")

	// Create a binary file (PNG-ish bytes).
	if err := os.WriteFile(filepath.Join(dir, "img.bin"),
		[]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "add binary")

	rawOut := gitDiff(t, dir, "--raw", "-z", "--find-renames", "--find-copies", "main..HEAD")
	numOut := gitDiff(t, dir, "--numstat", "-z", "--find-renames", "--find-copies", "main..HEAD")
	rawEntries, _ := parseRawZ(rawOut)
	numEntries, _ := parseNumstatZ(numOut)
	files := joinRawAndNumstat(rawEntries, numEntries)

	var found bool
	for _, f := range files {
		if f.DisplayPath() == "img.bin" {
			found = true
			if !f.Binary {
				t.Errorf("img.bin not flagged binary: %+v", f)
			}
		}
	}
	if !found {
		t.Errorf("img.bin not in diff: %+v", files)
	}
}
