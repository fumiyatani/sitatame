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

// TestResolveToggle_UndoUsesSameAnchor pins the P2 fix from PR #41 round 3:
// when a row hosts `[open A, resolved B]`, the first `x` must resolve A
// (open-biased default) and a follow-up `x` *without* moving the cursor must
// reopen A — not flip B, which the previous "last resolved" fallback did.
// Silent mutation of B would corrupt an unrelated comment with no UI feedback.
func TestResolveToggle_UndoUsesSameAnchor(t *testing.T) {
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

	// First press: open-biased default resolves A.
	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != review.StateResolved {
		t.Fatalf("after first x: A state = %q, want %q", got, review.StateResolved)
	}
	if got := m.Review.Comments[1].State; got != review.StateResolved {
		t.Fatalf("after first x: B state = %q, want %q (must not move)", got, review.StateResolved)
	}

	// Second press without moving cursor: must reopen A, not flip B.
	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != review.StateOpen {
		t.Errorf("after second x: A state = %q, want %q (undo target)", got, review.StateOpen)
	}
	if got := m.Review.Comments[1].State; got != review.StateResolved {
		t.Errorf("after second x: B state = %q, want %q (must not be silently mutated)", got, review.StateResolved)
	}
	if want := "reopened: a-open"; m.statusMsg != want {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, want)
	}
}

// TestResolveToggle_LastAnchorClearedOnCursorMove verifies the sticky anchor
// is bound to the current row: once the user moves away (and back), `x`
// resets to the open-biased default rather than re-using the previous
// row's anchor. Without this clear, a row visited later with a different
// `[open, resolved]` mix would target the wrong comment.
func TestResolveToggle_LastAnchorClearedOnCursorMove(t *testing.T) {
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
	row := m.Cursor()

	// First x: A becomes resolved. Then move away and come back.
	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[0].State; got != review.StateResolved {
		t.Fatalf("setup: A state = %q, want %q", got, review.StateResolved)
	}
	m, _ = applyKey(m, "j")
	if m.Cursor() == row {
		t.Fatalf("cursor did not move on j")
	}
	if m.lastToggledAnchor != "" {
		t.Errorf("lastToggledAnchor not cleared on cursor move: %q", m.lastToggledAnchor)
	}
	m, _ = applyKey(m, "k")
	if m.Cursor() != row {
		t.Fatalf("cursor did not return to original row (%d != %d)", m.Cursor(), row)
	}

	// Both comments are now resolved. With the sticky anchor cleared,
	// the open-biased default has no open candidate and falls back to
	// reopening the *last* resolved (a-resolved by slice order), proving
	// we are not re-targeting a-open from the prior toggle.
	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[1].State; got != review.StateOpen {
		t.Errorf("expected a-resolved to be reopened (default path), got state = %q", got)
	}
	if got := m.Review.Comments[0].State; got != review.StateResolved {
		t.Errorf("a-open should stay resolved after move-away undo path: state = %q", got)
	}
	if want := "reopened: a-resolved"; m.statusMsg != want {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, want)
	}
}

// TestResolveToggle_StickyClearedByTabAlone pins the cheapest failure mode of
// the P2: a single Tab press (no split-side navigation, no return trip) must
// already drop the sticky resolve anchor. Without this, a user that toggles
// in unified, peeks at split, and Tabs back would silently re-toggle the
// same comment on the next `x`.
func TestResolveToggle_StickyClearedByTabAlone(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{AnchorID: "a-open", Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2"},
		State:  review.StateOpen,
		Body:   "needs work",
	}}}
	m := setSize(New([]diffmodel.File{f}, r), 80, 24)
	m, _ = applyKey(m, "j") // hunk header
	m, _ = applyKey(m, "j") // line 1
	m, _ = applyKey(m, "j") // line 2

	m, _ = applyKey(m, "x")
	if m.lastToggledAnchor != "a-open" {
		t.Fatalf("setup: sticky not set after first toggle: %q", m.lastToggledAnchor)
	}

	// First Tab: enter split mode.
	m = sendNamedKey(m, tea.KeyTab)
	if m.layout != LayoutSplit {
		t.Fatalf("did not enter split layout")
	}
	if m.lastToggledAnchor != "" {
		t.Errorf("Tab into split did not clear sticky anchor: %q", m.lastToggledAnchor)
	}

	// Toggle once back in unified to re-arm sticky, then Tab in the
	// opposite direction (unified→split was already covered above; this
	// verifies the split→unified path also clears).
	m = sendNamedKey(m, tea.KeyTab) // back to unified
	m, _ = applyKey(m, "x")         // reopens a-open (sticky points at a-open)
	if m.lastToggledAnchor != "a-open" {
		t.Fatalf("setup: sticky not re-set: %q", m.lastToggledAnchor)
	}
	m = sendNamedKey(m, tea.KeyTab) // unified → split
	m = sendNamedKey(m, tea.KeyTab) // split → unified
	if m.lastToggledAnchor != "" {
		t.Errorf("Tab round-trip did not clear sticky anchor: %q", m.lastToggledAnchor)
	}
}

