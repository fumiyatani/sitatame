package tui

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/hinshun/vt10x"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// updateScenarioGolden mirrors the flag used by golden_test.go. We register
// our own so the two suites don't collide; callers run them together via
// `go test -update-golden ./internal/tui/...`.
var updateScenarioGolden = flag.Bool("update-scenario-golden", false,
	"rewrite testdata/scenarios/*.golden instead of comparing")

const (
	scenarioWaitForDuration = 2 * time.Second
	scenarioWaitInterval    = 5 * time.Millisecond
	// scenarioIdleFlushDuration caps how long we sit on the teatest output
	// reader when a step has *no* substring requirement (e.g. golden-only,
	// or substrings already satisfied before the latest event was processed).
	// Bubbletea typically emits the new frame within a few ms of receiving a
	// message; the budget here is a paranoia ceiling rather than an expected
	// wait. We keep it deliberately small so passing scenarios stay fast.
	scenarioIdleFlushDuration = 250 * time.Millisecond
	// finalQuitTimeout caps how long we wait for the program goroutine to
	// finish after we send tea.Quit. The actual model returns immediately
	// once Quit is processed, so this is just a paranoia ceiling.
	finalQuitTimeout = 3 * time.Second
)

// runScenario executes one Scenario against a teatest harness and asserts
// every Step's Expectation. See scenario.go for the DSL contract.
//
// The runner:
//   - builds a fresh Model from sc.Files / sc.BuildFiles + sc.InitialReview
//   - starts a teatest.TestModel with the requested initial window size
//   - replays each Step:
//   - delivers the input message via tm.Send
//   - tracks the current terminal dimensions (initial size, plus any
//     SendResize updates) so view reconstruction uses the latest cols/rows
//   - for each Step, runs at most one drain pass per step and asserts
//     every view-level expectation (ViewContains / ViewNotContains /
//     ViewGolden) against the same reconstructed screen — sharing the
//     drain is what lets a single Step combine substring and golden
//     checks without one starving the other
//   - sends tea.Quit, awaits FinalModel, and asserts every model-state
//     Expectation field across all steps (the final model is the only place
//     the runner can reach back into the Model without smuggling probes
//     through tea.Cmd, which the spec explicitly avoids).
//
// Model-state assertions on non-final steps are still permitted and are
// evaluated against the final model; this is documented in scenario.go so
// authors can decide whether to split a multi-state case into two scenarios
// or fold it into one with view-based mid-step checks.
//
// View assertions (ViewContains / ViewNotContains / ViewGolden) are evaluated
// against the currently visible terminal screen, not the cumulative byte
// stream. teatest.Output() yields every byte bubbletea has ever written to
// its in-memory writer, and bubbletea's standard renderer uses *delta repaint*
// — after the first frame, only changed lines are re-emitted via ANSI cursor
// motion. A naive substring scan of the cumulative bytes therefore both
//
//   - reports stale matches from prior frames (ViewContains can pass on a
//     substring that has since scrolled off-screen), and
//   - reports never-cleared matches (ViewNotContains can never pass once a
//     forbidden substring has been drawn once).
//
// Earlier iterations tried to slice the stream at the most recent inter-frame
// cursor marker, but delta repaint means most frames have *no* such marker —
// only changed rows are re-emitted, and the fixed header/body never reappear
// in the byte stream. The runner now feeds the cumulative bytes into a
// hinshun/vt10x virtual terminal sized to the scenario's WindowSize and
// renders the resulting cell grid into a string, which exactly mirrors what a
// real terminal would display.
func runScenario(t *testing.T, sc Scenario) {
	t.Helper()

	files := sc.Files
	if sc.BuildFiles != nil {
		files = sc.BuildFiles()
	}
	if files == nil {
		t.Fatalf("scenario %q: neither Files nor BuildFiles produced fixtures", sc.Name)
	}

	cols, rows := sc.WindowSize[0], sc.WindowSize[1]
	if cols == 0 || rows == 0 {
		cols, rows = 80, 24
	}

	m := New(files, sc.InitialReview)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(cols, rows))

	// drained accumulates everything we have already pulled out of the
	// teatest output reader so that ViewContains checks done in later
	// steps see the full history, not just the bytes produced since the
	// last drain. teatest.WaitFor reads from the live reader on each call,
	// which would otherwise lose early frames once a later step polls.
	var drained bytes.Buffer

	for i, step := range sc.Steps {
		stepLabel := fmt.Sprintf("step %d", i+1)
		if err := sendStep(tm, step); err != nil {
			t.Fatalf("scenario %q %s: %v", sc.Name, stepLabel, err)
		}

		// SendResize must update the terminal dimensions used for
		// subsequent view reconstruction; otherwise renderScreen would
		// keep replaying the byte stream through a vt10x sized to the
		// pre-resize window and miss any cells the model now writes past
		// the old right margin / bottom row.
		if step.SendResize != nil {
			size := *step.SendResize
			if size[0] > 0 && size[1] > 0 {
				cols, rows = size[0], size[1]
			}
		}

		// View assertions share a single drain pass per step so that a
		// substring check and a golden check on the same Step reconstruct
		// the screen from identical bytes. Without this sharing, the
		// substring poll would consume the new frame and the subsequent
		// golden poll would wait on bytes that already arrived.
		evaluateViewExpectations(t, sc.Name, stepLabel, tm, &drained, cols, rows, step.Expect)
	}

	if err := tm.Quit(); err != nil {
		t.Fatalf("scenario %q: quit failed: %v", sc.Name, err)
	}
	fm := tm.FinalModel(t, teatest.WithFinalTimeout(finalQuitTimeout))
	final, ok := fm.(Model)
	if !ok {
		t.Fatalf("scenario %q: FinalModel is %T, want tui.Model", sc.Name, fm)
	}

	for i, step := range sc.Steps {
		stepLabel := fmt.Sprintf("step %d", i+1)
		checkFinalState(t, sc.Name, stepLabel, final, step.Expect)
	}
}

