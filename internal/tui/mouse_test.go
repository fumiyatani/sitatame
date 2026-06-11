package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

func wheel(button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{Button: button, Action: tea.MouseActionPress}
}

func sendMouse(m Model, msg tea.MouseMsg) Model {
	upd, _ := m.Update(msg)
	return upd.(Model)
}

// bigFile builds a single file large enough that the viewport must scroll —
// 200 context lines + headers covers any reasonable terminal height.
func bigFile() diffmodel.File {
	prefixes := make([]byte, 200)
	for i := range prefixes {
		prefixes[i] = ' '
	}
	return makeFile("big", prefixes)
}

func TestMouse_WheelDownScrollsTopCursorFollowsToTop(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)

	// Cursor starts at 0; after wheel-down the new top is past the cursor,
	// so the cursor must be pulled to the new top edge to stay on-screen.
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))

	if m.Top() != mouseWheelStep {
		t.Errorf("Top = %d, want %d", m.Top(), mouseWheelStep)
	}
	if m.Cursor() != mouseWheelStep {
		t.Errorf("Cursor = %d, want %d (clamped to new top)", m.Cursor(), mouseWheelStep)
	}
}

func TestMouse_WheelUpClampsAtZero(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	wantCursor := m.Cursor()

	// Already at top — wheel up must not push Top below zero, and the cursor
	// is already inside the viewport so it stays put.
	m = sendMouse(m, wheel(tea.MouseButtonWheelUp))
	if m.Top() != 0 {
		t.Errorf("Top = %d, want 0 (clamped at top)", m.Top())
	}
	if m.Cursor() != wantCursor {
		t.Errorf("Cursor = %d, want unchanged %d (already on-screen)", m.Cursor(), wantCursor)
	}
}

func TestMouse_WheelDownClampsAtBottom(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)

	// Spam wheel-down well past the end; Top must clamp to len(rows)-vh and
	// cursor must clamp into the visible window's bottom edge.
	for i := 0; i < 500; i++ {
		m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	}
	vh := m.viewportHeight()
	maxTop := m.Rows() - vh
	if maxTop < 0 {
		maxTop = 0
	}
	if m.Top() != maxTop {
		t.Errorf("Top = %d, want %d (clamped at bottom)", m.Top(), maxTop)
	}
	// Cursor must end up inside the (final) viewport. We don't pin it to a
	// specific row — the exact value depends on whether the cursor was pulled
	// in from above (lands at top) or pushed down (lands at bottom).
	if m.Cursor() < m.Top() || m.Cursor() >= m.Top()+vh {
		t.Errorf("Cursor %d outside viewport [%d, %d)", m.Cursor(), m.Top(), m.Top()+vh)
	}
	if last := m.Rows() - 1; m.Cursor() > last {
		t.Errorf("Cursor %d past last row %d", m.Cursor(), last)
	}
}

// Regression for the off-screen-cursor bug: after a wheel scroll, pressing j
// must NOT snap top back to where the cursor used to be. The cursor must be
// pulled into the viewport by the wheel handler so scrollToCursor is a no-op.
func TestMouse_WheelThenMoveCursorDoesNotRewindTop(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)

	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	topAfterWheel := m.Top()
	if topAfterWheel == 0 {
		t.Fatalf("wheel didn't move top; precondition failed")
	}
	m, _ = applyKey(m, "j")
	if m.Top() < topAfterWheel {
		t.Errorf("Top rewound after j: got %d, want >= %d", m.Top(), topAfterWheel)
	}
	vh := m.viewportHeight()
	if m.Cursor() < m.Top() || m.Cursor() >= m.Top()+vh {
		t.Errorf("Cursor %d off-screen after j: viewport [%d, %d)", m.Cursor(), m.Top(), m.Top()+vh)
	}
}

// Symmetric check: wheel-up after scrolling down, then k must not rewind top.
func TestMouse_WheelUpThenMoveCursorDoesNotRewindTop(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)

	// Scroll well past the cursor so the cursor is off-screen below.
	for i := 0; i < 10; i++ {
		m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	}
	// Now wheel up a little; cursor should follow to remain on-screen.
	m = sendMouse(m, wheel(tea.MouseButtonWheelUp))
	topAfterWheel := m.Top()
	vh := m.viewportHeight()
	if m.Cursor() < topAfterWheel || m.Cursor() >= topAfterWheel+vh {
		t.Fatalf("Cursor %d off-screen after wheel: viewport [%d, %d)", m.Cursor(), topAfterWheel, topAfterWheel+vh)
	}
	// k must not push top down (i.e. rewind toward the cursor's old position).
	m, _ = applyKey(m, "k")
	if m.Top() > topAfterWheel {
		t.Errorf("Top advanced after k: got %d, want <= %d", m.Top(), topAfterWheel)
	}
}

func TestMouse_WheelDownScrollsUpAfterDown(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	if m.Top() != 2*mouseWheelStep {
		t.Fatalf("Top after two downs = %d, want %d", m.Top(), 2*mouseWheelStep)
	}
	m = sendMouse(m, wheel(tea.MouseButtonWheelUp))
	if m.Top() != mouseWheelStep {
		t.Errorf("Top after up = %d, want %d", m.Top(), mouseWheelStep)
	}
}

