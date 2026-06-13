package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sampleReview = `---
schema: 1
id: 20260501T152300-fix-auth
created_at: 2026-05-01T15:23:00+09:00
branch: feature/auth-refactor
base:
  ref: origin/main
  sha: 1a2b3c4d
head:
  ref: HEAD
  sha: abc123def
files:
  - path: src/auth.ts
    blob_base: 4e5f6a7b
    blob_head: 9c8d7e6f
    status: modified
review_comment: |
  全体的に方向性は良い。argon2 への移行は別 PR で進めたい。
comments:
  - anchor_id: 1f3d9f2c-7c1e-4a3a-9c10-aa23bb45cc67
    state: open
    kind: range
    path: src/auth.ts
    side: head
    blob: 9c8d7e6f
    line_start: 10
    line_end: 14
    body: |
      bcrypt ではなく argon2 を使ってほしい。
  - anchor_id: 2a4e9b3d-8c2f-4b4b-a020-bb34cc56dd78
    state: open
    kind: line
    path: src/auth.ts
    side: head
    blob: 9c8d7e6f
    line: 22
    body: 早期 return にしたい。
---

# Review: feature/auth-refactor

## 全体
全体的に方向性は良い。
`

func TestDecode_Sample(t *testing.T) {
	t.Parallel()
	r, err := Decode([]byte(sampleReview))
	if err != nil {
		t.Fatal(err)
	}
	if r.Schema != 1 {
		t.Errorf("schema = %d, want 1", r.Schema)
	}
	if r.ID != "20260501T152300-fix-auth" {
		t.Errorf("id = %q", r.ID)
	}
	if r.Branch != "feature/auth-refactor" {
		t.Errorf("branch = %q", r.Branch)
	}
	if r.Base.Ref != "origin/main" || r.Base.SHA != "1a2b3c4d" {
		t.Errorf("base = %+v", r.Base)
	}
	if r.Head.SHA != "abc123def" {
		t.Errorf("head = %+v", r.Head)
	}
	wantTime := time.Date(2026, 5, 1, 15, 23, 0, 0, time.FixedZone("", 9*3600))
	if !r.CreatedAt.Equal(wantTime) {
		t.Errorf("created_at = %v, want %v", r.CreatedAt, wantTime)
	}
	if len(r.Files) != 1 {
		t.Fatalf("files = %d", len(r.Files))
	}
	if r.Files[0].Path != "src/auth.ts" || r.Files[0].Status != "modified" {
		t.Errorf("file[0] = %+v", r.Files[0])
	}
	if len(r.Comments) != 2 {
		t.Fatalf("comments = %d", len(r.Comments))
	}
	c0 := r.Comments[0]
	if c0.Kind != KindRange || c0.LineStart != 10 || c0.LineEnd != 14 || c0.State != StateOpen {
		t.Errorf("comments[0] = %+v", c0)
	}
	c1 := r.Comments[1]
	if c1.Kind != KindLine || c1.Line != 22 {
		t.Errorf("comments[1] = %+v", c1)
	}
	if !strings.Contains(r.Body, "# Review: feature/auth-refactor") {
		t.Errorf("body missing heading: %q", r.Body)
	}
}

func TestEncode_Roundtrip_Known(t *testing.T) {
	t.Parallel()
	r, err := Decode([]byte(sampleReview))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Decode(out)
	if err != nil {
		t.Fatalf("re-decode: %v\n%s", err, out)
	}
	if r2.ID != r.ID || r2.Schema != r.Schema || r2.Branch != r.Branch {
		t.Errorf("top mismatch: %+v vs %+v", r, r2)
	}
	if !r2.CreatedAt.Equal(r.CreatedAt) {
		t.Errorf("created_at mismatch: %v vs %v", r.CreatedAt, r2.CreatedAt)
	}
	if r2.Base != r.Base || r2.Head != r.Head {
		t.Errorf("ref mismatch")
	}
	if len(r2.Files) != len(r.Files) || len(r2.Comments) != len(r.Comments) {
		t.Errorf("count mismatch")
	}
	if r2.Comments[0].LineStart != 10 || r2.Comments[0].LineEnd != 14 {
		t.Errorf("range mismatch: %+v", r2.Comments[0])
	}
}