// sendStep validates the Step shape and dispatches exactly one input.
func sendStep(tm *teatest.TestModel, step Step) error {
	specified := 0
	if step.SendKey != "" {
		specified++
	}
	if step.SendMouse != nil {
		specified++
	}
	if step.SendResize != nil {
		specified++
	}
	if specified != 1 {
		return fmt.Errorf("step must set exactly one of SendKey/SendMouse/SendResize (got %d)", specified)
	}

	switch {
	case step.SendKey != "":
		msg, err := keyMsgFromSpec(step.SendKey)
		if err != nil {
			return err
		}
		tm.Send(msg)
	case step.SendMouse != nil:
		msg, err := mouseMsgFromSpec(*step.SendMouse)
		if err != nil {
			return err
		}
		tm.Send(msg)
	case step.SendResize != nil:
		size := *step.SendResize
		tm.Send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
	}
	return nil
}

// keyMsgFromSpec maps the textual key spec used in Scenario steps onto a
// tea.KeyMsg. Single printable runes ("j", "k", "x", "?", "R", ...) become
// KeyRunes messages; the named specs below are translated to their tea.KeyType
// equivalents.
func keyMsgFromSpec(spec string) (tea.KeyMsg, error) {
	switch spec {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}, nil
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}, nil
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}, nil
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}, nil
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}, nil
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}, nil
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}, nil
	}
	if strings.HasPrefix(spec, "ctrl+") {
		return tea.KeyMsg{}, fmt.Errorf("unsupported ctrl key %q (add to keyMsgFromSpec if needed)", spec)
	}
	// Treat anything else as a literal rune sequence — typical for single
	// printable characters (`j`, `?`, `R`, etc.). Multi-rune specs are
	// accepted too; the tea event loop forwards them verbatim.
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(spec)}, nil
}

