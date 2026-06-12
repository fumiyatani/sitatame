package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// pickerFile produces a minimal File with one hunk carrying `adds` insertions
// and `dels` deletions. Named to avoid colliding with the package-wide
// makeFile helper in viewport_test.go, which has a different signature. The
// picker tests below need to drive the +/- count logic without going through
// the full numberedFile fixture, which would over-constrain the test against
// line-number details we don't care about here.
func pickerFile(path string, status diffmodel.Status, adds, dels int) diffmodel.File {
	lines := make([]diffmodel.Line, 0, adds+dels+1)
	lines = append(lines, diffmodel.Line{Prefix: ' ', Text: "ctx"})
	for i := 0; i < adds; i++ {
		lines = append(lines, diffmodel.Line{Prefix: '+', Text: fmt.Sprintf("add%d", i)})
	}
	for i := 0; i < dels; i++ {
		lines = append(lines, diffmodel.Line{Prefix: '-', Text: fmt.Sprintf("del%d", i)})
	}
	f := diffmodel.File{
		Status:   status,
		PrePath:  path,
		PostPath: path,
		Hunks: []diffmodel.Hunk{{
			BaseStart: 1, BaseLines: dels + 1, HeadStart: 1, HeadLines: adds + 1,
			Lines: lines,
		}},
	}
	diffmodel.AssignLineNumbers(&f.Hunks[0])
	return f
}

func TestFilePicker_InitialIdxMatchesCurrentFileIdx(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 0),
		pickerFile("c.go", diffmodel.StatusModified, 0, 1),
	}
	fp := newFilePicker(files, 1, 10)
	if got := fp.selected().Path; got != "b.go" {
		t.Errorf("initial selected path = %q, want b.go", got)
	}
	if fp.idx != 1 {
		t.Errorf("idx = %d, want 1", fp.idx)
	}
}

func TestFilePicker_InitialIdxClampsOutOfRange(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{pickerFile("a.go", diffmodel.StatusModified, 1, 0)}
	fp := newFilePicker(files, 99, 10)
	if fp.idx != 0 {
		t.Errorf("out-of-range currentFileIdx should clamp to 0, got %d", fp.idx)
	}
	fp = newFilePicker(files, -3, 10)
	if fp.idx != 0 {
		t.Errorf("negative currentFileIdx should clamp to 0, got %d", fp.idx)
	}
}

func TestFilePicker_MoveByClampsEnds(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 0),
		pickerFile("c.go", diffmodel.StatusModified, 0, 1),
	}
	fp := newFilePicker(files, 0, 10)
	fp.moveBy(-1) // clamp top
	if fp.idx != 0 {
		t.Errorf("moveBy(-1) at top: idx = %d, want 0", fp.idx)
	}
	fp.moveBy(1)
	if fp.idx != 1 {
		t.Errorf("moveBy(1): idx = %d, want 1", fp.idx)
	}
	fp.moveBy(99) // clamp bottom
	if fp.idx != 2 {
		t.Errorf("moveBy past end: idx = %d, want 2", fp.idx)
	}
}

func TestFilePicker_AddsDelsCount(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 12, 3),
	}
	fp := newFilePicker(files, 0, 10)
	it := fp.selected()
	if it.Adds != 12 || it.Dels != 3 {
		t.Errorf("adds/dels = (%d, %d), want (12, 3)", it.Adds, it.Dels)
	}
	if it.Status != "M" {
		t.Errorf("status = %q, want M", it.Status)
	}
}

func TestFilePicker_ScrollFollowsSelection(t *testing.T) {
	t.Parallel()
	var files []diffmodel.File
	for i := 0; i < 20; i++ {
		files = append(files, pickerFile(fmt.Sprintf("f%d.go", i), diffmodel.StatusModified, 1, 0))
	}
	fp := newFilePicker(files, 0, 5)
	// Walk past the bottom of the initial viewport. After moveBy(4) the
	// selection is at the last visible row (idx=4, top=0). The 5th moveBy
	// must scroll: idx=5 should imply top>=1 so the selection stays inside
	// [top, top+5).
	for i := 0; i < 5; i++ {
		fp.moveBy(1)
	}
	if fp.idx != 5 {
		t.Fatalf("idx = %d, want 5", fp.idx)
	}
	if fp.top < 1 {
		t.Errorf("top did not advance to keep selection visible: top=%d idx=%d", fp.top, fp.idx)
	}
	if fp.idx < fp.top || fp.idx >= fp.top+fp.height {
		t.Errorf("selection out of viewport: idx=%d top=%d height=%d", fp.idx, fp.top, fp.height)
	}
	// Now scroll back up; top must follow.
	for i := 0; i < 10; i++ {
		fp.moveBy(-1)
	}
	if fp.idx != 0 || fp.top != 0 {
		t.Errorf("after walk-back: idx=%d top=%d, want 0,0", fp.idx, fp.top)
	}
}

