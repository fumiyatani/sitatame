// Command yamlfixture regenerates the YAML round-trip fixtures consumed by the
// Kotlin Web UI PoC (web/fixtures/*.yaml).
//
// It runs the canonical Go encoder (internal/review.Encode) over a small set of
// hand-written inputs and writes the encoder output to disk. The encoder output
// is what the Kotlin codec must reproduce bit-for-bit on a decode/encode round
// trip. By emitting the encoder output (rather than the hand-written input),
// the generator guarantees the Go side is already idempotent for each fixture:
//
//	Decode(Encode(Decode(input))) == Decode(Encode(input))
//
// One fixture (with-yaml-comments.yaml) intentionally bypasses the Go encoder
// because YAML inline comments are not preserved by gopkg.in/yaml.v3 when
// re-marshalling a struct. That fixture is written verbatim and exists solely
// to probe whether snakeyaml-engine on the Kotlin side can preserve comments.
// If the Kotlin round-trip fails on that fixture, the Web UI route's Kill
// criteria for YAML comment preservation is triggered.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fumiyatani/sitatame/internal/review"
)

// fixture pairs an output filename with the input YAML to feed through the
// Go codec.
type fixture struct {
	name string
	in   string
}

// goEncodedFixtures are produced by running review.Decode then review.Encode
// over the input. The output is what the Kotlin side must round-trip.
var goEncodedFixtures = []fixture{
	{
		name: "minimal.yaml",
		in: `---
schema: 1
id: 20260501T100000-minimal
created_at: 2026-05-01T10:00:00Z
branch: feature/minimal
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
comments:
  - anchor_id: 11111111-1111-1111-1111-111111111111
    state: open
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 10
    body: please rename this variable.
---

# Review: minimal
`,
	},
	{
		name: "unknown-top.yaml",
		in: `---
schema: 1
id: 20260501T100100-unknown-top
created_at: 2026-05-01T10:01:00Z
branch: feature/unknown-top
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
experimental_metadata:
  owner: web-team
  priority: high
  tags:
    - poc
    - schema-drift
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
comments:
  - anchor_id: 22222222-2222-2222-2222-222222222222
    state: open
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 5
    body: top-level unknown key must survive.
---

# Review: unknown-top
`,
	},
	{
		name: "unknown-comment.yaml",
		in: `---
schema: 1
id: 20260501T100200-unknown-comment
created_at: 2026-05-01T10:02:00Z
branch: feature/unknown-comment
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
comments:
  - anchor_id: 33333333-3333-3333-3333-333333333333
    state: open
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 7
    body: comment-level unknown key must survive.
    extras_field: future-use
    reviewer_meta:
      assignee: alice
      due: 2026-05-31
---

# Review: unknown-comment
`,
	},
	{
		name: "array-order.yaml",
		in: `---
schema: 1
id: 20260501T100300-array-order
created_at: 2026-05-01T10:03:00Z
branch: feature/array-order
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
comments:
  - anchor_id: aaaaaaaa-0000-0000-0000-000000000001
    state: open
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 1
    body: first
  - anchor_id: aaaaaaaa-0000-0000-0000-000000000002
    state: open
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 2
    body: second
  - anchor_id: aaaaaaaa-0000-0000-0000-000000000003
    state: resolved
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 3
    body: third
  - anchor_id: aaaaaaaa-0000-0000-0000-000000000004
    state: stale
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 4
    body: fourth
---

# Review: array-order
`,
	},
	{
		// deleted-line-anchor exercises the PR #61 fix: a comment that points to
		// a line that only exists on the base side of the diff (a `-` line in
		// unified-diff terms). The anchor uses Side=base + Blob=BlobBase + the
		// pre-deletion Line number. Round-tripping must keep all three fields.
		name: "deleted-line-anchor.yaml",
		in: `---
schema: 1
id: 20260501T100500-deleted-line-anchor
created_at: 2026-05-01T10:05:00Z
branch: feature/deleted-line-anchor
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
comments:
  - anchor_id: 55555555-5555-5555-5555-555555555555
    kind: line
    path: src/app.go
    side: base
    blob: aa
    line: 42
    state: open
    body: this deleted line was load-bearing; restore it or document why it went away.
---

# Review: deleted-line-anchor
`,
	},
	{
		// rename-file pairs a file rename (rename_from / rename_to / similarity)
		// with one line comment on the renamed file and one range comment, so
		// the rename metadata and per-comment kinds both round-trip.
		name: "rename-file.yaml",
		in: `---
schema: 1
id: 20260501T100600-rename-file
created_at: 2026-05-01T10:06:00Z
branch: feature/rename-file
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/new.go
    blob_base: aa
    blob_head: bb
    status: renamed
    rename_from: src/old.go
    rename_to: src/new.go
    similarity: 95
comments:
  - anchor_id: 66666666-6666-6666-6666-666666666666
    kind: line
    path: src/new.go
    side: head
    blob: bb
    line: 5
    state: open
    body: rename looks fine, but please update the package doc.
  - anchor_id: 77777777-7777-7777-7777-777777777777
    kind: range
    path: src/new.go
    side: head
    blob: bb
    line_start: 20
    line_end: 24
    state: open
    body: this block is a verbatim copy from the old file; consider extracting.
---

# Review: rename-file
`,
	},
	{
		// stale-comment captures a real review state: the head blob hash on the
		// anchor no longer matches any current blob, so the comment is marked
		// stale. Round-tripping must keep the drifted blob hash verbatim so a
		// later "reattach" pass can compare against the original.
		name: "stale-comment.yaml",
		in: `---
schema: 1
id: 20260501T100700-stale-comment
created_at: 2026-05-01T10:07:00Z
branch: feature/stale-comment
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
comments:
  - anchor_id: 88888888-8888-8888-8888-888888888888
    kind: line
    path: src/app.go
    side: head
    blob: bbold
    line: 42
    state: stale
    body: anchor drifted after a rebase; reattach before resolving.
---

# Review: stale-comment
`,
	},
	{
		// review-level-comment exercises the top-level review_comment field with
		// a multi-line block scalar and an empty comments[] list (omitted from
		// the encoder output because Comments has omitempty).
		name: "review-level-comment.yaml",
		in: `---
schema: 1
id: 20260501T100800-review-level-comment
created_at: 2026-05-01T10:08:00Z
branch: feature/review-level-comment
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
review_comment: |
  overall direction is fine.
  let's defer the argon2 migration to a separate PR.
  the diff itself is small enough to merge today.
---

# Review: review-level-comment
`,
	},
	{
		// range-comment puts three comments (open / resolved / stale) on the
		// same range anchor. The encoder must keep the per-comment order stable
		// even when several comments share kind=range and line_start/line_end.
		name: "range-comment.yaml",
		in: `---
schema: 1
id: 20260501T100900-range-comment
created_at: 2026-05-01T10:09:00Z
branch: feature/range-comment
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
comments:
  - anchor_id: 99999999-9999-9999-9999-000000000001
    kind: range
    path: src/app.go
    side: head
    blob: bb
    line_start: 10
    line_end: 14
    state: open
    body: this block needs a comment explaining the invariant.
  - anchor_id: 99999999-9999-9999-9999-000000000002
    kind: range
    path: src/app.go
    side: head
    blob: bb
    line_start: 10
    line_end: 14
    state: resolved
    body: invariant documented in commit abcdef0; closing this thread.
  - anchor_id: 99999999-9999-9999-9999-000000000003
    kind: range
    path: src/app.go
    side: head
    blob: bb
    line_start: 10
    line_end: 14
    state: stale
    body: original concern no longer applies after the refactor.
---

# Review: range-comment
`,
	},
	{
		// multi-file covers three files: a modified file with two comments, a
		// modified file with one line + one range comment, and an added file
		// (no blob_base) with one file-level (kind=file) comment. Comments are
		// interleaved across files to lock in array ordering across paths.
		name: "multi-file.yaml",
		in: `---
schema: 1
id: 20260501T101000-multi-file
created_at: 2026-05-01T10:10:00Z
branch: feature/multi-file
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/a.go
    blob_base: a1
    blob_head: a2
    status: modified
  - path: src/b.go
    blob_base: b1
    blob_head: b2
    status: modified
  - path: src/c.go
    blob_head: c2
    status: added
comments:
  - anchor_id: aaaaaaaa-aaaa-aaaa-aaaa-000000000001
    kind: line
    path: src/a.go
    side: head
    blob: a2
    line: 1
    state: open
    body: a-file comment 1
  - anchor_id: bbbbbbbb-bbbb-bbbb-bbbb-000000000001
    kind: line
    path: src/b.go
    side: head
    blob: b2
    line: 3
    state: open
    body: b-file comment 1
  - anchor_id: aaaaaaaa-aaaa-aaaa-aaaa-000000000002
    kind: line
    path: src/a.go
    side: head
    blob: a2
    line: 2
    state: resolved
    body: a-file comment 2 (resolved)
  - anchor_id: bbbbbbbb-bbbb-bbbb-bbbb-000000000002
    kind: range
    path: src/b.go
    side: head
    blob: b2
    line_start: 10
    line_end: 12
    state: open
    body: b-file range comment
  - anchor_id: cccccccc-cccc-cccc-cccc-000000000001
    kind: file
    path: src/c.go
    state: open
    body: new file looks fine overall; one nit below.
---

# Review: multi-file
`,
	},
	{
		// extras-everywhere places unknown keys at every level: top-level
		// (experimental_metadata), files[0] (file_owner), comments[0]
		// (reviewer_tag + due_date map). All three must survive round-trip
		// in their respective Extras maps.
		name: "extras-everywhere.yaml",
		in: `---
schema: 1
id: 20260501T101100-extras-everywhere
created_at: 2026-05-01T10:11:00Z
branch: feature/extras-everywhere
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
    file_owner: alice
comments:
  - anchor_id: dddddddd-dddd-dddd-dddd-dddddddddddd
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 5
    state: open
    body: unknown keys live at every level.
    reviewer_tag: nitpick
    due_date:
      iso: 2026-05-31
      timezone: Asia/Tokyo
experimental_metadata:
  campaign: x42
  priority: high
  tags:
    - poc
    - schema-drift
---

# Review: extras-everywhere
`,
	},
	{
		// utf8-body keeps the front matter ASCII and pushes Japanese text into
		// the comment / review_comment bodies via a literal block scalar (|).
		// Block style avoids emitter-specific quoting differences for non-ASCII
		// scalars and exercises the UTF-8 transport path end-to-end.
		name: "utf8-body.yaml",
		in: `---
schema: 1
id: 20260501T101200-utf8-body
created_at: 2026-05-01T10:12:00Z
branch: feature/utf8-body
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
review_comment: |
  全体の方向性は良い。
  詳細は別 PR で詰めたい。
comments:
  - anchor_id: eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 5
    state: open
    body: |
      日本語コメント。
      この行は CJK 文字と全角記号「」を含む。
      末尾改行ありで block scalar に倒す。
---

# Review: utf8-body
`,
	},
}