func mouseMsgFromSpec(ev MouseEvent) (tea.MouseMsg, error) {
	var btn tea.MouseButton
	switch ev.Button {
	case "wheel_up":
		btn = tea.MouseButtonWheelUp
	case "wheel_down":
		btn = tea.MouseButtonWheelDown
	case "left":
		btn = tea.MouseButtonLeft
	case "right":
		btn = tea.MouseButtonRight
	case "middle":
		btn = tea.MouseButtonMiddle
	default:
		return tea.MouseMsg{}, fmt.Errorf("unknown mouse button %q", ev.Button)
	}
	action := tea.MouseActionPress
	switch ev.Action {
	case "", "press":
		action = tea.MouseActionPress
	case "release":
		action = tea.MouseActionRelease
	default:
		return tea.MouseMsg{}, fmt.Errorf("unknown mouse action %q", ev.Action)
	}
	return tea.MouseMsg{Button: btn, Action: action, X: ev.X, Y: ev.Y}, nil
}

// renderScreen reconstructs the currently visible terminal contents from the
// cumulative bubbletea output stream. teatest.Output() returns every byte
// bubbletea has ever written, and bubbletea's standard renderer paints with
// delta updates — only changed rows are re-emitted via ANSI cursor motion,
// while fixed header/body lines stay rendered from earlier frames. A plain
// substring scan of the raw bytes therefore cannot tell what is on screen.
//
// We replay the entire byte stream through a hinshun/vt10x virtual terminal
// sized to the scenario's WindowSize, then dump the resulting cell grid into
// a string with trailing whitespace trimmed per line. The output matches what
// a real terminal would display after consuming the same bytes, including
// delta repaints, cursor motions, and clear-screen sequences.
//
// vt10x's Cell(x, y) returns the rune at each grid position; an empty cell
// has Char == 0, which we render as a space so the resulting string lines up
// with header/body assertions written against the literal View() output.
func renderScreen(out []byte, cols, rows int) string {
	if cols <= 0 || rows <= 0 {
		cols, rows = 80, 24
	}
	term := vt10x.New(vt10x.WithSize(cols, rows))
	_, _ = term.Write(out)
	var b strings.Builder
	for y := 0; y < rows; y++ {
		var line strings.Builder
		for x := 0; x < cols; x++ {
			g := term.Cell(x, y).Char
			if g == 0 {
				g = ' '
			}
			line.WriteRune(g)
		}
		b.WriteString(strings.TrimRight(line.String(), " \t"))
		b.WriteByte('\n')
	}
	return b.String()
}

