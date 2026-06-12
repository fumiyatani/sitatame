package tui

import (
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// hintTestModel keeps the test inputs minimal: hintLine only reads m.width,
// m.selection, m.layout, m.cursor, and m.overlay. Building a full Model via
// New() would pull in row construction we don't need here.
func hintTestModel(width int) Model {
	return Model{width: width, overlay: map[int][]int{}}
}

func TestHint_Normal(t *testing.T) {
	t.Parallel()
	m := hintTestModel(80)
	got := hintLine(m)
	want := "j/k move · n/p file · c cmt · ? help · q quit"
	if !strings.HasPrefix(got, want) {
		t.Errorf("hint = %q, want prefix %q", got, want)
	}
	if strings.Contains(got, modeTagRange) || strings.Contains(got, modeTagHasComment) {
		t.Errorf("normal hint must not carry a mode tag, got %q", got)
	}
}

func TestHint_Selection(t *testing.T) {
	t.Parallel()
	m := hintTestModel(80)
	m.selection = &Selection{Anchor: 1, Extent: 2}
	got := hintLine(m)
	if !strings.Contains(got, "j/k extend") || !strings.Contains(got, "c COMMENT") {
		t.Errorf("selection hint missing rich left half: %q", got)
	}
	if !strings.Contains(got, modeTagRange) {
		t.Errorf("selection hint missing %q tag: %q", modeTagRange, got)
	}
}

func TestHint_SelectionWinsOverComment(t *testing.T) {
	t.Parallel()
	// Selection mode must dominate even when the cursor row also carries a
	// comment — otherwise a range-comment-in-progress would silently lose its
	// `-- RANGE --` indicator the instant the cursor extends over an
	// already-commented row.
	m := hintTestModel(80)
	m.cursor = 0
	m.overlay[0] = []int{0}
	m.selection = &Selection{Anchor: 0, Extent: 0}
	got := hintLine(m)
	if !strings.Contains(got, modeTagRange) {
		t.Errorf("selection should win when both apply, got %q", got)
	}
	if strings.Contains(got, "has comment") || strings.Contains(got, "has cmt") {
		t.Errorf("comment tag must not appear while selection is active: %q", got)
	}
}

func TestHint_HasComment(t *testing.T) {
	t.Parallel()
	m := hintTestModel(80)
	m.cursor = 3
	m.overlay[3] = []int{0}
	got := hintLine(m)
	if !strings.Contains(got, "x RESOLVE") || !strings.Contains(got, "c add") {
		t.Errorf("has-comment hint missing rich left half: %q", got)
	}
	if !strings.Contains(got, modeTagHasComment) {
		t.Errorf("has-comment hint missing %q tag: %q", modeTagHasComment, got)
	}
}

func TestHint_HasCommentIgnoresEmptyOverlay(t *testing.T) {
	t.Parallel()
	// An empty overlay slice must not be treated as "row has comments";
	// only a non-empty slice counts. Otherwise rows that happen to be in
	// the overlay map (e.g. KindReview anchors filtered out elsewhere) would
	// flip the hint into has-comment mode and confuse the reviewer.
	m := hintTestModel(80)
	m.cursor = 3
	m.overlay[3] = nil
	got := hintLine(m)
	if strings.Contains(got, modeTagHasComment) {
		t.Errorf("empty overlay must not trigger has-comment hint, got %q", got)
	}
	if !strings.Contains(got, "c cmt") {
		t.Errorf("expected normal hint, got %q", got)
	}
}

func TestHint_Split(t *testing.T) {
	t.Parallel()
	m := hintTestModel(80)
	m.layout = LayoutSplit
	got := hintLine(m)
	if !strings.Contains(got, "Tab unified") {
		t.Errorf("split hint missing %q: %q", "Tab unified", got)
	}
	if strings.Contains(got, "q quit") {
		t.Errorf("split hint should drop `q quit`, got %q", got)
	}
}

func TestHint_SplitBeatsSelectionAndComment(t *testing.T) {
	t.Parallel()
	// model.go blocks `r` and `c` inside split, but defensive: if the model
	// somehow carried a selection while in split layout the hint must still
	// surface the split variant — the right tag would otherwise mislead the
	// reviewer about what keys do.
	m := hintTestModel(80)
	m.layout = LayoutSplit
	m.cursor = 0
	m.overlay[0] = []int{0}
	m.selection = &Selection{Anchor: 0, Extent: 0}
	got := hintLine(m)
	if strings.Contains(got, modeTagRange) || strings.Contains(got, modeTagHasComment) {
		t.Errorf("split must beat selection / has-comment tags, got %q", got)
	}
	if !strings.Contains(got, "Tab unified") {
		t.Errorf("split hint missing %q: %q", "Tab unified", got)
	}
}

func TestHint_WidthFallback(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(m *Model)
		want  string // expected substring of the short variant
		notIn string // substring expected to be absent (richer variant)
	}{
		{
			name:  "normal",
			setup: func(m *Model) {},
			want:  "j/k · n/p · c · ? · q",
			notIn: "j/k move",
		},
		{
			name:  "selection",
			setup: func(m *Model) { m.selection = &Selection{Anchor: 0, Extent: 0} },
			want:  "extend · c · esc",
			notIn: "c COMMENT",
		},
		{
			name: "has_comment",
			setup: func(m *Model) {
				m.cursor = 0
				m.overlay[0] = []int{0}
			},
			want:  "j/k · x · c",
			notIn: "x RESOLVE",
		},
		{
			name:  "split",
			setup: func(m *Model) { m.layout = LayoutSplit },
			want:  "j/k · n/p · Tab · ?",
			notIn: "n/p file",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// 20 cols is narrower than any rich variant; selectHint must
			// fall through to the compact form.
			m := hintTestModel(20)
			tc.setup(&m)
			got := hintLine(m)
			if !strings.Contains(got, tc.want) {
				t.Errorf("hint = %q, want substring %q", got, tc.want)
			}
			if strings.Contains(got, tc.notIn) {
				t.Errorf("hint = %q, must not contain %q (rich variant should have been dropped)", got, tc.notIn)
			}
		})
	}
}

func TestHint_WidthZeroPicksRichVariant(t *testing.T) {
	t.Parallel()
	// Before the first WindowSizeMsg lands, m.width is 0. selectHint treats
	// "unknown size" as a fit so the initial paint surfaces the rich form
	// (otherwise users on a brand-new terminal session would see the cramped
	// fallback for a frame).
	m := hintTestModel(0)
	got := hintLine(m)
	if !strings.Contains(got, "j/k move") || !strings.Contains(got, "c cmt") {
		t.Errorf("width=0 should pick the rich normal hint, got %q", got)
	}
}

func TestHint_RealOverlayDrivesHasComment(t *testing.T) {
	t.Parallel()
	// Integration-ish: build a full model via New() (which constructs rows
	// and overlay from review.Comment) and walk the cursor to a row that
	// carries a comment. The hint must flip without us touching m.overlay
	// directly.
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{
			Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "b2",
		},
		State: review.StateOpen,
		Body:  "needs work",
	}}}
	m := setSize(New([]diffmodel.File{f}, r), 80, 24)
	// Walk to the commented row: file header (0), hunk header (1), line 1
	// (2), line 2 (3) — that's where the open comment is anchored.
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	if !hasComment(m.overlay[m.cursor]) {
		t.Fatalf("cursor row %d has no comment; overlay = %v", m.cursor, m.overlay)
	}
	got := hintLine(m)
	if !strings.Contains(got, modeTagHasComment) {
		t.Errorf("hint = %q, want substring %q", got, modeTagHasComment)
	}
}