// TestResolveToggle_StickyClearedAfterSplitNavigation pins the P2 reported on
// PR #41 round 4: a range comment toggled in unified must not bleed into
// the next `x` after the cursor has moved inside split. Range comments cover
// multiple rows, so an unmaintained sticky anchor would silently re-toggle
// the range on a follow-up `x` even though the user has navigated to a
// different line that hosts an unrelated open comment.
func TestResolveToggle_StickyClearedAfterSplitNavigation(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 4)
	r := review.Review{Comments: []review.Comment{
		{
			Anchor: review.Anchor{
				AnchorID: "a-range", Kind: review.KindRange,
				Path: "a.go", Side: review.SideHead,
				LineStart: 1, LineEnd: 2, Blob: "b2",
			},
			State: review.StateOpen,
			Body:  "range covers lines 1..2",
		},
		{
			Anchor: review.Anchor{
				AnchorID: "a-line4", Kind: review.KindLine,
				Path: "a.go", Side: review.SideHead,
				Line: 4, Blob: "b2",
			},
			State: review.StateOpen,
			Body:  "single-line comment on line 4",
		},
	}}
	m := setSize(New([]diffmodel.File{f}, r), 80, 24)

	// Park on row 2 (HeadLine=1) which the range anchor covers.
	m, _ = applyKey(m, "j") // hunk header
	m, _ = applyKey(m, "j") // line 1
	hits := m.Overlay()[m.Cursor()]
	if len(hits) == 0 || m.Review.Comments[hits[0]].AnchorID != "a-range" {
		t.Fatalf("setup: expected a-range overlay at line 1 row, got %+v", hits)
	}

	// First `x` resolves the range and arms the sticky anchor.
	m, _ = applyKey(m, "x")
	if m.Review.Comments[0].State != review.StateResolved {
		t.Fatalf("setup: range not resolved on first x: %q", m.Review.Comments[0].State)
	}
	if m.lastToggledAnchor != "a-range" {
		t.Fatalf("setup: sticky not set: %q", m.lastToggledAnchor)
	}

	// Tab into split, walk to the line 4 row, Tab back.
	m = sendNamedKey(m, tea.KeyTab)
	if m.layout != LayoutSplit {
		t.Fatalf("did not enter split layout")
	}
	// Move down 3 rows: line 1 → line 2 → line 3 → line 4. (split rows for
	// a context-only file mirror the unified row order one-to-one.)
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m = sendNamedKey(m, tea.KeyTab) // back to unified

	// We should now be on the row hosting a-line4.
	hits = m.Overlay()[m.Cursor()]
	if len(hits) == 0 {
		t.Fatalf("after split round-trip, expected overlay at cursor row %d, got none", m.Cursor())
	}
	gotAnchor := m.Review.Comments[hits[0]].AnchorID
	if gotAnchor != "a-line4" {
		t.Fatalf("cursor landed on %q instead of a-line4 (cursor=%d) — test setup needs adjustment", gotAnchor, m.Cursor())
	}

	// Critical assertion: `x` must resolve a-line4 via the open-biased
	// default, not silently re-toggle a-range via a stale sticky anchor.
	m, _ = applyKey(m, "x")
	if got := m.Review.Comments[1].State; got != review.StateResolved {
		t.Errorf("a-line4 should have been resolved by post-split x, got %q", got)
	}
	if got := m.Review.Comments[0].State; got != review.StateResolved {
		t.Errorf("a-range must remain resolved (sticky leak would have flipped it): got %q", got)
	}
	if want := "resolved: a-line4"; m.statusMsg != want {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, want)
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