// evaluateViewExpectations performs at most one drain pass per step and then
// asserts every view-level expectation against the same reconstructed screen.
// Sharing the drain pass is what makes "ViewContains + ViewGolden on the same
// Step" work: a previous design ran the substring poll first and then re-polled
// for golden, which timed out whenever the newest frame had already been
// consumed by the first poll.
//
// Polling strategy:
//   - If ViewContains / ViewNotContains is set, we use teatest.WaitFor with the
//     substring condition. The poll terminates as soon as the reconstructed
//     screen satisfies every required substring and contains no forbidden one.
//   - Otherwise, we still need to flush whatever bytes the latest event
//     produced so a golden check (or a Step with no view checks at all)
//     reflects the post-event frame. We use a short idle-flush wait that
//     succeeds as soon as at least one byte has come through, capped by
//     scenarioIdleFlushDuration so steps with no follow-up output stay quick.
//
// drained accumulates *every* byte teatest has ever emitted; renderScreen
// requires the full history because bubbletea's delta repaint leaves the
// fixed header/body in earlier frames and only re-emits changed rows.
func evaluateViewExpectations(
	t *testing.T,
	name, stepLabel string,
	tm *teatest.TestModel,
	drained *bytes.Buffer,
	cols, rows int,
	expect Expectation,
) {
	t.Helper()

	required := expect.ViewContains
	forbidden := expect.ViewNotContains
	hasSubstringCheck := len(required) > 0 || len(forbidden) > 0
	hasGoldenCheck := expect.ViewGolden != ""

	if !hasSubstringCheck && !hasGoldenCheck {
		// No view-level assertion for this Step. Skip the drain so the
		// next view assertion gets the freshest bytes; nothing in this
		// branch needs the screen to be observed right now.
		return
	}

	// Capture bytes via a TeeReader so WaitFor can poll while we keep a
	// copy for the next step. Without this, view assertions in step N+1
	// would not see frames produced by step N.
	var captured bytes.Buffer
	tee := io.TeeReader(tm.Output(), &captured)

	if hasSubstringCheck {
		teatest.WaitFor(t, tee, func(latest []byte) bool {
			out := append([]byte(nil), drained.Bytes()...)
			out = append(out, latest...)
			frame := renderScreen(out, cols, rows)
			for _, want := range required {
				if !strings.Contains(frame, want) {
					return false
				}
			}
			for _, bad := range forbidden {
				if strings.Contains(frame, bad) {
					return false
				}
			}
			return true
		},
			teatest.WithDuration(scenarioWaitForDuration),
			teatest.WithCheckInterval(scenarioWaitInterval),
		)
	} else {
		// Idle-flush: wait until at least one byte arrives, capped by a
		// short ceiling so steps that legitimately produce no output (or
		// already flushed) don't stall.
		teatest.WaitFor(t, tee, func(latest []byte) bool {
			return len(latest) > 0
		},
			teatest.WithDuration(scenarioIdleFlushDuration),
			teatest.WithCheckInterval(scenarioWaitInterval),
		)
	}

	drained.Write(captured.Bytes())

	if hasGoldenCheck {
		compareGolden(t, name, stepLabel, drained.Bytes(), cols, rows, expect.ViewGolden)
	}
}