func TestFilePicker_SingleFileDoesNotPanic(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{pickerFile("only.go", diffmodel.StatusModified, 1, 0)}
	fp := newFilePicker(files, 0, 5)
	fp.moveBy(1)  // no-op at end
	fp.moveBy(-1) // no-op at top
	if got := fp.selected().Path; got != "only.go" {
		t.Errorf("selected = %q, want only.go", got)
	}
}

func TestFilePicker_EmptyFilesNoOp(t *testing.T) {
	t.Parallel()
	fp := newFilePicker(nil, 0, 5)
	if len(fp.items) != 0 {
		t.Errorf("expected zero items, got %d", len(fp.items))
	}
	fp.moveBy(1)
	fp.moveBy(-1)
	if got := fp.selected(); got.Path != "" || got.FileIdx != 0 {
		t.Errorf("selected on empty = %+v, want zero", got)
	}
}

func TestFilePicker_HundredFilesScrollable(t *testing.T) {
	t.Parallel()
	var files []diffmodel.File
	for i := 0; i < 120; i++ {
		files = append(files, pickerFile(fmt.Sprintf("f%03d.go", i), diffmodel.StatusModified, i%5, i%3))
	}
	fp := newFilePicker(files, 0, 10)
	if len(fp.items) != 120 {
		t.Fatalf("items = %d, want 120", len(fp.items))
	}
	// Jump to the bottom and verify the picker viewport tracks the selection.
	fp.moveBy(200)
	if fp.idx != 119 {
		t.Errorf("idx after large delta = %d, want 119", fp.idx)
	}
	if fp.idx < fp.top || fp.idx >= fp.top+fp.height {
		t.Errorf("bottom selection out of viewport: idx=%d top=%d height=%d", fp.idx, fp.top, fp.height)
	}
	// Render: the visible row count must not exceed `height`, and the
	// selection must appear in the rendered output.
	m := New(files, review.Review{})
	m = setSize(m, 100, 30)
	m.filePicker = fp
	v := m.View()
	if !strings.Contains(v, "f119.go") {
		t.Errorf("rendered view missing selected entry f119.go:\n%s", v)
	}
	if strings.Contains(v, "f000.go") {
		t.Errorf("rendered view should not show f000.go (off-screen): \n%s", v)
	}
}

// TestModel_FOpensFilePicker pins the f keybinding: pressing f opens the
// modal initially focused on the file under the cursor.
func TestModel_FOpensFilePicker(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 1),
	}
	m := New(files, review.Review{})
	m = setSize(m, 80, 24)
	if m.FilePicker() != nil {
		t.Fatal("file picker should start closed")
	}
	m = sendKey(m, "f")
	fp := m.FilePicker()
	if fp == nil {
		t.Fatal("f should open file picker")
	}
	if got := fp.selected().Path; got != "a.go" {
		t.Errorf("initial selection = %q, want a.go", got)
	}
	v := m.View()
	if !strings.Contains(v, "Files (2)") {
		t.Errorf("view missing Files (2) title:\n%s", v)
	}
}

// TestModel_EnterJumpsToSelectedFile is the happy path: open picker, move
// down, Enter — cursor lands on the second file's header row and top pins to
// it (mirroring jumpFile's behavior).
func TestModel_EnterJumpsToSelectedFile(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 1),
	}
	m := New(files, review.Review{})
	m = setSize(m, 80, 24)
	m = sendKey(m, "f")
	m = sendKey(m, "j")
	if got := m.FilePicker().selected().Path; got != "b.go" {
		t.Fatalf("after j: selected = %q, want b.go", got)
	}
	m = sendNamedKey(m, tea.KeyEnter)
	if m.FilePicker() != nil {
		t.Errorf("Enter should close picker")
	}
	// b.go's header row index = (header + hunk + 1 ctx + 1 add) + 0 = 4
	wantRow := fileHeaderRowIndex(m.rows, 1)
	if wantRow < 0 {
		t.Fatal("setup: b.go's header row not found")
	}
	if m.Cursor() != wantRow {
		t.Errorf("Cursor = %d, want %d (b.go header)", m.Cursor(), wantRow)
	}
	if m.Top() != wantRow {
		t.Errorf("Top = %d, want %d", m.Top(), wantRow)
	}
}

