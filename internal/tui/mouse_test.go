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

// TestMouse_WheelExtendsActiveSelection guards the wheel-vs-range-selection
// integration: starting a range with `r`, then scrolling with the wheel so
// the cursor is clamped to a new row, must keep Selection.Extent in sync with
// the cursor. Otherwise `c` saves a comment against the pre-wheel extent and
// the on-screen highlight disagrees with what gets persisted.
func TestMouse_WheelExtendsActiveSelection(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)

	// Move onto the first content row (skip fileHeader + hunkHeader) so `r`
	// can anchor a selection — startSelection() rejects non-line rows.
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	anchor := m.Cursor()
	m, _ = applyKey(m, "r")
	if m.selection == nil {
		t.Fatalf("expected selection after r")
	}
	if m.selection.Anchor != anchor || m.selection.Extent != anchor {
		t.Fatalf("initial selection = (anchor=%d, extent=%d), want both %d",
			m.selection.Anchor, m.selection.Extent, anchor)
	}

	// Wheel down past the cursor; scrollViewportBy will clamp the cursor
	// forward to stay on-screen. Selection.Extent must follow.
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))

	if m.Cursor() == anchor {
		t.Fatalf("wheel didn't move cursor; precondition failed (cursor=%d)", m.Cursor())
	}
	if m.selection == nil {
		t.Fatalf("selection cleared unexpectedly after wheel")
	}
	if m.selection.Extent != m.Cursor() {
		t.Errorf("Selection.Extent = %d, want %d (cursor after wheel)",
			m.selection.Extent, m.Cursor())
	}
	if m.selection.Anchor != anchor {
		t.Errorf("Selection.Anchor drifted: got %d, want %d", m.selection.Anchor, anchor)
	}
}

// click builds a left-button press MouseMsg at (x, y). The diff handler
// only consults Y, so X is left at 0 in the helpers below.
func click(y int) tea.MouseMsg {
	return tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: y}
}

// TestMouse_LeftClickMovesCursor: a click on row Y inside the diff viewport
// must move the cursor to m.top + Y - statusBarRows. statusBarRows == 1 in
// the current layout (status bar occupies Y=0, diff starts at Y=1).
func TestMouse_LeftClickMovesCursor(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	// Click at Y=5 → row 5 - 1 = 4 (offset from m.top, which is 0).
	m = sendMouse(m, click(5))
	if got, want := m.Cursor(), m.Top()+5-statusBarRows; got != want {
		t.Errorf("Cursor = %d, want %d", got, want)
	}
}

// TestMouse_LeftClickAccountsForTop: cursor must move to m.top + (Y-1) so a
// click in the same physical row picks a different logical row after scrolling.
func TestMouse_LeftClickAccountsForTop(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	// Scroll first so m.top > 0, then click at Y=3.
	m = sendMouse(m, wheel(tea.MouseButtonWheelDown))
	top := m.Top()
	if top == 0 {
		t.Fatalf("precondition failed: wheel did not move top")
	}
	m = sendMouse(m, click(3))
	if got, want := m.Cursor(), top+3-statusBarRows; got != want {
		t.Errorf("Cursor = %d, want %d (top=%d)", got, want, top)
	}
}

// TestMouse_LeftClickOnStatusBarIgnored guards Y=0 (status bar) as no-op.
func TestMouse_LeftClickOnStatusBarIgnored(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	m, _ = applyKey(m, "j")
	before := m.Cursor()
	m = sendMouse(m, click(0))
	if m.Cursor() != before {
		t.Errorf("Cursor = %d, want %d (Y=0 is status bar)", m.Cursor(), before)
	}
}

// TestMouse_LeftClickOnHintIgnored: clicks at or past status+viewportHeight are
// on the hint line / trailing padding and must not move the cursor.
func TestMouse_LeftClickOnHintIgnored(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	before := m.Cursor()
	// height=20, viewportHeight=18, statusBarRows=1; Y=19 is the hint row.
	m = sendMouse(m, click(19))
	if m.Cursor() != before {
		t.Errorf("Cursor = %d, want %d (Y=19 is hint)", m.Cursor(), before)
	}
}

// TestMouse_LeftClickPastLastRowIgnored: clicking beyond the last rendered row
// (trailing pad after a short file) is a silent no-op rather than snapping to
// the last row.
func TestMouse_LeftClickPastLastRowIgnored(t *testing.T) {
	t.Parallel()
	// Tiny file: 1 hunk with 2 lines → rows = header + hunk header + 2 = 4.
	// viewport=18 so most of the screen is empty pad.
	files := []diffmodel.File{makeFile("a", []byte{' ', ' '})}
	m := setSize(New(files, review.Review{}), 80, 20)
	before := m.Cursor()
	m = sendMouse(m, click(10)) // row 10-1=9, but only 4 rows exist.
	if m.Cursor() != before {
		t.Errorf("Cursor = %d, want %d (click past last row should no-op)", m.Cursor(), before)
	}
}

