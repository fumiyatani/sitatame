package tui

import (
	"strings"
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

// TestScenario_ScrollThenViewTopMarker is a regression case for the
// "view assertions read the cumulative output history" bug that the
// initial harness had. With two files in the diff, the first step
// observes file 1's "M a.go" header at the top of the viewport;
// scrolling deep into file 2 then makes that header scroll off-screen.
// bubbletea's teatest output stream still contains the bytes of every
// previous frame, so a check that doesn't slice to the latest frame
// would still see "M a.go" after the scroll and pass — or, worse,
// trip ViewNotContains["M a.go"] forever.
//
// The scenario asserts in two phases:
//   - step 1 (`j` once): a.go's header is at the top of the visible
//     frame; ViewContains: "M a.go" pins the starting state and proves
//     the bytes are written.
//   - final step (30 j-presses later): ViewContains: "M b.go" plus
//     ViewNotContains: "M a.go" — file 1's header has scrolled off.
//
// If checkViewSubstrings is regressed to read the cumulative stream
// instead of the latest frame, step 1's bytes still match "M a.go" and
// the ViewNotContains in the final step fails. That's the contract.
func TestScenario_ScrollThenViewTopMarker(t *testing.T) {
	t.Parallel()
	a := numberedFile("a.go", "a.go", "a1", "a2", 20)
	b := numberedFile("b.go", "b.go", "b1", "b2", 20)

	steps := []Step{
		{
			SendKey: "j",
			Expect:  Expectation{ViewContains: []string{"M a.go"}},
		},
	}
	// 29 more j-presses (total 30) walks past file 1 (header + hunk
	// header + 20 lines = 22 rows) and well into file 2; with viewport
	// height 22 the file 1 header row (row 0) is comfortably off-screen.
	for i := 0; i < 29; i++ {
		steps = append(steps, Step{SendKey: "j"})
	}
	steps[len(steps)-1].Expect = Expectation{
		ViewContains:    []string{"M b.go"},
		ViewNotContains: []string{"M a.go"},
	}

	runScenario(t, Scenario{
		Name:  "scroll_then_view_top_marker",
		Files: []diffmodel.File{a, b},
		Steps: steps,
	})
}

// wideLineFile builds a single-file diff whose context line body is long
// enough that the trailing marker only fits when the viewport is wider than
// the default 80 columns. Used by TestScenario_ResizeChangesRenderDimensions
// to prove that SendResize updates the dimensions used for screen
// reconstruction; without that update the post-resize frame would still be
// replayed through an 80-cell vt10x and the marker stayed clipped.
func wideLineFile(path, marker string) diffmodel.File {
	// 70 columns of filler followed by the marker puts the marker past the
	// 80-column clip point (cursor gutter + marker gutter + line-number
	// gutter eat ~7 cols before the body starts, so a 70-col filler crowds
	// the marker well past column 80) while leaving plenty of room inside
	// a 120-column viewport. Picking the filler length empirically rather
	// than computing it keeps the fixture readable and decoupled from the
	// renderer's exact gutter arithmetic.
	body := strings.Repeat("x", 70) + marker
	h := diffmodel.Hunk{
		BaseStart: 1, BaseLines: 1,
		HeadStart: 1, HeadLines: 1,
		Lines: []diffmodel.Line{{Prefix: ' ', Text: body}},
	}
	diffmodel.AssignLineNumbers(&h)
	return diffmodel.File{
		Status:   diffmodel.StatusModified,
		PrePath:  path, PostPath: path,
		BlobBase: "b1", BlobHead: "b2",
		Hunks: []diffmodel.Hunk{h},
	}
}

// TestScenario_ViewContainsAndGoldenTogether pins the shared-drain contract:
// when a single Step lists both ViewContains and ViewGolden, the runner must
// reconstruct the screen once and feed it to both assertions. Earlier the
// substring poll consumed the post-event frame and the subsequent golden poll
// blocked waiting for bytes that had already been delivered, so the golden
// step timed out. The fix is documented in evaluateViewExpectations.
//
// The golden snapshot is the same frame the substring matches, so the two
// assertions are not duplicates — they exercise different code paths
// (substring scan vs. byte-for-byte comparison) over the same bytes.
func TestScenario_ViewContainsAndGoldenTogether(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	runScenario(t, Scenario{
		Name:  "contains_and_golden_together",
		Files: []diffmodel.File{f},
		Steps: []Step{
			{
				SendKey: "j",
				Expect: Expectation{
					ViewContains: []string{"sitatame", "a.go"},
					ViewGolden:   "contains_and_golden_together",
				},
			},
		},
	})
}

// TestScenario_ResizeChangesRenderDimensions pins the SendResize handling in
// runScenario: the cols/rows used by renderScreen must follow the latest
// tea.WindowSizeMsg so the vt10x emulator allocates a wide-enough grid for
// the new frame. The fixture lays a marker 70 columns into the only diff
// line; at 80 columns renderRow clips the body with `…` and the marker is
// absent from the reconstructed screen, so we use ViewNotContains as the
// pre-resize guard. After SendResize {120, 40} the same body fits, and
// ViewContains finds the marker — but only if renderScreen now reconstructs
// the bytes through a 120-cell grid. If we left cols/rows pinned to 80x24
// the wider frame would still get clipped to 80 cells and ViewContains
// would time out.
func TestScenario_ResizeChangesRenderDimensions(t *testing.T) {
	t.Parallel()
	const marker = "UNIQUE_120_MARKER"
	f := wideLineFile("a.go", marker)
	runScenario(t, Scenario{
		Name:  "resize_changes_render_dimensions",
		Files: []diffmodel.File{f},
		Steps: []Step{
			{
				// Walk onto the diff body so the cursor row is the one
				// being clipped at 80 columns.
				SendKey: "j",
				Expect: Expectation{
					ViewNotContains: []string{marker},
				},
			},
			{
				SendResize: &[2]int{120, 40},
				Expect: Expectation{
					ViewContains: []string{marker},
				},
			},
		},
	})
}

// TestScenario_NoOpEventStillEvaluatesGolden pins the idle-flush contract:
// when a Step delivers a valid-but-no-op input (mouse release in the unified
// view, non-wheel mouse button, Esc with no modal open, ...), the model
// returns unchanged and the renderer emits zero new bytes. A naive
// "wait for bytes" idle-flush would time out and fail the test even though
// the golden snapshot for that Step is — correctly — the unchanged previous
// frame. The runner must drain whatever bytes are available, then evaluate
// the golden against the existing accumulated screen.
//
// The scenario walks one row down (producing a real frame so the golden
// snapshot has a stable, post-`j` baseline), then sends a no-op (a mouse
// release event, which the unified-mode handler explicitly drops at
// model.go's `msg.Action != tea.MouseActionPress` guard). The same golden
// file used for the post-`j` frame is asserted again on the no-op Step — if
// the idle-flush were still hard-waiting on new bytes, that second
// assertion would time out and fail.
//
// Why a mouse release in particular: it survives every layer of the input
// pipeline (mouseMsgFromSpec accepts "release", tea forwards it, the model
// reads it as a MouseMsg) and is documented in scenario.go as a legitimate
// DSL input. The same idle-flush guarantee applies to the other no-op
// classes — non-wheel mouse buttons (`left`/`right`/`middle` press without
// a click handler), and `esc` outside any modal — but pinning one is
// enough to lock the contract.
func TestScenario_NoOpEventStillEvaluatesGolden(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	runScenario(t, Scenario{
		Name:  "noop_event_still_evaluates_golden",
		Files: []diffmodel.File{f},
		Steps: []Step{
			{
				// Establish the baseline frame we'll re-assert below.
				SendKey: "j",
				Expect: Expectation{
					ViewGolden: "noop_event_still_evaluates_golden",
				},
			},
			{
				// Mouse release: the unified-mode model drops it before
				// touching cursor / top / selection state, so the next
				// frame is byte-identical to the previous one. With the
				// old "wait for bytes" idle-flush this Step's golden
				// check would time out; with the new idle-flush it
				// compares the unchanged screen and passes.
				SendMouse: &MouseEvent{Button: "left", Action: "release"},
				Expect: Expectation{
					ViewGolden: "noop_event_still_evaluates_golden",
				},
			},
		},
	})
}

// TestScenario_DeltaRepaintPreservesHeader is a regression case for the
// "view assertions slice the byte stream at the latest cursor marker" bug.
// bubbletea's standard renderer paints with delta updates: after the first
// frame, only rows whose contents changed are re-emitted via ANSI cursor
// motion. Unchanged rows (the help hint at the bottom, fixed banner labels,
// etc.) stay rendered from the *earlier* frame's bytes — they never re-appear
// in the cumulative output stream.
//
// The previous latestFrame heuristic sliced the stream at the most recent
// cursor-home / clear-screen marker and treated the suffix as "the current
// screen". That suffix only contains the bytes of *delta-repainted* rows,
// not the lines that were left alone by the renderer, so ViewContains for a
// fixed footer/hint substring would time out after the first delta paint.
//
// This scenario boots the model, sends one `j`, and asserts both a piece of
// the status header (top row) and a piece of the help hint (bottom row).
// The hint line is the load-bearing assertion: it does not change between
// frame 0 and frame 1, so the delta paint after `j` does not re-emit it,
// and a latestFrame-style slice cannot find it. Reconstructing the screen
// through vt10x (which replays every byte and tracks per-cell state) makes
// both substrings observable.
func TestScenario_DeltaRepaintPreservesHeader(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 10)
	runScenario(t, Scenario{
		Name:  "delta_repaint_preserves_header",
		Files: []diffmodel.File{f},
		Steps: []Step{
			{
				SendKey: "j",
				Expect: Expectation{
					ViewContains: []string{
						// statusLine (top row): file path is part of the
						// always-rendered banner that changes per cursor move.
						"sitatame",
						"a.go",
						// hintLine (bottom row): unchanged by `j`, so a
						// delta-repaint frame does not re-emit it. This is
						// the assertion that fails under the old slice-based
						// latestFrame and passes once we replay through vt10x.
						"j/k move",
					},
				},
			},
		},
	})
}
