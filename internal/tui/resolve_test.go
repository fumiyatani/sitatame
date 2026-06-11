package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// TestResolveToggle_FlipsAnchoredComment exercises the happy path: cursor is
// parked on a line that owns an open comment, `x` toggles to resolved, `x`
// again toggles back to open.
func TestResolveToggle_FlipsAnchoredComment(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
		State:  review.StateOpen,
		Body:   "needs work",
	}}}
	m := New([]diffmodel.File{f}, r)

	// Walk to the row that overlays comment 0. From the overlay test we know
	// HeadLine=2 → row index 3 (file header, hunk header, line 1, line 2).
	m, _ = applyKey(m, "j") // hunk header
	m, _ = applyKey(m, "j") // line 1
	m, _ = applyKey(m, "j") // line 2

	if got := m.Cursor(); got != 3 {
		t.Fatalf("cursor = %d, want 3 (HeadLine=2 row)", got)
	}
	if hits := m.Overlay()[m.Cursor()]; len(hits) == 0 {
		t.Fatalf("overlay empty at cursor row — test setup is wrong: %+v", m.Overlay())
	}

	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != review.StateResolved {
		t.Fatalf("first toggle: state = %q, want %q", got, review.StateResolved)
	}

	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != review.StateOpen {
		t.Fatalf("second toggle: state = %q, want %q", got, review.StateOpen)
	}
}

// TestResolveToggle_NoopOnRowWithoutComment guarantees that `x` on an
// unanchored row does nothing: no panic, no state mutation, no spurious
// comment creation. Important because a stray `x` while navigating must not
// alter the review.
func TestResolveToggle_NoopOnRowWithoutComment(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
		State:  review.StateOpen,
		Body:   "needs work",
	}}}
	m := New([]diffmodel.File{f}, r)

	// Stay on row 0 (file header) — no overlay there.
	if hits := m.Overlay()[m.Cursor()]; len(hits) != 0 {
		t.Fatalf("file-header row should not have overlay: %+v", hits)
	}

	before := m.Review.Comments[0].State
	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != before {
		t.Errorf("state changed despite no overlay at cursor: %q → %q", before, got)
	}
	if len(m.Review.Comments) != 1 {
		t.Errorf("comment count changed: got %d, want 1", len(m.Review.Comments))
	}
}

// TestResolveToggle_StaleIsIgnored guarantees that comments in StateStale
// are skipped: `x` on a row that *only* hosts a stale comment must not flip
// it. Stale signals "anchor is broken"; resolving silently would hide the
// follow-up the original reviewer expects to handle by hand.
func TestResolveToggle_StaleIsIgnored(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{AnchorID: "a-stale", Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
		State:  review.StateStale,
		Body:   "drifted",
	}}}
	m := New([]diffmodel.File{f}, r)

	// Park on the row that overlays comment 0 (HeadLine=2 → row 3).
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	if hits := m.Overlay()[m.Cursor()]; len(hits) == 0 {
		t.Fatalf("overlay empty at cursor row — test setup is wrong: %+v", m.Overlay())
	}

	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != review.StateStale {
		t.Errorf("stale comment was mutated by x: state = %q, want %q", got, review.StateStale)
	}
	if m.statusMsg != "" {
		t.Errorf("statusMsg should be empty when no toggle happened, got %q", m.statusMsg)
	}
}

// TestResolveToggle_MultipleComments_PicksOpenOverResolved covers the [open,
// resolved] ordering: when both states share the row, `x` must resolve the
// open one rather than reopening the resolved one (otherwise the visible
// "needs work" item is left untouched). statusMsg should echo the resolved
// anchor_id.
func TestResolveToggle_MultipleComments_PicksOpenOverResolved(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{
		{
			Anchor: review.Anchor{AnchorID: "a-open", Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
			State:  review.StateOpen,
			Body:   "needs work",
		},
		{
			Anchor: review.Anchor{AnchorID: "a-resolved", Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
			State:  review.StateResolved,
			Body:   "already fixed",
		},
	}}
	m := New([]diffmodel.File{f}, r)
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	if hits := m.Overlay()[m.Cursor()]; len(hits) != 2 {
		t.Fatalf("expected 2 overlays at cursor, got %d", len(hits))
	}

	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != review.StateResolved {
		t.Errorf("open comment should be resolved: state = %q, want %q", got, review.StateResolved)
	}
	if got := m.Review.Comments[1].State; got != review.StateResolved {
		t.Errorf("already-resolved comment must not change: state = %q, want %q", got, review.StateResolved)
	}
	if want := "resolved: a-open"; m.statusMsg != want {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, want)
	}
}

// TestResolveToggle_MultipleComments_ReversedOrderStillPicksOpen flips the
// slice order to [resolved, open]. Selection must remain open-biased, not
// "last index". This is the regression case the previous "idxs[len-1]"
// implementation failed on.
func TestResolveToggle_MultipleComments_ReversedOrderStillPicksOpen(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{
		{
			Anchor: review.Anchor{AnchorID: "a-resolved", Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
			State:  review.StateResolved,
			Body:   "already fixed",
		},
		{
			Anchor: review.Anchor{AnchorID: "a-open", Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
			State:  review.StateOpen,
			Body:   "needs work",
		},
	}}
	m := New([]diffmodel.File{f}, r)
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	if hits := m.Overlay()[m.Cursor()]; len(hits) != 2 {
		t.Fatalf("expected 2 overlays at cursor, got %d", len(hits))
	}

	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[1].State; got != review.StateResolved {
		t.Errorf("open comment (index 1) should be resolved: state = %q, want %q", got, review.StateResolved)
	}
	if got := m.Review.Comments[0].State; got != review.StateResolved {
		t.Errorf("already-resolved comment (index 0) must not change: state = %q, want %q", got, review.StateResolved)
	}
	if want := "resolved: a-open"; m.statusMsg != want {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, want)
	}
}

// TestResolveToggle_NoopDuringModal ensures `x` is consumed by the modal's
// textarea (becomes literal input) and never reaches toggleResolvedAtCursor.
// State stays unchanged and the modal remains open.
func TestResolveToggle_NoopDuringModal(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{AnchorID: "a-open", Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
		State:  review.StateOpen,
		Body:   "needs work",
	}}}
	m := New([]diffmodel.File{f}, r)

	m, _ = applyKey(m, "R") // opens the review-level modal
	if m.modal == nil {
		t.Fatalf("review modal did not open")
	}

	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != review.StateOpen {
		t.Errorf("modal-mode `x` must not mutate state: %q", got)
	}
	if m.modal == nil {
		t.Errorf("modal was closed by `x` — should still be open")
	}
}

// TestResolveToggle_SplitModeShowsHint mirrors the c/r/R guards: x is
// preview-only in split layout and should surface the same hint without
// touching review state.
func TestResolveToggle_SplitModeShowsHint(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
		State:  review.StateOpen,
	}}}
	m := setSize(New([]diffmodel.File{f}, r), 80, 24)
	m = sendNamedKey(m, tea.KeyTab) // enter split
	if m.layout != LayoutSplit {
		t.Fatalf("did not enter split layout")
	}

	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != review.StateOpen {
		t.Errorf("split mode `x` must not mutate state: %q", got)
	}
	if m.statusMsg != previewOnlyMsg {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, previewOnlyMsg)
	}
}
