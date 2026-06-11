package tui

import (
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// The five seed scenarios. Each is a regression case for a previously-shipped
// fix. Adding new scenarios is documented in README.md (search "Adding a
// scenario") and in the GoDoc on tui.Scenario.

// TestScenario_MouseWheelHelpGuard pins PR #40's help-overlay wheel guard:
// while the help modal is open, wheel events must not silently scroll the
// diff behind it. The five wheel-down sends in a row would have moved Top
// well past zero without the guard.
func TestScenario_MouseWheelHelpGuard(t *testing.T) {
	t.Parallel()
	runScenario(t, Scenario{
		Name:  "mouse_wheel_help_guard",
		Files: []diffmodel.File{scenarioBigFile()},
		Steps: []Step{
			{SendKey: "?"},
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			{
				SendMouse: &MouseEvent{Button: "wheel_down"},
				Expect: Expectation{
					Top:    intPtr(0),
					Cursor: intPtr(0),
				},
			},
		},
	})
}

// TestScenario_ResolveToggleUndo pins PR #41's sticky-anchor behaviour:
// `[open A, resolved B]` on the same row must resolve A on first `x` and
// reopen A (not flip B) on a follow-up `x` without moving the cursor.
//
// The mid-step check is done via ViewContains against the status bar — the
// runner can't peek at m.Review.Comments mid-program because teatest doesn't
// expose live model state. The final state is verified on the final model
// after Quit.
func TestScenario_ResolveToggleUndo(t *testing.T) {
	t.Parallel()
	f, openComment := scenarioFileWithComment("a.go", 2, "a-open")
	resolvedComment := review.Comment{
		Anchor: review.Anchor{
			AnchorID: "a-resolved",
			Kind:     review.KindLine,
			Path:     "a.go",
			Side:     review.SideHead,
			Line:     2,
			Blob:     "b2",
		},
		State: review.StateResolved,
		Body:  "already fixed",
	}
	r := review.Review{Comments: []review.Comment{openComment, resolvedComment}}

	runScenario(t, Scenario{
		Name:          "resolve_toggle_undo",
		Files:         []diffmodel.File{f},
		InitialReview: r,
		Steps: []Step{
			// Walk to HeadLine=2 (row 3: file header, hunk header, line 1, line 2).
			{SendKey: "j"},
			{SendKey: "j"},
			{SendKey: "j"},
			{
				// First `x`: open-biased default resolves A. statusMsg
				// echoes the anchor_id, which we can observe via the view.
				SendKey: "x",
				Expect: Expectation{
					ViewContains: []string{"resolved: a-open"},
				},
			},
			{
				// Second `x` on the same row: must reopen A, NOT flip B.
				// Status bar should now read "reopened: a-open".
				SendKey: "x",
				Expect: Expectation{
					ViewContains: []string{"reopened: a-open"},
					Comments: []*review.Comment{
						{Anchor: review.Anchor{AnchorID: "a-open"}, State: review.StateOpen},
						{Anchor: review.Anchor{AnchorID: "a-resolved"}, State: review.StateResolved},
					},
					CommentsLen: intPtr(2),
				},
			},
		},
	})
}

// TestScenario_SplitTabRoundtrip pins the split-mode preview-only guard
// (added in the split-layout PRs): Tab toggles into split, `c` in split is
// rejected with the previewOnlyMsg, Tab returns to unified, and `c` in
// unified now opens the comment modal as normal.
func TestScenario_SplitTabRoundtrip(t *testing.T) {
	t.Parallel()
	runScenario(t, Scenario{
		Name:  "split_tab_roundtrip",
		Files: []diffmodel.File{scenarioBigFile()},
		Steps: []Step{
			// Move onto a content row so `c` in unified can anchor.
			{SendKey: "j"},
			{SendKey: "j"},
			// Enter split.
			{
				SendKey: "tab",
				Expect:  Expectation{ViewContains: []string{"split: preview"}},
			},
			// `c` in split must be rejected with the preview-only message.
			// The full previewOnlyMsg is too long for the 80-col status bar,
			// so we match its head — enough to prove the guard fired.
			{
				SendKey: "c",
				Expect: Expectation{
					ViewContains: []string{"split is preview-only"},
				},
			},
			// Back to unified.
			{
				SendKey: "tab",
				Expect:  Expectation{ViewContains: []string{"[unified]"}},
			},
			// `c` in unified opens the modal — modalView replaces the diff
			// so we look for the modal footer (always present regardless of
			// header truncation).
			{
				SendKey: "c",
				Expect: Expectation{
					ViewContains: []string{"Ctrl+S save"},
					Layout:       layoutPtr(LayoutUnified),
				},
			},
		},
	})
}

// TestScenario_RangeCommentFlow exercises the full range-comment happy path:
// start a selection with `r`, extend it with two `j` presses, open the modal
// with `c`, type a body, and confirm with Ctrl+S. The resulting comment must
// be Kind=Range with LineStart..LineEnd covering the selected rows.
//
// Uses a numbered context-only file so the line numbers are predictable.
func TestScenario_RangeCommentFlow(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 5)

	runScenario(t, Scenario{
		Name:  "range_comment_flow",
		Files: []diffmodel.File{f},
		Steps: []Step{
			// Walk to first content line (row 2: file header, hunk header, line 1).
			{SendKey: "j"},
			{SendKey: "j"},
			{SendKey: "r"},
			{SendKey: "j"}, // extend selection to line 2
			{SendKey: "j"}, // extend selection to line 3
			{
				SendKey: "c",
				Expect:  Expectation{ViewContains: []string{"range comment: a.go"}},
			},
			// Type a single-character body so the textarea has content.
			{SendKey: "h"},
			{SendKey: "i"},
			{
				SendKey: "ctrl+s",
				Expect: Expectation{
					CommentsLen: intPtr(1),
					Comments: []*review.Comment{
						{
							Anchor: review.Anchor{
								Kind:      review.KindRange,
								Path:      "a.go",
								LineStart: 1,
								LineEnd:   3,
							},
							State: review.StateOpen,
							Body:  "hi",
						},
					},
				},
			},
		},
	})
}

