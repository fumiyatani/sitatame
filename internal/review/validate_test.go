package review

import (
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
