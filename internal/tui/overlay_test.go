package tui

import (
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// numberedFile produces a file with a single context-only hunk so all line
// numbers are populated on both sides.
func numberedFile(prePath, postPath, blobBase, blobHead string, lineCount int) diffmodel.File {
	h := diffmodel.Hunk{
		BaseStart: 1, BaseLines: lineCount,
		HeadStart: 1, HeadLines: lineCount,
	}
	for i := 0; i < lineCount; i++ {
		h.Lines = append(h.Lines, diffmodel.Line{Prefix: ' ', Text: "x"})
	}
	diffmodel.AssignLineNumbers(&h)
	status := diffmodel.StatusModified
	if prePath != postPath {
		status = diffmodel.StatusRenamed
	}
	return diffmodel.File{
		Status:   status,
		PrePath:  prePath, PostPath: postPath,
		BlobBase: blobBase, BlobHead: blobHead,
		RenameFrom: func() string {
			if prePath != postPath {
				return prePath
			}
			return ""
		}(),
		RenameTo: func() string {
			if prePath != postPath {
				return postPath
			}
			return ""
		}(),
		Hunks: []diffmodel.Hunk{h},
	}
}

func TestOverlay_OpenMarkerOnAnchoredLine(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
		State:  review.StateOpen,
		Body:   "hi",
	}}}
	m := New([]diffmodel.File{f}, r)

	// rows: 0 file header, 1 hunk header, 2-4 lines (HeadLine=1,2,3).
	hits := m.Overlay()
	got, ok := hits[3] // HeadLine=2 → row index 3
	if !ok || len(got) != 1 {
		t.Fatalf("overlay missing on line 2 row: %+v", hits)
	}
	view := m.View()
	if !strings.Contains(view, markerOpen) {
		t.Errorf("view missing open marker %q", markerOpen)
	}
}

func TestOverlay_RenameOnly_PreservesAnchorOnNewPath(t *testing.T) {
	t.Parallel()
	// Same blobs on both sides → rename-only. Comment was authored against the
	// old path; Validate should bump Path to the new path so the overlay binds
	// to the renamed file's row.
	f := numberedFile("old.go", "new.go", "blobX", "blobX", 2)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{
			Kind: review.KindLine, Path: "old.go", Side: review.SideHead, Line: 1, Blob: "blobX",
		},
		State: review.StateOpen,
	}}}
	review.Validate(&r, []diffmodel.File{f})
	if got := r.Comments[0].State; got != review.StateOpen {
		t.Fatalf("rename-only should remain open, got %q", got)
	}
	if got := r.Comments[0].Path; got != "new.go" {
		t.Fatalf("Validate should rewrite path to new.go, got %q", got)
	}

	m := New([]diffmodel.File{f}, r)
	hits := m.Overlay()
	if len(hits) == 0 {
		t.Fatalf("overlay empty after rename-only validate")
	}
	view := m.View()
	if !strings.Contains(view, "new.go") {
		t.Errorf("view should mention new.go: %s", view)
	}
	if !strings.Contains(view, markerOpen) {
		t.Errorf("view missing open marker after rename-only: %s", view)
	}
}

// TestOverlay_BaseSideAnchorMapsDeletedRow pins issue #36/#19's overlay path:
// a comment stored with Side=base + Line=BaseLine must light up the `-` row in
// a modified hunk, not a context row that happens to share a head-side number.
func TestOverlay_BaseSideAnchorMapsDeletedRow(t *testing.T) {
	t.Parallel()
	// Hunk: ` x` (b=1,h=1), `+y` (h=2), `-z` (b=2).
	f := diffmodel.File{
		Status:   diffmodel.StatusModified,
		PrePath:  "a.go", PostPath: "a.go",
		BlobBase: "bb", BlobHead: "bh",
		Hunks: []diffmodel.Hunk{{
			BaseStart: 1, BaseLines: 2, HeadStart: 1, HeadLines: 2,
			Lines: []diffmodel.Line{
				{Prefix: ' ', Text: "x"},
				{Prefix: '+', Text: "y"},
				{Prefix: '-', Text: "z"},
			},
		}},
	}
	diffmodel.AssignLineNumbers(&f.Hunks[0])
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{
			Kind: review.KindLine, Path: "a.go", Side: review.SideBase, Line: 2, Blob: "bb",
		},
		State: review.StateOpen,
		Body:  "undo this",
	}}}
	m := New([]diffmodel.File{f}, r)

	// rows: 0 hdr, 1 hunk hdr, 2 ` x`, 3 `+y`, 4 `-z`.
	hits := m.Overlay()
	got, ok := hits[4]
	if !ok || len(got) != 1 {
		t.Fatalf("overlay missing on `-z` row (idx 4): %+v", hits)
	}
	// And it must NOT light up the `+y` row (idx 3) even though that row's
	// HeadLine=2 matches anchor.Line=2 — Side selects the side, not the
	// number.
	if _, on := hits[3]; on {
		t.Errorf("overlay erroneously lit on `+y` row: %+v", hits)
	}
}

func TestOverlay_RenameEdit_StaleAndNoEdit(t *testing.T) {
	t.Parallel()
	// Path moved AND blob changed → stale.
	f := numberedFile("old.go", "new.go", "oldBlob", "newBlob", 2)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{
			Kind: review.KindLine, Path: "old.go", Side: review.SideHead, Line: 1, Blob: "missingBlob",
		},
		State: review.StateOpen, // pre-validation
		Body:  "old body",
	}}}
	review.Validate(&r, []diffmodel.File{f})
	if got := r.Comments[0].State; got != review.StateStale {
		t.Fatalf("rename+edit must mark stale, got %q", got)
	}

	m := New([]diffmodel.File{f}, r)
	view := m.View()
	if !strings.Contains(view, markerStale) {
		t.Errorf("view missing stale marker %q: %s", markerStale, view)
	}

	// `c` on the stale-anchored row should not mutate the existing comment —
	// it appends a fresh one. The original stale comment must survive
	// untouched (read-only).
	// Walk to a content line first.
	m, _ = applyKey(m, "j") // hunk header
	m, _ = applyKey(m, "j") // first content line
	m, _ = applyKey(m, "c")
	m = typeBody(m, "new note")
	m = modalSendSave(m)

	if len(m.Review.Comments) != 2 {
		t.Fatalf("expected original + new comment, got %d", len(m.Review.Comments))
	}
	if got := m.Review.Comments[0]; got.State != review.StateStale || got.Body != "old body" {
		t.Errorf("original stale comment was mutated: %+v", got)
	}
}
