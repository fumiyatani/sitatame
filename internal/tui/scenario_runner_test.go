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
//   - for ViewContains / ViewNotContains / ViewGolden, polls the
//     accumulated output via teatest.WaitFor until the view stabilizes
//   - sends tea.Quit, awaits FinalModel, and asserts every model-state
//     Expectation field across all steps (the final model is the only place
//     the runner can reach back into the Model without smuggling probes
//     through tea.Cmd, which the spec explicitly avoids).
//
// Model-state assertions on non-final steps are still permitted and are
// evaluated against the final model; this is documented in scenario.go so
// authors can decide whether to split a multi-state case into two scenarios
// or fold it into one with view-based mid-step checks.
func runScenario(t *testing.T, sc Scenario) {
	t.Helper()

	files := sc.Files
	if sc.BuildFiles != nil {
		files = sc.BuildFiles()
	}
	if files == nil {
		t.Fatalf("scenario %q: neither Files nor BuildFiles produced fixtures", sc.Name)
	}

	w, h := sc.WindowSize[0], sc.WindowSize[1]
	if w == 0 || h == 0 {
		w, h = 80, 24
	}

	m := New(files, sc.InitialReview)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(w, h))

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

		// View-level checks need a moment for the program goroutine to
		// process the message and re-render. teatest.WaitFor handles
		// that polling for us.
		if len(step.Expect.ViewContains) > 0 || len(step.Expect.ViewNotContains) > 0 {
			checkViewSubstrings(t, sc.Name, stepLabel, tm, &drained, step.Expect)
		}
		if step.Expect.ViewGolden != "" {
			checkViewGolden(t, sc.Name, stepLabel, tm, &drained, step.Expect.ViewGolden)
		}
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

// checkViewSubstrings polls the teatest output until every required substring
// appears (or every forbidden substring is confirmed absent at the steady
// state). The drained buffer carries history across calls because the output
// reader is consumed by ReadAll on each WaitFor pass.
func checkViewSubstrings(
	t *testing.T,
	name, stepLabel string,
	tm *teatest.TestModel,
	drained *bytes.Buffer,
	expect Expectation,
) {
	t.Helper()
	required := expect.ViewContains
	forbidden := expect.ViewNotContains

	condition := func(latest []byte) bool {
		out := append([]byte(nil), drained.Bytes()...)
		out = append(out, latest...)
		text := stripANSI(string(out))
		for _, want := range required {
			if !strings.Contains(text, want) {
				return false
			}
		}
		// For ViewNotContains the polling can only confirm "not yet". The
		// runner accepts the first time required passes and forbidden is
		// absent — if a forbidden substring is currently in the buffer
		// because of an earlier step's frame, the condition keeps waiting
		// until a later frame overwrites it (teatest output is cumulative
		// but the ANSI compressor collapses consecutive frames, so this
		// converges in practice).
		for _, bad := range forbidden {
			if strings.Contains(text, bad) {
				return false
			}
		}
		return true
	}

	// Capture bytes via a TeeReader so WaitFor can poll while we keep a
	// copy for the next step. Without this, ViewContains in step N+1
	// would not see frames produced by step N.
	var captured bytes.Buffer
	tee := io.TeeReader(tm.Output(), &captured)
	teatest.WaitFor(t, tee, func(b []byte) bool {
		return condition(b)
	},
		teatest.WithDuration(scenarioWaitForDuration),
		teatest.WithCheckInterval(scenarioWaitInterval),
	)
	drained.Write(captured.Bytes())
}

// checkViewGolden compares the current rendered view against a golden file.
// Like golden_test.go, the comparison is ANSI-stripped and trailing whitespace
// trimmed per line so cursor / textarea noise doesn't make snapshots flaky.
func checkViewGolden(
	t *testing.T,
	name, stepLabel string,
	tm *teatest.TestModel,
	drained *bytes.Buffer,
	goldenName string,
) {
	t.Helper()

	// Drain everything currently buffered so the snapshot captures the
	// latest frame. The ANSICompressor in teatest collapses consecutive
	// frames, so the last full View is what survives at the tail.
	var captured bytes.Buffer
	tee := io.TeeReader(tm.Output(), &captured)
	teatest.WaitFor(t, tee, func(b []byte) bool {
		// Wait until at least one frame has come through this step.
		return len(b) > 0
	},
		teatest.WithDuration(scenarioWaitForDuration),
		teatest.WithCheckInterval(scenarioWaitInterval),
	)
	drained.Write(captured.Bytes())

	combined := stripANSI(drained.String())
	lines := strings.Split(combined, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	got := strings.Join(lines, "\n")

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