// TestMouse_LeftClickIgnoredWhileHelpOpen mirrors the wheel-help guard.
func TestMouse_LeftClickIgnoredWhileHelpOpen(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	m, _ = applyKey(m, "?")
	if !m.ShowingHelp() {
		t.Fatal("expected help to be open")
	}
	before := m.Cursor()
	m = sendMouse(m, click(5))
	if m.Cursor() != before {
		t.Errorf("help open: Cursor = %d, want %d", m.Cursor(), before)
	}
}

// TestMouse_LeftClickIgnoredWhileModalOpen mirrors the wheel-modal guard:
// modal Update path consumes the message entirely.
func TestMouse_LeftClickIgnoredWhileModalOpen(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	if m.Modal() == nil {
		t.Fatal("expected comment modal to be open")
	}
	before := m.Cursor()
	m = sendMouse(m, click(5))
	if m.Cursor() != before {
		t.Errorf("modal open: Cursor = %d, want %d", m.Cursor(), before)
	}
}

// TestMouse_LeftClickExtendsSelection: an active range must follow the click,
// just like wheel scrolling and j/k do, so the rendered highlight and the
// persisted Extent stay in sync.
func TestMouse_LeftClickExtendsSelection(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	// Anchor a selection on the first content row.
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	anchor := m.Cursor()
	m, _ = applyKey(m, "r")
	if m.selection == nil {
		t.Fatalf("expected selection after r")
	}
	// Click a few rows down inside the same hunk.
	target := anchor + 3
	clickY := target - m.Top() + statusBarRows
	m = sendMouse(m, click(clickY))
	if m.Cursor() != target {
		t.Fatalf("Cursor = %d, want %d", m.Cursor(), target)
	}
	if m.selection == nil {
		t.Fatalf("selection cleared unexpectedly after click")
	}
	if m.selection.Anchor != anchor || m.selection.Extent != target {
		t.Errorf("Selection = (anchor=%d, extent=%d), want (%d, %d)",
			m.selection.Anchor, m.selection.Extent, anchor, target)
	}
}

// TestMouse_LeftClickInvalidatesStickyToggle: cursor moves via click must drop
// the sticky resolve anchor so the next `x` re-evaluates the open-biased
// default rather than re-toggling the previously-touched comment.
func TestMouse_LeftClickInvalidatesStickyToggle(t *testing.T) {
	t.Parallel()
	m := setSize(New([]diffmodel.File{bigFile()}, review.Review{}), 80, 20)
	m.lastToggledAnchor = "stale"
	m = sendMouse(m, click(3))
	if m.lastToggledAnchor != "" {
		t.Errorf("lastToggledAnchor = %q, want \"\" (click must invalidate)", m.lastToggledAnchor)
	}
}

// TestMouse_LeftClickOnFileHeader: clicking the file header row (the first
// row) leaves the cursor on rowFileHeader. A follow-up `c` should open the
// modal with KindFile, since modal kind comes from the anchor row.
func TestMouse_LeftClickOnFileHeaderThenComment(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	// Move cursor away from row 0 first so the click is a real move.
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	// Click Y=1 → row m.top + 0 = 0, the file header.
	m = sendMouse(m, click(1))
	if m.Cursor() != 0 {
		t.Fatalf("Cursor = %d, want 0 (file header)", m.Cursor())
	}
	m, _ = applyKey(m, "c")
	if m.Modal() == nil {
		t.Fatal("expected modal to open after c on file header")
	}
	if got := m.Modal().anchor.Kind; got != review.KindFile {
		t.Errorf("modal kind = %v, want KindFile", got)
	}
}

// TestMouse_LeftClickSplitMovesCursor mirrors the unified test for split.
func TestMouse_LeftClickSplitMovesCursor(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{bigFile()}
	m := setSize(New(files, review.Review{}), 80, 20)
	m = sendNamedKey(m, tea.KeyTab)
	if m.layout != LayoutSplit {
		t.Fatalf("expected LayoutSplit")
	}
	m = sendMouse(m, click(5))
	want := m.splitTop + 5 - statusBarRows
	if m.splitCursor != want {
		t.Errorf("splitCursor = %d, want %d", m.splitCursor, want)
	}
	// Unified cursor must not move in split mode.
	if m.Cursor() != 0 {
		t.Errorf("unified Cursor mutated in split: %d", m.Cursor())
	}
}

// TestMouse_LeftClickSplitPastLastRowIgnored: clicks past the rendered split
// rows must be a silent no-op, same invariant as unified.
func TestMouse_LeftClickSplitPastLastRowIgnored(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{makeFile("a", []byte{' ', ' '})}
	m := setSize(New(files, review.Review{}), 80, 20)
	m = sendNamedKey(m, tea.KeyTab)
	before := m.splitCursor
	m = sendMouse(m, click(10))
	if m.splitCursor != before {
		t.Errorf("splitCursor = %d, want %d", m.splitCursor, before)
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