func TestMouse_ReleaseIgnored(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	// A release event with WheelDown button must not scroll.
	upd, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionRelease})
	m = upd.(Model)
	if m.Top() != 0 {
		t.Errorf("Top = %d, want 0 (release ignored)", m.Top())
	}
}

func TestMouse_NonWheelButtonsIgnored(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	for _, btn := range []tea.MouseButton{tea.MouseButtonLeft, tea.MouseButtonRight, tea.MouseButtonMiddle} {
		m = sendMouse(m, wheel(btn))
	}
	if m.Top() != 0 {
		t.Errorf("Top = %d, want 0 (non-wheel buttons must not scroll)", m.Top())
	}
}

func TestMouse_IgnoredWhileModalOpen(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	// Move cursor onto a line row, then open the comment modal.
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	if m.Modal() == nil {
		t.Fatal("expected comment modal to be open")
	}
	beforeTop := m.Top()
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	if m.Top() != beforeTop {
		t.Errorf("modal open: Top = %d, want %d (mouse must be ignored)", m.Top(), beforeTop)
	}
}

// Help is rendered as a full-screen overlay. If wheel events were forwarded to
// the diff while help is up, the user would see no change until closing help —
// then the viewport would silently jump. Make sure wheel is dropped instead.
func TestMouse_IgnoredWhileHelpOpen(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	m, _ = applyKey(m, "?")
	if !m.ShowingHelp() {
		t.Fatal("expected help to be open")
	}
	beforeTop := m.Top()
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	if m.Top() != beforeTop {
		t.Errorf("help open: Top = %d, want %d (mouse must be ignored)", m.Top(), beforeTop)
	}
}

// Shift+R opens the review-level modal directly (no anchor needed). Same
// invariant as the comment modal: wheel events must not leak through to the
// background viewport.
func TestMouse_IgnoredWhileReviewModalOpen(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	m.openReviewModal()
	if m.Modal() == nil {
		t.Fatal("expected review modal to be open")
	}
	beforeTop := m.Top()
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	if m.Top() != beforeTop {
		t.Errorf("review modal open: Top = %d, want %d (mouse must be ignored)", m.Top(), beforeTop)
	}
}

// SplitTop accessor for tests; mirrored on Top()/Cursor().
func (m Model) splitTopForTest() int { return m.splitTop }

func TestMouse_SplitLayout_WheelDownScrolls(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	m = sendNamedKey(m, tea.KeyTab)
	if m.layout != LayoutSplit {
		t.Fatalf("expected LayoutSplit, got %v", m.layout)
	}

	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))

	if m.splitTopForTest() != mouseWheelStep {
		t.Errorf("splitTop = %d, want %d", m.splitTopForTest(), mouseWheelStep)
	}
	// splitCursor started at 0 (off-screen after a 3-row scroll) and must be
	// pulled into the new viewport so j/k from here don't snap top back.
	vh := m.viewportHeight()
	if m.splitCursor < m.splitTopForTest() || m.splitCursor >= m.splitTopForTest()+vh {
		t.Errorf("splitCursor %d outside viewport [%d, %d)", m.splitCursor, m.splitTopForTest(), m.splitTopForTest()+vh)
	}
	// Unified Top must not move while we're in split layout.
	if m.Top() != 0 {
		t.Errorf("unified Top mutated in split: %d", m.Top())
	}
}

func TestMouse_SplitLayout_WheelThenMoveCursorDoesNotRewindTop(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	m = sendNamedKey(m, tea.KeyTab)

	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	topAfterWheel := m.splitTopForTest()
	if topAfterWheel == 0 {
		t.Fatalf("wheel didn't move splitTop; precondition failed")
	}
	m, _ = applyKey(m, "j")
	if m.splitTopForTest() < topAfterWheel {
		t.Errorf("splitTop rewound after j: got %d, want >= %d", m.splitTopForTest(), topAfterWheel)
	}
	vh := m.viewportHeight()
	if m.splitCursor < m.splitTopForTest() || m.splitCursor >= m.splitTopForTest()+vh {
		t.Errorf("splitCursor %d off-screen after j: viewport [%d, %d)", m.splitCursor, m.splitTopForTest(), m.splitTopForTest()+vh)
	}
}

func TestMouse_SplitLayout_WheelUpClampsAtZero(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	m = sendNamedKey(m, tea.KeyTab)
	m = sendMouse(m, wheel(tea.MouseButtonWheelUp))
	if m.splitTopForTest() != 0 {
		t.Errorf("splitTop = %d, want 0", m.splitTopForTest())
	}
}

func TestMouse_SplitLayout_WheelDownClampsAtBottom(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	m = sendNamedKey(m, tea.KeyTab)
	for i := 0; i < 500; i++ {
		m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	}
	maxTop := len(m.splitRows) - m.viewportHeight()
	if maxTop < 0 {
		maxTop = 0
	}
	if m.splitTopForTest() != maxTop {
		t.Errorf("splitTop = %d, want %d (clamped)", m.splitTopForTest(), maxTop)
	}
}
