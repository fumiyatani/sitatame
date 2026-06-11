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

func TestMouse_WheelDownScrollsTopCursorUnchanged(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	wantCursor := m.Cursor()

	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))

	if m.Top() != mouseWheelStep {
		t.Errorf("Top = %d, want %d", m.Top(), mouseWheelStep)
	}
	if m.Cursor() != wantCursor {
		t.Errorf("Cursor = %d, want unchanged %d", m.Cursor(), wantCursor)
	}
}

func TestMouse_WheelUpClampsAtZero(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	wantCursor := m.Cursor()

	// Already at top — wheel up must not push Top below zero.
	m = sendMouse(m, wheel(tea.MouseButtonWheelUp))
	if m.Top() != 0 {
		t.Errorf("Top = %d, want 0 (clamped at top)", m.Top())
	}
	if m.Cursor() != wantCursor {
		t.Errorf("Cursor = %d, want unchanged %d", m.Cursor(), wantCursor)
	}
}

func TestMouse_WheelDownClampsAtBottom(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	wantCursor := m.Cursor()

	// Spam wheel-down well past the end; Top must clamp to len(rows)-vh.
	for i := 0; i < 500; i++ {
		m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	}
	maxTop := m.Rows() - m.viewportHeight()
	if maxTop < 0 {
		maxTop = 0
	}
	if m.Top() != maxTop {
		t.Errorf("Top = %d, want %d (clamped at bottom)", m.Top(), maxTop)
	}
	if m.Cursor() != wantCursor {
		t.Errorf("Cursor = %d, want unchanged %d", m.Cursor(), wantCursor)
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
	wantCursor := m.splitCursor

	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))

	if m.splitTopForTest() != mouseWheelStep {
		t.Errorf("splitTop = %d, want %d", m.splitTopForTest(), mouseWheelStep)
	}
	if m.splitCursor != wantCursor {
		t.Errorf("splitCursor = %d, want unchanged %d", m.splitCursor, wantCursor)
	}
	// Unified Top must not move while we're in split layout.
	if m.Top() != 0 {
		t.Errorf("unified Top mutated in split: %d", m.Top())
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
