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
