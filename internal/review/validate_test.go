package review

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
)

func makeReview(comments ...Comment) Review {
	return Review{Comments: comments}
}

// makeFileWithDeletedLine returns a modified file whose hunk contains one `-`
// row (BaseLine=baseLine, HeadLine=0) and one `+` row (BaseLine=0,
// HeadLine=headLine), so test anchors can reference both sides unambiguously.
func makeFileWithDeletedLine(path, baseBlob, headBlob string, baseLine, headLine int) diffmodel.File {
	return diffmodel.File{
		Status:   diffmodel.StatusModified,
		PrePath:  path,
		PostPath: path,
		BlobBase: baseBlob,
		BlobHead: headBlob,
		Hunks: []diffmodel.Hunk{{
			BaseStart: baseLine, BaseLines: 1,
			HeadStart: headLine, HeadLines: 1,
			Lines: []diffmodel.Line{
				{Prefix: '-', BaseLine: baseLine, HeadLine: 0, Text: "old line"},
				{Prefix: '+', BaseLine: 0, HeadLine: headLine, Text: "new line"},
			},
		}},
	}
}

func TestValidate_BlobMatch_Open(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{{
		Status:   diffmodel.StatusModified,
		PrePath:  "src/a.go",
		PostPath: "src/a.go",
		BlobBase: "old", BlobHead: "new",
	}}
	r := makeReview(Comment{
		Anchor: Anchor{Kind: KindLine, Path: "src/a.go", Side: SideHead, Blob: "new", Line: 5},
		State:  StateStale, // start stale to ensure it gets flipped
	})
	Validate(&r, files)
	if r.Comments[0].State != StateOpen {
		t.Errorf("state = %q, want open", r.Comments[0].State)
	}
	if r.Comments[0].Path != "src/a.go" {
		t.Errorf("path mutated: %+v", r.Comments[0])
	}
}

func TestValidate_BlobMismatchSamePath_Stale(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{{
		Status:   diffmodel.StatusModified,
		PrePath:  "src/a.go",
		PostPath: "src/a.go",
		BlobBase: "old", BlobHead: "new",
	}}
	r := makeReview(Comment{
		Anchor: Anchor{Kind: KindLine, Path: "src/a.go", Side: SideHead, Blob: "stale-blob", Line: 5},
		State:  StateOpen,
	})
	Validate(&r, files)
	if r.Comments[0].State != StateStale {
		t.Errorf("state = %q, want stale", r.Comments[0].State)
	}
}

func TestValidate_FileGone_Stale(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{} // unrelated diff
	r := makeReview(Comment{
		Anchor: Anchor{Kind: KindLine, Path: "src/a.go", Side: SideHead, Blob: "anything", Line: 5},
		State:  StateOpen,
	})
	Validate(&r, files)
	if r.Comments[0].State != StateStale {
		t.Errorf("state = %q, want stale", r.Comments[0].State)
	}
}

func TestValidate_RenameOnly_OpenWithUpdatedPath(t *testing.T) {
	t.Parallel()
	// Rename-only: blob_head matches but path moved.
	files := []diffmodel.File{{
		Status:   diffmodel.StatusRenamed,
		PrePath:  "src/old.go",
		PostPath: "src/new.go",
		BlobBase: "b1", BlobHead: "b2",
		RenameFrom: "src/old.go", RenameTo: "src/new.go",
		Similarity: 100,
	}}
	r := makeReview(Comment{
		Anchor: Anchor{Kind: KindLine, Path: "src/old.go", Side: SideHead, Blob: "b2", Line: 5},
		State:  StateStale,
	})
	Validate(&r, files)
	c := r.Comments[0]
	if c.State != StateOpen {
		t.Errorf("state = %q, want open", c.State)
	}
	if c.Path != "src/new.go" {
		t.Errorf("path = %q, want src/new.go", c.Path)
	}
	if c.RenameFrom != "src/old.go" || c.RenameTo != "src/new.go" {
		t.Errorf("rename meta missing: %+v", c)
	}
}

