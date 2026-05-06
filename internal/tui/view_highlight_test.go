package tui

import (
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// view_highlight_test verifies that the comment-row coloring lands on the
// expected columns. Uses raw m.View() (ANSI preserved) — goldenSnapshot
// strips ANSI and would silently pass even if coloring were removed.

func TestMainView_HighlightOn_KindLineComment(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2},
		State:  review.StateOpen,
		Body:   "look here",
	}}}
	m := setSize(New([]diffmodel.File{f}, r), 60, 12)

	out := m.View()
	if !strings.Contains(out, commentColorFG) {
		t.Fatalf("expected highlight SGR %q in view, got:\n%s", commentColorFG, out)
	}
}

func TestMainView_HighlightOn_KindRangeAllAnchoredRows(t *testing.T) {
	t.Parallel()
	// KindRange should color every row inside [LineStart, LineEnd]. A single
	// strings.Contains check would pass even if only the first row picked up
	// the SGR — count the colored row lines instead so a partial-coverage
	// regression breaks the test.
	f := numberedFile("a.go", "a.go", "b1", "b2", 5)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{
			Kind: review.KindRange, Path: "a.go", Side: review.SideHead,
			LineStart: 2, LineEnd: 4,
		},
		State: review.StateOpen,
		Body:  "spans 3 lines",
	}}}
	m := setSize(New([]diffmodel.File{f}, r), 60, 16)

	colored := 0
	for _, ln := range strings.Split(m.View(), "\n") {
		if strings.Contains(ln, commentColorFG) {
			colored++
		}
	}
	if colored != 3 {
		t.Fatalf("KindRange [2..4] should color exactly 3 rows, got %d, view:\n%s", colored, m.View())
	}
}

func TestMainView_HighlightOff_NoComments(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	m := setSize(New([]diffmodel.File{f}, review.Review{}), 60, 12)

	if out := m.View(); strings.Contains(out, commentColorFG) {
		t.Fatalf("did not expect highlight SGR in commentless view, got:\n%s", out)
	}
}

func TestMainView_HighlightOff_KindReviewOnly(t *testing.T) {
	t.Parallel()
	// KindReview is anchorless; buildOverlay skips it, so no row should color.
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{Kind: review.KindReview},
		State:  review.StateOpen,
		Body:   "overall",
	}}}
	m := setSize(New([]diffmodel.File{f}, r), 60, 12)

	if out := m.View(); strings.Contains(out, commentColorFG) {
		t.Fatalf("KindReview should not color any row, got:\n%s", out)
	}
}

func TestMainView_HighlightOn_KindFileFallback(t *testing.T) {
	t.Parallel()
	// File-comment anchors to the file-header row, where lineNumberGutter
	// returns blanks. Verify the fallback path applies coloring to marker /
	// body so the row is still visibly highlighted.
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{Kind: review.KindFile, Path: "a.go"},
		State:  review.StateOpen,
		Body:   "whole-file note",
	}}}
	m := setSize(New([]diffmodel.File{f}, r), 60, 12)

	out := m.View()
	if !strings.Contains(out, commentColorFG) {
		t.Fatalf("expected fallback highlight for KindFile, got:\n%s", out)
	}
	// The file-header line itself should carry the coloring (look for the
	// path text wrapped or co-located with the SGR).
	headerLine := ""
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(stripANSI(ln), "a.go") && strings.Contains(ln, commentColorFG) {
			headerLine = ln
			break
		}
	}
	if headerLine == "" {
		t.Fatalf("KindFile highlight did not land on the file-header row, got:\n%s", out)
	}
}

func TestMainView_HighlightDoesNotColorCursorMarker(t *testing.T) {
	t.Parallel()
	// Cursor is on the file-header row at startup. If we accidentally colored
	// the cursor gutter, the SGR would appear before "> " in the rendered row.
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{Kind: review.KindFile, Path: "a.go"},
		State:  review.StateOpen,
		Body:   "whole-file note",
	}}}
	m := setSize(New([]diffmodel.File{f}, r), 60, 12)

	out := m.View()
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, cursorMarker) {
			return // cursor row starts with literal "> " — coloring stayed out of the cursor gutter.
		}
	}
	t.Fatalf("no row started with bare cursorMarker %q (cursor gutter may have been colored), got:\n%s", cursorMarker, out)
}

// Regression: confirmModal must rebuild m.overlay so a comment added during
// the live TUI session immediately renders its marker / highlight color.
// Previously buildOverlay only ran in New(), and adding a comment in-session
// left overlay stale until restart.
func TestConfirmModal_RebuildsOverlayForLiveHighlight(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := setSize(New(files, review.Review{}), 60, 12)

	// Pre-condition: no highlight before any comment exists.
	if strings.Contains(m.View(), commentColorFG) {
		t.Fatalf("unexpected highlight before any comment was added")
	}

	// Move to first content line and open the line-comment modal.
	m, _ = applyKey(m, "j") // hunk header
	m, _ = applyKey(m, "j") // first content line
	m, _ = applyKey(m, "c")
	if m.Modal() == nil {
		t.Fatalf("c on content line should open the line-comment modal")
	}
	m = typeBody(m, "live note")
	m = modalSendCtrlS(m)

	if !strings.Contains(m.View(), commentColorFG) {
		t.Fatalf("expected highlight to appear right after confirmModal, got:\n%s", m.View())
	}
}

func TestMainView_HighlightSameColor_OpenAndStaleMix(t *testing.T) {
	t.Parallel()
	// Two comments on the same line, one open + one stale. The row coloring
	// must use the single "has-comment" color regardless of state mix —
	// open/stale distinction is left to the existing overlay marker.
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{
		{Anchor: review.Anchor{Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2}, State: review.StateOpen, Body: "x"},
		{Anchor: review.Anchor{Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2}, State: review.StateStale, Body: "y"},
	}}
	m := setSize(New([]diffmodel.File{f}, r), 60, 12)

	out := m.View()
	// commentColorFG should appear; no other variant SGR for "stale color"
	// should leak in (we only define one color constant).
	if !strings.Contains(out, commentColorFG) {
		t.Fatalf("expected highlight on mixed-state row, got:\n%s", out)
	}
}
