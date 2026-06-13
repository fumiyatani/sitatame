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

// TestValidateWithWarnings_LegacyHeadAnchor pins issue #36/#19's backward
// compatibility path: a draft saved before the Side-derivation fix may carry a
// Comment with Side=head AND a line number whose value only exists on the base
// side (because the buggy openCommentModal stored BaseLine under Side=head).
// The new validator must emit a one-line warning per offending anchor so the
// user notices before re-saving — silent fixing would mask the data corruption.
func TestValidateWithWarnings_LegacyHeadAnchor(t *testing.T) {
	t.Parallel()
	// File where line 5 only exists on the base side (e.g. a `-` row).
	files := []diffmodel.File{{
		Status:   diffmodel.StatusModified,
		PrePath:  "src/a.go",
		PostPath: "src/a.go",
		BlobBase: "old", BlobHead: "new",
	}}
	r := makeReview(Comment{
		Anchor: Anchor{
			AnchorID: "legacy-1",
			Kind:     KindLine,
			Path:     "src/a.go",
			Side:     SideHead, // ← legacy bug: head + base-only number
			Blob:     "old",    // and it points at the base blob
			Line:     5,
		},
		State: StateOpen,
	})

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)

	out := buf.String()
	if !strings.Contains(out, "legacy-1") {
		t.Errorf("warning should name the anchor_id, got %q", out)
	}
	if !strings.Contains(out, "legacy anchor") {
		t.Errorf("warning should mention 'legacy anchor', got %q", out)
	}
}

// TestValidateWithWarnings_NoFalsePositives makes sure healthy anchors do not
// trigger the legacy warning. A Side=head anchor with a matching head blob is
// the canonical case and must produce no stderr noise.
func TestValidateWithWarnings_NoFalsePositives(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{{
		Status:   diffmodel.StatusModified,
		PrePath:  "src/a.go",
		PostPath: "src/a.go",
		BlobBase: "old", BlobHead: "new",
	}}
	r := makeReview(
		Comment{
			Anchor: Anchor{AnchorID: "ok-1", Kind: KindLine, Path: "src/a.go", Side: SideHead, Blob: "new", Line: 5},
			State:  StateOpen,
		},
		Comment{
			Anchor: Anchor{AnchorID: "ok-2", Kind: KindLine, Path: "src/a.go", Side: SideBase, Blob: "old", Line: 3},
			State:  StateOpen,
		},
		Comment{
			Anchor: Anchor{AnchorID: "ok-3", Kind: KindFile, Path: "src/a.go", Side: SideHead, Blob: "new"},
			State:  StateOpen,
		},
	)

	var buf bytes.Buffer
	ValidateWithWarnings(&r, files, &buf)
	if out := buf.String(); out != "" {
		t.Errorf("expected no warnings, got %q", out)
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