func TestEncode_PreservesUnknownTopKey(t *testing.T) {
	t.Parallel()
	in := `---
schema: 1
id: x
created_at: 2026-05-01T00:00:00Z
branch: feat
base:
  ref: origin/main
  sha: aaa
head:
  ref: HEAD
  sha: bbb
custom_top: hello-world
labels:
  - urgent
  - security
---

body
`
	r, err := Decode([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if r.Extras == nil || r.Extras["custom_top"] == nil {
		t.Fatalf("custom_top not captured: %+v", r.Extras)
	}
	if r.Extras["labels"] == nil {
		t.Fatalf("labels not captured")
	}

	out, err := Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "custom_top: hello-world") {
		t.Errorf("custom_top missing in output:\n%s", s)
	}
	if !strings.Contains(s, "- urgent") || !strings.Contains(s, "- security") {
		t.Errorf("labels missing in output:\n%s", s)
	}
}

func TestEncode_PreservesUnknownFileAndCommentKey(t *testing.T) {
	t.Parallel()
	in := `---
schema: 1
id: x
created_at: 2026-05-01T00:00:00Z
branch: feat
base:
  ref: origin/main
  sha: a
head:
  ref: HEAD
  sha: b
files:
  - path: src/a.go
    blob_base: aa
    blob_head: bb
    status: modified
    file_extension_extra: hello
comments:
  - anchor_id: 11111111-1111-1111-1111-111111111111
    state: open
    kind: line
    path: src/a.go
    side: head
    blob: bb
    line: 5
    body: hi
    custom_meta:
      tag: design
---

body
`
	r, err := Decode([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if r.Files[0].Extras["file_extension_extra"] == nil {
		t.Fatalf("file extra not captured: %+v", r.Files[0].Extras)
	}
	if r.Comments[0].Extras["custom_meta"] == nil {
		t.Fatalf("comment extra not captured: %+v", r.Comments[0].Extras)
	}
	out, err := Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "file_extension_extra: hello") {
		t.Errorf("file extra missing:\n%s", s)
	}
	if !strings.Contains(s, "custom_meta:") || !strings.Contains(s, "tag: design") {
		t.Errorf("comment extra missing:\n%s", s)
	}

	// Confirm the second round-trip is also stable.
	r2, err := Decode(out)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Files[0].Extras["file_extension_extra"] == nil ||
		r2.Comments[0].Extras["custom_meta"] == nil {
		t.Errorf("extras dropped on second round-trip")
	}
}

// TestEncode_RoundtripWebFixtures is the Go-side counterpart of the Kotlin
// RoundtripTest. It walks every YAML fixture under web/fixtures/ and asserts
// that Decode + Encode is a fixed point on each one. If any fixture ever stops
// being a fixed point on the Go side (e.g. because the encoder learned a new
// canonical form), the generator self-check at cmd/yamlfixture would catch it
// — but only when someone remembers to run it. This test keeps the property
// honest under `go test ./...` so regular CI catches the same regression.
func TestEncode_RoundtripWebFixtures(t *testing.T) {
	t.Parallel()
	// The test binary runs with cwd = package dir (internal/review). The
	// fixtures live at <repo>/web/fixtures.
	dir := filepath.Join("..", "..", "web", "fixtures")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fixture dir %s: %v", dir, err)
	}

	// with-yaml-comments.yaml is hand-written verbatim because gopkg.in/yaml.v3
	// does not preserve inline YAML comments through a struct round-trip. It is
	// part of the Kotlin gate only; the Go encoder is not expected to reproduce
	// it bit-for-bit, so skip it here. See cmd/yamlfixture/main.go for context.
	skip := map[string]bool{
		"with-yaml-comments.yaml": true,
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if skip[e.Name()] {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, name)
			in, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			r, err := Decode(in)
			if err != nil {
				t.Fatalf("decode %s: %v", name, err)
			}
			out, err := Encode(r)
			if err != nil {
				t.Fatalf("encode %s: %v", name, err)
			}
			if string(out) != string(in) {
				t.Fatalf("round-trip not idempotent for %s\n--- input ---\n%s\n--- output ---\n%s",
					name, in, out)
			}
		})
		count++
	}

	// Coverage floor: if a future PR removes fixtures the round-trip test
	// becomes meaningless. Mirror Kotlin's MIN_FIXTURE_COUNT.
	const minFixtures = 12 // 13 total minus with-yaml-comments
	if count < minFixtures {
		t.Errorf("fixture count regression: found %d eligible fixtures under %s, want >= %d",
			count, dir, minFixtures)
	}
}