// TestModel_EnterArrowDownAlsoMoves verifies the picker accepts the literal
// arrow-down key as well as j, matching the UI contract documented in help.
func TestModel_PickerArrowKeys(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 1),
		pickerFile("c.go", diffmodel.StatusModified, 0, 1),
	}
	m := New(files, review.Review{})
	m = setSize(m, 80, 24)
	m = sendKey(m, "f")
	m = sendNamedKey(m, tea.KeyDown)
	m = sendNamedKey(m, tea.KeyDown)
	if got := m.FilePicker().selected().Path; got != "c.go" {
		t.Errorf("after 2x down: selected = %q, want c.go", got)
	}
	m = sendNamedKey(m, tea.KeyUp)
	if got := m.FilePicker().selected().Path; got != "b.go" {
		t.Errorf("after up: selected = %q, want b.go", got)
	}
}

// TestModel_EscClosesPickerWithoutMovingCursor pins the cancel contract.
func TestModel_EscClosesPickerWithoutMovingCursor(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 1),
	}
	m := New(files, review.Review{})
	m = setSize(m, 80, 24)
	// Walk into file 0's first content line so we can verify cursor doesn't
	// move on Esc.
	m = sendKey(m, "j")
	m = sendKey(m, "j")
	before := m.Cursor()
	m = sendKey(m, "f")
	m = sendKey(m, "j") // move selection to b.go
	m = sendNamedKey(m, tea.KeyEsc)
	if m.FilePicker() != nil {
		t.Errorf("Esc should close picker")
	}
	if m.Cursor() != before {
		t.Errorf("Esc must not move cursor: before=%d after=%d", before, m.Cursor())
	}
}

// TestModel_FInSplitModeShowsPreviewOnlyMsg matches the existing guard pattern
// for c/r/x: f is suppressed in split mode with the preview-only banner.
func TestModel_FInSplitModeShowsPreviewOnlyMsg(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 1),
	}
	m := New(files, review.Review{})
	m = setSize(m, 80, 24)
	m = sendNamedKey(m, tea.KeyTab) // into split
	m = sendKey(m, "f")
	if m.FilePicker() != nil {
		t.Errorf("f in split mode should not open picker")
	}
	if !strings.Contains(m.View(), "split is preview-only") {
		t.Errorf("expected preview-only banner, view:\n%s", m.View())
	}
}

// TestModel_FIgnoredWhileTextareaModalOpen — when the comment modal owns the
// keyboard, f must reach the textarea (and be typed) rather than open a
// second modal on top.
func TestModel_FIgnoredWhileTextareaModalOpen(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := New(files, review.Review{})
	m = setSize(m, 80, 24)
	// Open file-kind modal at file header (row 0).
	m, _ = applyKey(m, "c")
	if m.Modal() == nil {
		t.Fatal("modal precondition failed")
	}
	m = sendKey(m, "f")
	if m.FilePicker() != nil {
		t.Errorf("f inside comment modal must not open picker")
	}
	if m.Modal() == nil {
		t.Errorf("f inside comment modal must not close modal")
	}
	if got := m.Modal().Body(); !strings.Contains(got, "f") {
		t.Errorf("f should have been typed into the textarea, body=%q", got)
	}
}

// TestModel_FIgnoredWhileHelpOpen — `?` then `f`: the help view absorbs `?`
// only, and any subsequent key should also keep the diff bindings disabled
// since the help overlay is a modal too. The pre-existing TestModel_HelpToggle
// covers Esc closing help; here we confirm that the picker stays closed
// while help is showing.
func TestModel_FIgnoredWhileHelpOpen(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 1),
	}
	m := New(files, review.Review{})
	m = setSize(m, 80, 24)
	m = sendKey(m, "?")
	if !m.ShowingHelp() {
		t.Fatal("help precondition failed")
	}
	m = sendKey(m, "f")
	if m.FilePicker() != nil {
		t.Errorf("f while help open must not open picker")
	}
	if !m.ShowingHelp() {
		t.Errorf("f must not close help")
	}
}

