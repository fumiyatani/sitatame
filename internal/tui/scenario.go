package tui

import (
	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// Scenario is a declarative description of a TUI interaction sequence. The
// runner (see scenario_runner_test.go) spins up a teatest harness around a
// freshly-built [Model], replays Steps in order, then asserts each step's
// Expectation either against the rendered view (per-step) or against the
// final model state (after tea.Quit).
//
// Scenarios let an AI agent (or human) add regression cases without writing
// boilerplate teatest plumbing. The DSL intentionally favors clarity over
// expressiveness: every field is optional, ordering is explicit, and the
// runner refuses to silently skip assertions — an unknown SendKey or an
// invalid step shape fails the test outright.
//
// Example:
//
//	tui.Scenario{
//	    Name: "mouse_wheel_help_guard",
//	    Files: []diffmodel.File{bigFile()},
//	    Steps: []tui.Step{
//	        {SendKey: "?"},
//	        {SendMouse: &tui.MouseEvent{Button: "wheel_down"}},
//	        {SendMouse: &tui.MouseEvent{Button: "wheel_down"}},
//	        {SendMouse: &tui.MouseEvent{Button: "wheel_down"}},
//	        {SendMouse: &tui.MouseEvent{Button: "wheel_down"}},
//	        {SendMouse: &tui.MouseEvent{Button: "wheel_down"},
//	            Expect: tui.Expectation{Top: intPtr(0)}},
//	    },
//	}
//
// Note: model-state assertions in Expectation (Cursor, Top, Comments, Layout,
// QuitReason, Quitting) are evaluated against the *final* model after the
// runner sends tea.Quit at the end of the scenario. Mid-scenario checks that
// need to look inside the model must rely on ViewContains/ViewGolden, which
// the runner verifies against the live teatest output reader after each Send.
// This split keeps the harness compatible with teatest's read-only Output()
// API without taking a hard dependency on probing internals mid-program.
type Scenario struct {
	// Name is used for sub-test naming and golden-file lookup.
	Name string

	// Files seeds the Model. Either Files or BuildFiles must be set.
	Files []diffmodel.File

	// BuildFiles defers file construction until just before Model creation.
	// Useful for scenarios that share fixtures across cases. Takes precedence
	// over Files when both are set.
	BuildFiles func() []diffmodel.File

	// InitialReview pre-loads the Model with an existing review so scenarios
	// can exercise `x` (resolve toggle), overlay rendering, etc. without
	// having to walk through comment creation first.
	InitialReview review.Review

	// WindowSize is the initial terminal size as {width, height}. A zero
	// value (or either axis zero) falls back to {80, 24}.
	WindowSize [2]int

	// Steps run in order. The Expectation on each Step is evaluated as
	// described in the type-level GoDoc.
	Steps []Step
}

// Step describes one input event plus optional assertions to verify after the
// event was delivered to the model.
//
// Exactly one of SendKey / SendMouse / SendResize must be set per Step.
type Step struct {
	// SendKey is a textual key spec matched against tea.KeyMsg.String().
	// Examples: "j", "k", "x", "?", "esc", "tab", "enter", "ctrl+s", "R".
	// Unknown specs cause the runner to fail the test.
	SendKey string

	// SendMouse delivers a mouse event. Releases and non-wheel buttons are
	// allowed so scenarios can pin the "release is ignored" invariant.
	SendMouse *MouseEvent

	// SendResize delivers a tea.WindowSizeMsg as {width, height}. Use this
	// to exercise viewport re-layout mid-scenario; the initial size is set
	// via Scenario.WindowSize instead.
	SendResize *[2]int

	// Expect is asserted after the event is sent. See the docs on each
	// Expectation field for evaluation semantics.
	Expect Expectation
}

// MouseEvent is a thin wrapper around tea.MouseMsg parameters so scenarios
// don't need to import bubbletea constants directly.
type MouseEvent struct {
	// Button is one of: "wheel_up", "wheel_down", "left", "right", "middle".
	Button string
	// Action is one of: "press" (default), "release". An empty string is
	// treated as "press" — the common case for wheel scrolling.
	Action string
	// X, Y are the cell coordinates of the event. Most scenarios leave them
	// zero; the unified scroll handler doesn't look at coordinates.
	X, Y int
}

// Expectation lists optional assertions for a Step. Zero-valued fields are
// skipped, so empty Expectation{} means "deliver the event, don't check
// anything". Pointer fields use nil to signal "no assertion" so the assertion
// for "zero value is expected" stays expressible.
type Expectation struct {
	// Cursor asserts m.Cursor() (final-model only).
	Cursor *int
	// Top asserts m.Top() (final-model only).
	Top *int
	// QuitReason asserts m.QuitReason() (final-model only).
	QuitReason *QuitReason
	// Quitting asserts m.Quitting() (final-model only).
	Quitting *bool
	// Layout asserts the layout mode (final-model only).
	Layout *LayoutMode
	// Comments asserts a partial match against m.Review.Comments
	// (final-model only). The comparison is index-prefixed: each pointer
	// in the slice that is non-nil is compared by anchor + state + body
	// against the same index in the model. A nil entry means "any value".
	Comments []*review.Comment
	// CommentsLen asserts len(m.Review.Comments) (final-model only).
	CommentsLen *int
	// ViewContains lists substrings that must be present in the rendered
	// view after this step (ANSI stripped). Checked against the teatest
	// output reader via WaitFor.
	ViewContains []string
	// ViewNotContains is the negative form: each substring must be absent
	// from the rendered view after this step. Useful for confirming that
	// a guard hid an element (e.g. "RANGE" tag disappears after Tab).
	ViewNotContains []string
	// ViewGolden, when non-empty, names a golden file under
	// testdata/scenarios/<ViewGolden>.golden. The runner compares it
	// against the ANSI-stripped view of this step.
	ViewGolden string
}
