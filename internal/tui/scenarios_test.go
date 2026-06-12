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
				// `x` always emits a status-bar redraw, so we hold the
				// post-event byte guard here to surface a silently
				// dropped key as a timeout.
				SendKey:                "x",
				RequirePostEventOutput: true,
				Expect: Expectation{
					ViewContains: []string{"resolved: a-open"},
				},
			},
			{
				// Second `x` on the same row: must reopen A, NOT flip B.
				// Status bar should now read "reopened: a-open".
				SendKey:                "x",
				RequirePostEventOutput: true,
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
			// Enter split. Mode toggle always repaints, so we keep the
			// post-event byte guard.
			{
				SendKey:                "tab",
				RequirePostEventOutput: true,
				Expect:                 Expectation{ViewContains: []string{"split: preview"}},
			},
			// `c` in split must be rejected with the preview-only message.
			// The full previewOnlyMsg is too long for the 80-col status bar,
			// so we match its head — enough to prove the guard fired. The
			// status bar redraw always lands a byte, so we keep the guard.
			{
				SendKey:                "c",
				RequirePostEventOutput: true,
				Expect: Expectation{
					ViewContains: []string{"split is preview-only"},
				},
			},
			// Back to unified.
			{
				SendKey:                "tab",
				RequirePostEventOutput: true,
				Expect:                 Expectation{ViewContains: []string{"[unified]"}},
			},
			// `c` in unified opens the modal — modalView replaces the diff
			// so we look for the modal footer (always present regardless of
			// header truncation).
			{
				SendKey:                "c",
				RequirePostEventOutput: true,
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
				SendKey:                "c",
				RequirePostEventOutput: true,
				Expect:                 Expectation{ViewContains: []string{"range comment: a.go"}},
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
				SendKey:                "r",
				RequirePostEventOutput: true,
				Expect:                 Expectation{ViewContains: []string{"RANGE"}},
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
			// `j` always emits a delta repaint, so we require post-event
			// output to surface a silently dropped key.
			SendKey:                "j",
			RequirePostEventOutput: true,
			Expect:                 Expectation{ViewContains: []string{"M a.go"}},
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
	steps[len(steps)-1].RequirePostEventOutput = true

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
				SendKey:                "j",
				RequirePostEventOutput: true,
				Expect: Expectation{
					ViewContains: []string{"sitatame", "a.go"},
					ViewGolden:   "contains_and_golden_together",
				},
			},
		},
	})
}