func TestValidate_RenamePlusEdit_Stale(t *testing.T) {
	t.Parallel()
	// Rename + edit: blob_head differs from anchor.Blob, AND path moved so
	// path lookup with old path also fails. Result: stale.
	files := []diffmodel.File{{
		Status:   diffmodel.StatusRenamed,
		PrePath:  "src/old.go",
		PostPath: "src/new.go",
		BlobBase: "b1", BlobHead: "b3-edited",
		RenameFrom: "src/old.go", RenameTo: "src/new.go",
		Similarity: 80,
	}}
	r := makeReview(Comment{
		Anchor: Anchor{Kind: KindLine, Path: "src/old.go", Side: SideHead, Blob: "b2-original", Line: 5},
		State:  StateOpen,
	})
	Validate(&r, files)
	if r.Comments[0].State != StateStale {
		t.Errorf("state = %q, want stale", r.Comments[0].State)
	}
}

func TestValidate_BaseSide(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{{
		Status:   diffmodel.StatusModified,
		PrePath:  "src/a.go",
		PostPath: "src/a.go",
		BlobBase: "old-blob", BlobHead: "new-blob",
	}}
	r := makeReview(Comment{
		Anchor: Anchor{Kind: KindLine, Path: "src/a.go", Side: SideBase, Blob: "old-blob", Line: 3},
		State:  StateStale,
	})
	Validate(&r, files)
	if r.Comments[0].State != StateOpen {
		t.Errorf("state = %q, want open (base side blob match)", r.Comments[0].State)
	}
}

func TestValidate_KindReview_Untouched(t *testing.T) {
	t.Parallel()
	r := makeReview(Comment{
		Anchor: Anchor{Kind: KindReview},
		State:  StateOpen,
	})
	Validate(&r, nil)
	if r.Comments[0].State != StateOpen {
		t.Errorf("kind=review should not be re-classified, got %q", r.Comments[0].State)
	}
}

// TestValidateWithWarnings_LegacyDeletedLineAnchor_RealBug pins the actual
// issue #36 / #19 buggy shape: openCommentModal on a `-` row of a modified
// file persisted Side=head, Line=<BaseLine>, Blob=<HeadBlob> (because
// lineNumberAt fell back to BaseLine when HeadLine was zero, and blobForSide
// returned BlobHead for Side=head). The validator must detect that and warn,
// because the side metadata contradicts the line number / row identity.
func TestValidateWithWarnings_LegacyDeletedLineAnchor_RealBug(t *testing.T) {
	t.Parallel()
	// `-` row at BaseLine=5, `+` row at HeadLine=8. The buggy anchor points to
	// the `-` row but mis-records Side=head + HeadBlob.
	files := []diffmodel.File{makeFileWithDeletedLine("src/a.go", "oldblob", "newblob", 5, 8)}
	r := makeReview(Comment{
		Anchor: Anchor{
			AnchorID: "legacy-1",
			Kind:     KindLine,
			Path:     "src/a.go",
			Side:     SideHead, // ← bug: head + base-only line number
			Blob:     "newblob",
			Line:     5, // BaseLine value, not HeadLine
		},
		State: StateOpen,
	})

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)

	out := buf.String()
	if !strings.Contains(out, "legacy-1") {
		t.Errorf("warning should name the anchor_id, got %q", out)
	}
	if !strings.Contains(out, "legacy head-side anchor") {
		t.Errorf("warning should mention 'legacy head-side anchor', got %q", out)
	}
	if !strings.Contains(out, "deleted line") {
		t.Errorf("warning should mention 'deleted line', got %q", out)
	}
}

// TestValidateWithWarnings_CorrectAnchor_NoWarning covers the post-fix shape
// for the same `-` row: Side=base, Line=BaseLine, Blob=BlobBase. This must
// produce no warning — the anchor is internally consistent.
func TestValidateWithWarnings_CorrectAnchor_NoWarning(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{makeFileWithDeletedLine("src/a.go", "oldblob", "newblob", 5, 8)}
	r := makeReview(Comment{
		Anchor: Anchor{
			AnchorID: "ok-base",
			Kind:     KindLine,
			Path:     "src/a.go",
			Side:     SideBase,
			Blob:     "oldblob",
			Line:     5,
		},
		State: StateOpen,
	})

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)
	if out := buf.String(); out != "" {
		t.Errorf("expected no warnings for properly-saved base-side anchor, got %q", out)
	}
}

