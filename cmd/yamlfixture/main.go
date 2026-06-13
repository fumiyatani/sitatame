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