// TestScenario_CombinedAssertionSettlesBeforeGolden pins the post-substring
// idle-settle contract: when a Step lists both ViewContains and ViewGolden,
// the substring match by itself can fire mid-frame (bubbletea writes a single
// frame across multiple syscalls — control-sequence prelude first, then the
// content burst). If the runner used the captured bytes at that instant for
// the golden comparison too, the golden would be compared against a partial
// frame and either flake under load or false-fail outright.
//
// The fix in evaluateViewExpectations is: after WaitFor returns on the
// substring predicate, call idleFlush so any straggler bytes from the same
// frame land in `captured` before it's promoted into `drained` and handed to
// compareGolden. This test exercises the contract by targeting the substring
// at the very top of the frame (the status header) while the golden snapshot
// covers the whole screen, including the hint line at the bottom row — a
// region bubbletea typically emits late within a single frame. Without the
// settle, the golden would be missing the hint line whenever the scheduler
// happens to split the frame across writes, which makes this case flaky on
// loaded CI machines (round 7 cross-review's flag).
func TestScenario_CombinedAssertionSettlesBeforeGolden(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	runScenario(t, Scenario{
		Name:  "combined_assertion_settles_before_golden",
		Files: []diffmodel.File{f},
		Steps: []Step{
			{
				SendKey:                "j",
				RequirePostEventOutput: true,
				Expect: Expectation{
					// Substring lives in the header — bubbletea emits this
					// first within the post-event frame, so WaitFor can fire
					// before the hint line is written. The golden below
					// includes the hint line, so without the post-WaitFor
					// idle-settle the comparison would race.
					ViewContains: []string{"sitatame"},
					ViewGolden:   "combined_assertion_settles_before_golden",
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
				// being clipped at 80 columns. `j` always emits a delta
				// repaint so we keep the post-event guard.
				SendKey:                "j",
				RequirePostEventOutput: true,
				Expect: Expectation{
					ViewNotContains: []string{marker},
				},
			},
			{
				// Resize forces a full repaint at the new size.
				SendResize:             &[2]int{120, 40},
				RequirePostEventOutput: true,
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

// TestScenario_RequiresPostEventOutput pins the "Step's substring assertion
// must observe post-event bytes" contract documented in
// evaluateViewExpectations. The previous runner fed `drained + latest` (with
// latest possibly empty) into renderScreen and returned true the instant
// `drained` alone matched. Because the initial render alone contains the
// status header ("sitatame"), the focused file path ("a.go"), and the help
// hint ("j/k move"), nearly every Step that asserted any of those substrings
// would short-circuit on the cumulative buffer before observing whether
// bubbletea actually reacted to the Step's input — silently dropped inputs
// produced false positives.
//
// The scenario walks one `j` (which renders a fresh frame seeding the
// cumulative buffer with all three substrings above), then walks a second
// `j` and asserts the same trio. The second Step's assertion must wait for
// the second-`j` frame bytes before declaring victory, even though every
// required substring is already in `drained` from the first Step. If
// evaluateViewExpectations were regressed to skip the `len(latest) == 0`
// guard, the assertion would still pass — but it would also pass if the
// second `j` were silently dropped, which is the regression we're guarding
// against. The runner relies on bubbletea actually emitting a frame for `j`
// (it always does, since `j` moves the cursor and the renderer redraws the
// affected rows), so this is indirect coverage: the test passes today, and
// will keep passing as long as both (a) the guard remains and (b) `j` keeps
// producing post-event bytes. If (a) is regressed, the contract is documented
// here for the next reviewer; if (b) is regressed the entire scenario harness
// breaks visibly elsewhere.
func TestScenario_RequiresPostEventOutput(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 10)
	runScenario(t, Scenario{
		Name:  "requires_post_event_output",
		Files: []diffmodel.File{f},
		Steps: []Step{
			{
				// Step 1 seeds the cumulative drained buffer with the
				// header, file path, and hint line.
				SendKey:                "j",
				RequirePostEventOutput: true,
				Expect: Expectation{
					ViewContains: []string{"sitatame", "a.go", "j/k move"},
				},
			},
			{
				// Step 2 asserts the same substrings. They are already in
				// drained from Step 1, so the guard is the only thing
				// stopping the WaitFor from returning true on its first
				// poll. We rely on `j` emitting a fresh frame.
				SendKey:                "j",
				RequirePostEventOutput: true,
				Expect: Expectation{
					ViewContains: []string{"sitatame", "a.go", "j/k move"},
				},
			},
		},
	})
}

// TestScenario_NoViewStepDrainsBoundary pins the "advance drain boundary on
// every step" contract documented at the bottom of runScenario's Step loop.
//
// Before the fix, a Step with no view expectation (e.g. just SendKey: "j" with
// an empty Expect) skipped the entire evaluateViewExpectations path — including
// the captured-byte writeback into `drained`. The bytes bubbletea emitted in
// response to that Step's input stayed parked on tm.Output()'s reader. When the
// *next* Step kicked off a substring assertion, teatest.WaitFor's first poll
// would pull those leftover bytes back as "post-event output", satisfying the
// `len(latest) > 0` guard with bytes the current Step did not produce. Any
// substring already in the previous frame would then short-circuit the
// assertion to true — even if the current Step's input had been silently
// dropped (the regression the post-event guard was supposed to catch).
//
// The scenario uses two `j` Steps:
//   - Step 1: SendKey: "j" with no view expectation. The runner advances the
//     drain boundary at the end of this Step so its bytes land in `drained`,
//     not on the live reader.
//   - Step 2: SendKey: "j" with ViewContains asserting substrings already
//     present in the Step 1 frame (header / hint line / path). These all live
//     in `drained` by the time Step 2 starts; the only way the assertion can
//     fairly pass is if Step 2's WaitFor observes its own post-event frame
//     bytes (which it does because `j` always emits a delta repaint), not
//     Step 1's bytes leaked through the unflushed reader.
//
// If a future change regresses the end-of-step drain to "skip on no-view"
// (the original bug), Step 2's WaitFor reads Step 1's parked bytes on the
// first poll and returns immediately — the test would still pass on the
// happy path, but it would also pass if Step 2's `j` were silently dropped.
// This is the same "indirect coverage" approach used by
// TestScenario_RequiresPostEventOutput: the contract is documented here, and
// the assertion exercises the path the fix protects.
func TestScenario_NoViewStepDrainsBoundary(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 10)
	runScenario(t, Scenario{
		Name:  "no_view_step_drains_boundary",
		Files: []diffmodel.File{f},
		Steps: []Step{
			{
				// No view expectation — the Step loop must still drain
				// this Step's emitted bytes into `drained` so they do
				// not leak forward as "post-event output" for Step 2.
				SendKey: "j",
			},
			{
				// Same substrings as Step 1's frame would contain. They
				// are already in `drained` after Step 1's end-of-step
				// flush; the WaitFor here can only complete by observing
				// Step 2's own post-event frame bytes.
				SendKey:                "j",
				RequirePostEventOutput: true,
				Expect: Expectation{
					ViewContains: []string{"sitatame", "a.go", "j/k move"},
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
				SendKey:                "j",
				RequirePostEventOutput: true,
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

// TestScenario_RejectsUnknownKeySpec pins the strict-key-spec contract.
//
// Before the fix, keyMsgFromSpec fell through to "treat anything else as a
// literal rune sequence" for any spec that did not match the explicit
// named-key table — multi-rune typos ("ctrl-s" instead of "ctrl+s") or
// unmapped named keys ("F1", "pgup-half") were silently injected as a
// KeyRunes message whose payload was the spec string itself. bubbletea
// then ignored the unrecognized rune sequence, the renderer emitted
// nothing, and any Step without a view assertion happily passed against
// a wrong input — the exact false-positive class the runner is supposed
// to catch.
//
// The strict path in keyMsgFromSpec now returns an error on any multi-rune
// spec that is neither a named key nor in the ctrl+ table. sendStep
// surfaces that error and runScenario calls t.Fatalf with it.
//
// We can't drive runScenario through an outer-passing sub-test (Go's
// t.Run propagates the inner Fatal to the parent, so the outer test would
// FAIL even though that is "the expected behavior"). Instead we exercise
// the underlying keyMsgFromSpec and sendStep paths directly — both
// surfaces a Scenario author would hit when typo-ing a key. This is the
// same contract pinned at the layer that actually enforces it.
//
// Table covers:
//   - "ctrl-s": typo of "ctrl+s". The reviewer's specific example.
//   - "F1": unmapped named key with multiple runes.
//   - "pgup-half": made-up name with the right shape (no ctrl+ prefix).
//
// Single-rune printable specs ("j", "?", "R") and the named-key table
// entries ("up", "down", "esc", ...) are validated implicitly by the
// scenario suite running green; if they regressed every other Scenario
// would fail noisily.
func TestScenario_RejectsUnknownKeySpec(t *testing.T) {
	t.Parallel()
	cases := []string{"ctrl-s", "F1", "pgup-half"}
	for _, spec := range cases {
		spec := spec
		t.Run(spec, func(t *testing.T) {
			t.Parallel()
			if _, err := keyMsgFromSpec(spec); err == nil {
				t.Fatalf("keyMsgFromSpec(%q) returned no error; want one", spec)
			}
			step := Step{SendKey: spec}
			if err := sendStep(nil, step); err == nil {
				t.Fatalf("sendStep(SendKey=%q) returned no error; want one", spec)
			}
		})
	}
}

// TestScenario_NoOpViewContainsOnUnchangedScreen pins the
// "no-op-tolerant view assertion" contract added in this PR.
//
// Before the fix, evaluateViewExpectations required at least one
// post-event byte before any ViewContains/ViewNotContains predicate was
// allowed to flip true. That guard was the right call for Steps whose
// input was expected to change the screen — a silently-dropped input
// would surface as a timeout rather than a false-positive match against
// `drained`'s pre-existing contents — but it ruled out a legitimate use
// case: asserting that a substring is present (or absent) on the
// *unchanged* previous frame after a valid no-op input. The DSL
// documented no-op inputs as supported (mouse release, non-wheel button,
// esc-with-no-modal, ...) but the substring branch could not be used on
// them; the golden branch already tolerated zero bytes via idleFlush,
// so the two branches diverged on their contract.
//
// The fix moves the post-event-byte requirement behind an opt-in
// Step.RequirePostEventOutput flag. The default is no-op tolerant, and
// the stale-byte protection moves to runScenario's end-of-step
// drainAvailable boundary instead (pinned by
// TestScenario_NoViewStepDrainsBoundary).
//
// This scenario walks one row down to establish a frame containing
// "sitatame" (always in the status header), then sends a mouse-release
// — a no-op for the unified-mode model — and asserts ViewContains:
// ["sitatame"] on the unchanged frame. With the old guard this would
// time out; with the new no-op-tolerant default it passes immediately
// after the idle settle.
func TestScenario_NoOpViewContainsOnUnchangedScreen(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	runScenario(t, Scenario{
		Name:  "noop_view_contains_on_unchanged_screen",
		Files: []diffmodel.File{f},
		Steps: []Step{
			{
				// Establish a stable post-`j` frame seeded with the
				// header.
				SendKey:                "j",
				RequirePostEventOutput: true,
				Expect: Expectation{
					ViewContains: []string{"sitatame"},
				},
			},
			{
				// Mouse release: no-op in unified mode. The screen is
				// byte-identical to the previous frame, so this Step's
				// substring assertion must succeed against the
				// unchanged screen with no post-event bytes required.
				SendMouse: &MouseEvent{Button: "left", Action: "release"},
				Expect: Expectation{
					ViewContains: []string{"sitatame"},
				},
			},
		},
	})
}

// TestScenario_FirstStepRequiresPostEventOutput pins the "drain the initial
// render before the Step loop" contract added to runScenario.
//
// Before the fix, teatest.NewTestModel + the WithInitialTermSize WindowSizeMsg
// caused bubbletea to emit its initial paint into tm.Output() before the Step
// loop started. `drained` was empty, so when Step 1's WaitFor predicate ran,
// `latest` (the bytes WaitFor reads off the live reader) immediately contained
// the entire initial frame — the `len(latest) > 0` guard inside
// evaluateViewExpectations was therefore satisfied by the initial paint, not by
// any byte produced in response to Step 1's input. Any ViewContains substring
// already present in the initial render ("sitatame", file path, hint line) would
// short-circuit to true on WaitFor's first poll, before bubbletea had time to
// react to Step 1's SendKey. A Step whose input was silently dropped would
// still "pass" against the initial frame.
//
// The fix is a single drainAvailable call right after NewTestModel and before
// the Step loop: it pulls the initial paint into `drained`, so Step 1's WaitFor
// starts from a clean reader and the post-event guard once again requires that
// bubbletea emit at least one byte *in response to* Step 1's input.
//
// This scenario verifies the contract the same way TestScenario_RequiresPostEventOutput
// does for Step N>1: substrings that are present in the initial render
// ("sitatame", file path, hint line) are asserted on Step 1; the only way the
// assertion can pass is if Step 1's `j` actually produces post-event bytes
// (cursor move, delta repaint) — which it does. If a future change regresses
// the initial drain, the test would still pass on the happy path, but it would
// also pass if Step 1's input were silently dropped — making this the indirect
// coverage for the guard.
func TestScenario_FirstStepRequiresPostEventOutput(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 10)
	runScenario(t, Scenario{
		Name:  "first_step_requires_post_event_output",
		Files: []diffmodel.File{f},
		Steps: []Step{
			{
				// All three substrings live in the initial render. Without
				// the pre-loop drain, WaitFor would see them on its first
				// poll via the unflushed initial frame and return true
				// before `j` had any chance to take effect. With the
				// drain in place, the post-event guard forces WaitFor to
				// wait for `j`'s delta repaint before locking in a verdict.
				SendKey:                "j",
				RequirePostEventOutput: true,
				Expect: Expectation{
					ViewContains: []string{"sitatame", "a.go", "j/k move"},
				},
			},
		},
	})
}

// TestScenario_LeftClickMovesCursor pins the click-to-place-cursor flow added
// for issue #47: a left-button press at a diff Y coordinate moves the cursor
// onto the corresponding row. We click Y=5 on a fresh model with no scrolling,
// so the targeted row is m.top + (Y - statusBarRows) = 0 + 4 = 4.
// numberedFile emits rows: file header (0), hunk header (1), content lines
// 2..N — so row 4 is the third content line.
func TestScenario_LeftClickMovesCursor(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 10)
	runScenario(t, Scenario{
		Name:       "left_click_moves_cursor",
		Files:      []diffmodel.File{f},
		WindowSize: [2]int{60, 14},
		Steps: []Step{
			{
				SendMouse:              &MouseEvent{Button: "left", Action: "press", Y: 5},
				RequirePostEventOutput: true,
				Expect: Expectation{
					Cursor: intPtr(4),
				},
			},
		},
	})
}