// TestValidateWithWarnings_BothSidesValid_HeadOnPlusLine pins the canonical
// `+` row case: Side=head, Line=HeadLine, Blob=BlobHead. This is the original
// healthy shape and must never trigger the legacy warning.
func TestValidateWithWarnings_BothSidesValid_HeadOnPlusLine(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{makeFileWithDeletedLine("src/a.go", "oldblob", "newblob", 5, 8)}
	r := makeReview(
		Comment{
			Anchor: Anchor{AnchorID: "ok-head", Kind: KindLine, Path: "src/a.go", Side: SideHead, Blob: "newblob", Line: 8},
			State:  StateOpen,
		},
		Comment{
			// File-kind anchor: no line number to validate, must be ignored by
			// the legacy detector regardless of side.
			Anchor: Anchor{AnchorID: "ok-file", Kind: KindFile, Path: "src/a.go", Side: SideHead, Blob: "newblob"},
			State:  StateOpen,
		},
	)

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)
	if out := buf.String(); out != "" {
		t.Errorf("expected no warnings for healthy anchors, got %q", out)
	}
}

// TestValidateWithWarnings_HeadBlobMismatch_NoWarning makes sure the detector
// does not fire when the head-side anchor's blob does not match BlobHead. That
// is a different kind of corruption (or simply a stale anchor) and is left to
// validateAnchor to mark stale.
func TestValidateWithWarnings_HeadBlobMismatch_NoWarning(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{makeFileWithDeletedLine("src/a.go", "oldblob", "newblob", 5, 8)}
	r := makeReview(Comment{
		Anchor: Anchor{
			AnchorID: "weird",
			Kind:     KindLine,
			Path:     "src/a.go",
			Side:     SideHead,
			Blob:     "some-other-blob",
			Line:     5,
		},
		State: StateOpen,
	})

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)
	if out := buf.String(); out != "" {
		t.Errorf("expected no warning when blob does not match BlobHead, got %q", out)
	}
}

// makeFileWithDeletedRange returns a modified file whose hunk contains a run of
// `-` rows for BaseLines [baseStart, baseEnd] and one `+` row at headLine, so
// range-anchor tests can target the all-deleted span unambiguously.
func makeFileWithDeletedRange(path, baseBlob, headBlob string, baseStart, baseEnd, headLine int) diffmodel.File {
	lines := make([]diffmodel.Line, 0, (baseEnd-baseStart+1)+1)
	for n := baseStart; n <= baseEnd; n++ {
		lines = append(lines, diffmodel.Line{Prefix: '-', BaseLine: n, HeadLine: 0, Text: "old"})
	}
	lines = append(lines, diffmodel.Line{Prefix: '+', BaseLine: 0, HeadLine: headLine, Text: "new"})
	return diffmodel.File{
		Status:   diffmodel.StatusModified,
		PrePath:  path,
		PostPath: path,
		BlobBase: baseBlob,
		BlobHead: headBlob,
		Hunks: []diffmodel.Hunk{{
			BaseStart: baseStart, BaseLines: baseEnd - baseStart + 1,
			HeadStart: headLine, HeadLines: 1,
			Lines: lines,
		}},
	}
}