// TestEncode_DeletedLineAnchor exercises the Side=base / Blob=BlobBase shape
// introduced by sitatame#61: a comment on a line that exists only on the base
// side of the diff. The fixture documents the canonical form; this test
// asserts the struct-level fields decode the way the rest of the codebase
// expects.
func TestEncode_DeletedLineAnchor(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "web", "fixtures", "deleted-line-anchor.yaml")
	in, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture missing (run `make web-fixtures`): %v", err)
	}
	r, err := Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(r.Comments))
	}
	c := r.Comments[0]
	if c.Side != SideBase {
		t.Errorf("side = %q, want %q", c.Side, SideBase)
	}
	if c.Blob == "" {
		t.Errorf("blob unexpectedly empty for a base-side anchor")
	}
	if c.Line == 0 {
		t.Errorf("line unexpectedly zero for a base-side anchor")
	}
}

// TestEncode_RangeCommentMultiState asserts the range-comment fixture decodes
// three comments sharing the same anchor range with distinct states. This is
// the real-world shape after a reviewer leaves multiple notes on the same
// block over time (open -> resolved -> stale).
func TestEncode_RangeCommentMultiState(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "web", "fixtures", "range-comment.yaml")
	in, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture missing (run `make web-fixtures`): %v", err)
	}
	r, err := Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Comments) != 3 {
		t.Fatalf("comments = %d, want 3", len(r.Comments))
	}
	wantStates := []State{StateOpen, StateResolved, StateStale}
	for i, want := range wantStates {
		c := r.Comments[i]
		if c.Kind != KindRange {
			t.Errorf("comments[%d].kind = %q, want range", i, c.Kind)
		}
		if c.LineStart != 10 || c.LineEnd != 14 {
			t.Errorf("comments[%d] range = %d..%d, want 10..14", i, c.LineStart, c.LineEnd)
		}
		if c.State != want {
			t.Errorf("comments[%d].state = %q, want %q", i, c.State, want)
		}
	}
}

// TestEncode_ExtrasEverywherePreserved decodes the extras-everywhere fixture
// and asserts that unknown keys at top / file / comment level all land in the
// corresponding Extras maps. This is the structured assertion behind the
// bit-exact round-trip: if Extras drop a key, the round-trip would still pass
// but downstream consumers would lose the data.
func TestEncode_ExtrasEverywherePreserved(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "web", "fixtures", "extras-everywhere.yaml")
	in, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture missing (run `make web-fixtures`): %v", err)
	}
	r, err := Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Extras["experimental_metadata"] == nil {
		t.Errorf("top-level extras missing experimental_metadata: %+v", r.Extras)
	}
	if len(r.Files) == 0 || r.Files[0].Extras["file_owner"] == nil {
		t.Errorf("file extras missing file_owner: %+v", r.Files[0].Extras)
	}
	if len(r.Comments) == 0 {
		t.Fatal("no comments decoded")
	}
	cx := r.Comments[0].Extras
	if cx["reviewer_tag"] == nil || cx["due_date"] == nil {
		t.Errorf("comment extras missing reviewer_tag/due_date: %+v", cx)
	}
}

func TestDecode_RejectsMissingDelim(t *testing.T) {
	t.Parallel()
	cases := []string{
		"no front matter at all",
		"---\nfoo: bar\n",      // missing closing
		"---\nfoo: bar\n--\n",  // wrong closing token
	}
	for _, in := range cases {
		if _, err := Decode([]byte(in)); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}