// TestScenario_FilePickerJumpFlow is the end-to-end teatest scenario:
// f to open, j to move down, Enter to jump. Final cursor must land on
// the second file's header row.
func TestScenario_FilePickerJumpFlow(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 1),
	}
	// Pre-compute the expected cursor index so the assertion is explicit.
	probe := New(files, review.Review{})
	want := fileHeaderRowIndex(probe.rows, 1)
	runScenario(t, Scenario{
		Name:  "file_picker_jump_flow",
		Files: files,
		Steps: []Step{
			{
				SendKey:                "f",
				RequirePostEventOutput: true,
				Expect:                 Expectation{ViewContains: []string{"Files (2)"}},
			},
			{SendKey: "j"},
			{
				SendKey: "enter",
				Expect: Expectation{
					Cursor: intPtr(want),
					Top:    intPtr(want),
				},
			},
		},
	})
}

// TestFilePicker_ResizeMaintainsCursorVisibility guards the picker-open
// WindowSizeMsg path: the underlying diff viewport must follow the new height
// even though we don't render it. Without scrollToCursor() on this path, m.top
// keeps pointing at the pre-resize row range and the cursor falls outside
// [m.top, m.top+viewportHeight()) once the picker closes — the diff would then
// appear scrolled into a stale region on Esc.
func TestFilePicker_ResizeMaintainsCursorVisibility(t *testing.T) {
	t.Parallel()
	// Many files so the cursor can sit far past the row count a tiny
	// viewport can hold.
	var files []diffmodel.File
	for i := 0; i < 30; i++ {
		files = append(files, pickerFile(fmt.Sprintf("f%02d.go", i), diffmodel.StatusModified, 1, 0))
	}
	m := New(files, review.Review{})
	m = setSize(m, 80, 24)
	// Walk the cursor deep into the row stream so a later shrink will force
	// scrollToCursor() to update m.top.
	for i := 0; i < 40; i++ {
		m = sendKey(m, "j")
	}
	cursorBefore := m.Cursor()
	// Open the picker, then shrink the window. Picker absorbs the resize;
	// underlying diff invariants must still be satisfied.
	m = sendKey(m, "f")
	if m.FilePicker() == nil {
		t.Fatal("precondition: picker should be open")
	}
	m = setSize(m, 80, 8)
	// Close the picker. After Esc the cursor must remain inside the visible
	// viewport for the new (smaller) height.
	m = sendNamedKey(m, tea.KeyEsc)
	if m.FilePicker() != nil {
		t.Fatal("Esc should close picker")
	}
	if m.Cursor() != cursorBefore {
		t.Errorf("Esc moved cursor: before=%d after=%d", cursorBefore, m.Cursor())
	}
	h := m.viewportHeight()
	if m.Cursor() < m.Top() || m.Cursor() >= m.Top()+h {
		t.Errorf("cursor out of viewport after resize-while-picker-open: cursor=%d top=%d height=%d",
			m.Cursor(), m.Top(), h)
	}
}

// TestFilePickerView_HintFitsNarrowWidth pins the bottom-border width
// invariant: every rendered line of the picker view must fit inside the modal
// width when the terminal itself is narrow. Without the hint-variant fallback,
// the full legend overflows past the right border and wraps, collapsing the
// modal layout.
func TestFilePickerView_HintFitsNarrowWidth(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 1),
	}
	m := New(files, review.Review{})
	m = setSize(m, 30, 12)
	m = sendKey(m, "f")
	if m.FilePicker() == nil {
		t.Fatal("precondition: picker should be open")
	}
	v := m.View()
	for i, line := range strings.Split(v, "\n") {
		if w := ColWidth(line); w > 30 {
			t.Errorf("line %d width=%d exceeds 30: %q", i, w, line)
		}
	}
}

// TestFilePickerView_HintFallsbackForVerySmall guarantees the hint helper
// degrades gracefully — at very small widths no variant fits, so the helper
// must return "" rather than truncate mid-token (which would produce noisy
// half-words inside the border).
func TestFilePickerView_HintFallsbackForVerySmall(t *testing.T) {
	t.Parallel()
	// width=10: modal budget = width-3 = 7. Shortest variant " jk Ent Esc "
	// is 12 columns, so the helper must fall through to "".
	if got := pickerHintForWidth(10); got != "" {
		t.Errorf("very small width: got %q, want empty", got)
	}
	// Sanity: at a comfortable width we pick the full legend.
	full := " j/k or up/down select - Enter jump - Esc close "
	if got := pickerHintForWidth(80); got != full {
		t.Errorf("wide width: got %q, want full legend", got)
	}
	// At a medium width, the shorter variant should win.
	medium := " j/k select - Enter - Esc "
	if got := pickerHintForWidth(30); got != medium {
		t.Errorf("medium width: got %q, want medium legend", got)
	}
}