// TestValidateWithWarnings_LegacyRangeAllDeleted pins the range-anchor analogue
// of the issue #36 bug: a KindRange comment with Side=head whose [LineStart,
// LineEnd] is composed entirely of `-` rows (BaseLines), recorded under
// Blob=BlobHead. The validator must warn — the side metadata contradicts the
// span identity.
func TestValidateWithWarnings_LegacyRangeAllDeleted(t *testing.T) {
	t.Parallel()
	// `-` rows at BaseLine 5,6,7; `+` row at HeadLine 8.
	files := []diffmodel.File{makeFileWithDeletedRange("src/a.go", "oldblob", "newblob", 5, 7, 8)}
	r := makeReview(Comment{
		Anchor: Anchor{
			AnchorID:  "legacy-range",
			Kind:      KindRange,
			Path:      "src/a.go",
			Side:      SideHead,
			Blob:      "newblob",
			LineStart: 5,
			LineEnd:   7,
		},
		State: StateOpen,
	})

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)

	out := buf.String()
	if !strings.Contains(out, "legacy-range") {
		t.Errorf("warning should name the anchor_id, got %q", out)
	}
	if !strings.Contains(out, "legacy head-side anchor") {
		t.Errorf("warning should mention 'legacy head-side anchor', got %q", out)
	}
}

// TestValidateWithWarnings_CorrectRangeAllDeleted_NoWarning covers the
// post-fix shape for the same all-deleted span: Side=base, Blob=BlobBase. Must
// produce no warning.
func TestValidateWithWarnings_CorrectRangeAllDeleted_NoWarning(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{makeFileWithDeletedRange("src/a.go", "oldblob", "newblob", 5, 7, 8)}
	r := makeReview(Comment{
		Anchor: Anchor{
			AnchorID:  "ok-range-base",
			Kind:      KindRange,
			Path:      "src/a.go",
			Side:      SideBase,
			Blob:      "oldblob",
			LineStart: 5,
			LineEnd:   7,
		},
		State: StateOpen,
	})

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)
	if out := buf.String(); out != "" {
		t.Errorf("expected no warnings for properly-saved base-side range anchor, got %q", out)
	}
}

// TestValidateWithWarnings_MixedRange_NoWarning covers a range that mixes `-`
// and `+` rows. The new logic intentionally keeps such ranges on Head, so the
// legacy detector must stay silent (no double-warning noise).
func TestValidateWithWarnings_MixedRange_NoWarning(t *testing.T) {
	t.Parallel()
	// Build a file where BaseLines 5,6 are `-` rows and HeadLine 7 is a `+` row.
	// The range anchor spans BaseLine 5..HeadLine 7 — mixed in the row sense
	// because line 7 has a HeadLine match (consistent with Side=head).
	files := []diffmodel.File{{
		Status:   diffmodel.StatusModified,
		PrePath:  "src/a.go",
		PostPath: "src/a.go",
		BlobBase: "oldblob",
		BlobHead: "newblob",
		Hunks: []diffmodel.Hunk{{
			BaseStart: 5, BaseLines: 2,
			HeadStart: 7, HeadLines: 1,
			Lines: []diffmodel.Line{
				{Prefix: '-', BaseLine: 5, HeadLine: 0, Text: "old1"},
				{Prefix: '-', BaseLine: 6, HeadLine: 0, Text: "old2"},
				{Prefix: '+', BaseLine: 0, HeadLine: 7, Text: "new"},
			},
		}},
	}}
	r := makeReview(Comment{
		Anchor: Anchor{
			AnchorID:  "mixed-range",
			Kind:      KindRange,
			Path:      "src/a.go",
			Side:      SideHead,
			Blob:      "newblob",
			LineStart: 5,
			LineEnd:   7,
		},
		State: StateOpen,
	})

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)
	if out := buf.String(); out != "" {
		t.Errorf("expected no warning for mixed range (Head retained by design), got %q", out)
	}
}