// withYamlCommentsFixture is written verbatim because gopkg.in/yaml.v3 does
// not preserve inline (#) comments when re-marshalling structs through the
// canonical Encode path. The fixture probes the Kotlin snakeyaml-engine side
// only: if Kotlin can decode + emit this byte-for-byte, the Web UI route's
// Kill criterion for YAML comment preservation is satisfied.
//
// Note on comment placement: snakeyaml-engine attaches a comment that appears
// before a block-sequence item to the item itself, not to the sequence parent.
// Its round-trip emits `- # comment\n    key: value` rather than
//
//	# comment
//	- key: value
//
// That is semantically equivalent and still satisfies the route's Kill
// criterion (comments survive); the fixture is written in the
// engine-normalised form so the bit-exact assertion can hold.
const withYamlCommentsFixture = `---
# top-level note: this fixture exercises YAML comment preservation
schema: 1
id: 20260501T100400-with-yaml-comments
created_at: 2026-05-01T10:04:00Z
branch: feature/with-yaml-comments
base:
  ref: origin/main
  sha: aaaaaaa
head:
  ref: HEAD
  sha: bbbbbbb
files:
  - # the only file touched in this review
    path: src/app.go
    blob_base: aa
    blob_head: bb
    status: modified
comments:
  - anchor_id: 44444444-4444-4444-4444-444444444444
    state: open
    kind: line
    path: src/app.go
    side: head
    blob: bb
    line: 9
    # reviewer wrote a short note
    body: keep this comment intact across round-trip.
---

# Review: with-yaml-comments
`