// TestScenario_MouseWheelExtendsSelection pins PR #40 round 4's
// extendSelection() call in scrollViewportBy. Starting a range on a content
// row and then wheel-scrolling past the cursor must keep Selection.Extent
// pinned to the (clamped) cursor. Confirming a comment then yields a Range
// whose LineEnd reflects the wheel-driven extent, not the pre-wheel anchor.
func TestScenario_MouseWheelExtendsSelection(t *testing.T) {
	t.Parallel()
	// Use a small content file so we can predict line numbers, but big enough
	// that a wheel tick moves the cursor by mouseWheelStep rows. numberedFile
	// with 20 content lines gives plenty of headroom past the initial
	// viewport (24 - chrome) so the wheel actually advances.
	f := numberedFile("a.go", "a.go", "b1", "b2", 20)

	runScenario(t, Scenario{
		Name:       "mouse_wheel_extends_selection",
		Files:      []diffmodel.File{f},
		WindowSize: [2]int{60, 12}, // tight viewport so the wheel clamp kicks in
		Steps: []Step{
			// Walk to first content line.
			{SendKey: "j"},
			{SendKey: "j"},
			// Start the range at line 1.
			{
				SendKey: "r",
				Expect:  Expectation{ViewContains: []string{"RANGE"}},
			},
			// Wheel-scroll three times: each tick advances the cursor by
			// mouseWheelStep (clamped to viewport bottom) and the
			// selection extent must follow.
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			// Open the modal at the wheel-extended cursor position.
			{SendKey: "c"},
			{SendKey: "x"},
			{
				SendKey: "ctrl+s",
				Expect: Expectation{
					CommentsLen: intPtr(1),
					// LineStart pinned to line 1 (selection anchor row),
					// LineEnd must be > 1 to prove the wheel extended
					// the selection. The exact LineEnd depends on
					// viewport/chrome math which we don't want to
					// over-pin here — the strict ">" check lives in
					// TestScenario_MouseWheelExtendsSelection_LineEnd.
					Comments: []*review.Comment{{Anchor: review.Anchor{Kind: review.KindRange, LineStart: 1}}},
				},
			},
		},
	})
}

// TestScenario_MouseWheelExtendsSelection_LineEnd is the sibling assertion
// that the previous scenario can't express through the partial Comments
// matcher: LineEnd must be strictly greater than LineStart. Kept as a small
// follow-up table-driven check so the scenario harness itself stays clean.
//
// This is intentionally a Go-level test rather than a Scenario because the
// Expectation DSL is "field-equals" only — adding "field >= X" comparators
// would complicate the surface for a single edge case. The driver still uses
// runScenario; we just inspect the final model on top.
func TestScenario_MouseWheelExtendsSelection_LineEnd(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 20)
	sc := Scenario{
		Name:       "mouse_wheel_extends_selection_lineend",
		Files:      []diffmodel.File{f},
		WindowSize: [2]int{60, 12},
		Steps: []Step{
			{SendKey: "j"},
			{SendKey: "j"},
			{SendKey: "r"},
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			{SendMouse: &MouseEvent{Button: "wheel_down"}},
			{SendKey: "c"},
			{SendKey: "x"},
			{SendKey: "ctrl+s"},
		},
	}
	// Inline runner: we need the final model so we can do a > comparison.
	m := New(sc.Files, sc.InitialReview)
	m = setSize(m, sc.WindowSize[0], sc.WindowSize[1])
	for _, st := range sc.Steps {
		switch {
		case st.SendKey != "":
			msg, err := keyMsgFromSpec(st.SendKey)
			if err != nil {
				t.Fatal(err)
			}
			upd, _ := m.Update(msg)
			m = upd.(Model)
		case st.SendMouse != nil:
			msg, err := mouseMsgFromSpec(*st.SendMouse)
			if err != nil {
				t.Fatal(err)
			}
			upd, _ := m.Update(msg)
			m = upd.(Model)
		}
	}
	if got := len(m.Review.Comments); got != 1 {
		t.Fatalf("len(Comments) = %d, want 1", got)
	}
	c := m.Review.Comments[0]
	if c.Kind != review.KindRange {
		t.Errorf("Kind = %q, want %q", c.Kind, review.KindRange)
	}
	if c.LineStart < 1 {
		t.Errorf("LineStart = %d, want >= 1", c.LineStart)
	}
	if c.LineEnd <= c.LineStart {
		t.Errorf("LineEnd = %d, want > LineStart=%d (wheel did not extend selection)",
			c.LineEnd, c.LineStart)
	}
}