// TestValidateWithWarnings_LegacyAnchorWithOverlappingHeadLine covers the
// PR #61 round-4 [P2]: deleting a base line shifts subsequent rows so that the
// same integer can appear as a HeadLine elsewhere in the file (here, the `+`
// row's HeadLine == 5, the same as the deleted BaseLine 5). The legacy buggy
// modal still saved Side=head + Line=5 (BaseLine) + Blob=BlobHead, which
// silently overlays the comment onto the unrelated `+` row. The detector must
// fire — HeadLine overlap is NOT a clean signal.
func TestValidateWithWarnings_LegacyAnchorWithOverlappingHeadLine(t *testing.T) {
	t.Parallel()
	// `-` row at BaseLine=5, `+` row at HeadLine=5. Same integer on both
	// sides, but they refer to different rows.
	files := []diffmodel.File{makeFileWithDeletedLine("src/a.go", "oldblob", "newblob", 5, 5)}
	r := makeReview(Comment{
		Anchor: Anchor{
			AnchorID: "overlap-line",
			Kind:     KindLine,
			Path:     "src/a.go",
			Side:     SideHead,
			Blob:     "newblob",
			Line:     5,
		},
		State: StateOpen,
	})

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)

	out := buf.String()
	if !strings.Contains(out, "overlap-line") {
		t.Errorf("warning should name the anchor_id even when HeadLine overlaps, got %q", out)
	}
	if !strings.Contains(out, "legacy head-side anchor") {
		t.Errorf("warning should mention 'legacy head-side anchor', got %q", out)
	}
}

// TestValidateWithWarnings_LegacyRangeWithOverlappingHeadLines is the range
// analogue of the overlap case. BaseLines 5,6,7 are all `-` rows; HeadLines
// 5,6,7 also exist as `+`/context rows in the same file. The buggy modal still
// saved the all-deleted span under Side=head + Blob=BlobHead, and the detector
// must warn.
func TestValidateWithWarnings_LegacyRangeWithOverlappingHeadLines(t *testing.T) {
	t.Parallel()
	// `-` rows at BaseLine 5,6,7 and `+` rows at HeadLine 5,6,7 — same
	// integers reused on the head side for unrelated rows.
	files := []diffmodel.File{{
		Status:   diffmodel.StatusModified,
		PrePath:  "src/a.go",
		PostPath: "src/a.go",
		BlobBase: "oldblob",
		BlobHead: "newblob",
		Hunks: []diffmodel.Hunk{{
			BaseStart: 5, BaseLines: 3,
			HeadStart: 5, HeadLines: 3,
			Lines: []diffmodel.Line{
				{Prefix: '-', BaseLine: 5, HeadLine: 0, Text: "old1"},
				{Prefix: '-', BaseLine: 6, HeadLine: 0, Text: "old2"},
				{Prefix: '-', BaseLine: 7, HeadLine: 0, Text: "old3"},
				{Prefix: '+', BaseLine: 0, HeadLine: 5, Text: "new1"},
				{Prefix: '+', BaseLine: 0, HeadLine: 6, Text: "new2"},
				{Prefix: '+', BaseLine: 0, HeadLine: 7, Text: "new3"},
			},
		}},
	}}
	r := makeReview(Comment{
		Anchor: Anchor{
			AnchorID:  "overlap-range",
			Kind:      KindRange,
			Path:      "src/a.go",
			Side:      SideHead,
			Blob:      "newblob",
			LineStart: 5,
			LineEnd:   7,
		},
		State: StateOpen,
	})

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)

	out := buf.String()
	if !strings.Contains(out, "overlap-range") {
		t.Errorf("warning should name the anchor_id even when HeadLines overlap, got %q", out)
	}
	if !strings.Contains(out, "legacy head-side anchor") {
		t.Errorf("warning should mention 'legacy head-side anchor', got %q", out)
	}
}

// TestValidate_StillWorksWithoutWriter pins the original Validate signature:
// no behavior change for callers that don't care about warnings.
func TestValidate_StillWorksWithoutWriter(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{{
		Status:   diffmodel.StatusModified,
		PrePath:  "src/a.go",
		PostPath: "src/a.go",
		BlobBase: "old", BlobHead: "new",
	}}
	r := makeReview(Comment{
		Anchor: Anchor{Kind: KindLine, Path: "src/a.go", Side: SideHead, Blob: "new", Line: 5},
		State:  StateStale,
	})
	Validate(&r, files)
	if r.Comments[0].State != StateOpen {
		t.Errorf("Validate must keep its original behavior: %q", r.Comments[0].State)
	}
}