// compareGolden reconstructs the screen from the cumulative byte stream and
// compares it against testdata/scenarios/<goldenName>.golden. Trailing
// whitespace per line is trimmed inside renderScreen so cursor/textarea noise
// doesn't make snapshots flaky.
func compareGolden(
	t *testing.T,
	name, stepLabel string,
	out []byte,
	cols, rows int,
	goldenName string,
) {
	t.Helper()
	got := renderScreen(out, cols, rows)

	path := filepath.Join("testdata", "scenarios", goldenName+".golden")
	if *updateScenarioGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("scenario %q %s: %v", name, stepLabel, err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("scenario %q %s: %v", name, stepLabel, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("scenario %q %s: missing golden %s — run with -update-scenario-golden to seed",
			name, stepLabel, path)
	}
	if string(want) != got {
		t.Errorf("scenario %q %s: golden mismatch (%s)\nwant:\n%s\n----\ngot:\n%s",
			name, stepLabel, path, string(want), got)
	}
}

// checkFinalState asserts every model-state Expectation field from a Step
// against the final model. Fields left as nil/zero are skipped.
func checkFinalState(t *testing.T, name, stepLabel string, fm Model, expect Expectation) {
	t.Helper()
	if expect.Cursor != nil {
		if got := fm.Cursor(); got != *expect.Cursor {
			t.Errorf("scenario %q %s: Cursor = %d, want %d", name, stepLabel, got, *expect.Cursor)
		}
	}
	if expect.Top != nil {
		if got := fm.Top(); got != *expect.Top {
			t.Errorf("scenario %q %s: Top = %d, want %d", name, stepLabel, got, *expect.Top)
		}
	}
	if expect.QuitReason != nil {
		if got := fm.QuitReason(); got != *expect.QuitReason {
			t.Errorf("scenario %q %s: QuitReason = %v, want %v", name, stepLabel, got, *expect.QuitReason)
		}
	}
	if expect.Quitting != nil {
		if got := fm.Quitting(); got != *expect.Quitting {
			t.Errorf("scenario %q %s: Quitting = %v, want %v", name, stepLabel, got, *expect.Quitting)
		}
	}
	if expect.Layout != nil {
		if got := fm.layout; got != *expect.Layout {
			t.Errorf("scenario %q %s: Layout = %v, want %v", name, stepLabel, got, *expect.Layout)
		}
	}
	if expect.CommentsLen != nil {
		if got := len(fm.Review.Comments); got != *expect.CommentsLen {
			t.Errorf("scenario %q %s: len(Comments) = %d, want %d",
				name, stepLabel, got, *expect.CommentsLen)
		}
	}
	for i, want := range expect.Comments {
		if want == nil {
			continue
		}
		if i >= len(fm.Review.Comments) {
			t.Errorf("scenario %q %s: Comments[%d] missing (have %d)",
				name, stepLabel, i, len(fm.Review.Comments))
			continue
		}
		got := fm.Review.Comments[i]
		if want.Kind != "" && got.Kind != want.Kind {
			t.Errorf("scenario %q %s: Comments[%d].Kind = %q, want %q",
				name, stepLabel, i, got.Kind, want.Kind)
		}
		if want.State != "" && got.State != want.State {
			t.Errorf("scenario %q %s: Comments[%d].State = %q, want %q",
				name, stepLabel, i, got.State, want.State)
		}
		if want.AnchorID != "" && got.AnchorID != want.AnchorID {
			t.Errorf("scenario %q %s: Comments[%d].AnchorID = %q, want %q",
				name, stepLabel, i, got.AnchorID, want.AnchorID)
		}
		if want.Line != 0 && got.Line != want.Line {
			t.Errorf("scenario %q %s: Comments[%d].Line = %d, want %d",
				name, stepLabel, i, got.Line, want.Line)
		}
		if want.LineStart != 0 && got.LineStart != want.LineStart {
			t.Errorf("scenario %q %s: Comments[%d].LineStart = %d, want %d",
				name, stepLabel, i, got.LineStart, want.LineStart)
		}
		if want.LineEnd != 0 && got.LineEnd != want.LineEnd {
			t.Errorf("scenario %q %s: Comments[%d].LineEnd = %d, want %d",
				name, stepLabel, i, got.LineEnd, want.LineEnd)
		}
		if want.Body != "" && got.Body != want.Body {
			t.Errorf("scenario %q %s: Comments[%d].Body = %q, want %q",
				name, stepLabel, i, got.Body, want.Body)
		}
		if want.Path != "" && got.Path != want.Path {
			t.Errorf("scenario %q %s: Comments[%d].Path = %q, want %q",
				name, stepLabel, i, got.Path, want.Path)
		}
	}
}

// Helpers for scenarios that need pointer-typed expectations without a noisy
// inline declaration.

func intPtr(v int) *int                  { return &v }
func boolPtr(v bool) *bool               { return &v }
func quitPtr(v QuitReason) *QuitReason   { return &v }
func layoutPtr(v LayoutMode) *LayoutMode { return &v }

// scenarioBigFile is a self-contained large file fixture so scenarios that
// need to scroll don't have to reach into mouse_test.go's bigFile (test
// helpers aren't sub-test scoped, but importing across files within the same
// package is fine — this is just clarity).
func scenarioBigFile() diffmodel.File {
	prefixes := make([]byte, 200)
	for i := range prefixes {
		prefixes[i] = ' '
	}
	return makeFile("scenario_big.go", prefixes)
}

// scenarioFileWithComment seeds a numbered file plus a single open comment
// anchored to a known HeadLine. Used by the resolve_toggle_undo scenario.
func scenarioFileWithComment(
	path string,
	headLine int,
	anchorID string,
) (diffmodel.File, review.Comment) {
	f := numberedFile(path, path, "b1", "b2", 5)
	c := review.Comment{
		Anchor: review.Anchor{
			AnchorID: anchorID,
			Kind:     review.KindLine,
			Path:     path,
			Side:     review.SideHead,
			Line:     headLine,
			Blob:     "b2",
		},
		State: review.StateOpen,
		Body:  "needs work",
	}
	return f, c
}
