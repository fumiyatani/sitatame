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