func main() {
	var outDir string
	flag.StringVar(&outDir, "out", "web/fixtures", "output directory for generated fixtures")
	flag.Parse()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", outDir, err)
		os.Exit(1)
	}

	// Run the canonical codec round-trip for each generated fixture.
	for _, f := range goEncodedFixtures {
		out, err := encodeFixture(f.in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f.name, err)
			os.Exit(1)
		}
		if err := writeFile(filepath.Join(outDir, f.name), out); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f.name, err)
			os.Exit(1)
		}
		// Self-check: a second round-trip must be a fixed point. If this fails
		// the generator output is non-idempotent and the fixture is unusable.
		if err := assertIdempotent(out); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f.name, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "wrote %s (%d bytes)\n", f.name, len(out))
	}

	// Write the comment-preservation probe verbatim.
	probePath := filepath.Join(outDir, "with-yaml-comments.yaml")
	if err := writeFile(probePath, []byte(withYamlCommentsFixture)); err != nil {
		fmt.Fprintf(os.Stderr, "with-yaml-comments.yaml: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "wrote with-yaml-comments.yaml (verbatim, %d bytes)\n", len(withYamlCommentsFixture))
}

// encodeFixture runs Decode then Encode and returns the canonical Go output.
func encodeFixture(in string) ([]byte, error) {
	r, err := review.Decode([]byte(in))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	out, err := review.Encode(r)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return out, nil
}

// assertIdempotent verifies that running Decode/Encode a second time produces
// the exact same bytes. The generator must not write a fixture that the Go
// codec itself cannot round-trip.
func assertIdempotent(b []byte) error {
	r, err := review.Decode(b)
	if err != nil {
		return fmt.Errorf("self-check decode: %w", err)
	}
	out, err := review.Encode(r)
	if err != nil {
		return fmt.Errorf("self-check encode: %w", err)
	}
	if string(out) != string(b) {
		return fmt.Errorf("self-check: round-trip not idempotent\n--- first ---\n%s\n--- second ---\n%s", b, out)
	}
	return nil
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
