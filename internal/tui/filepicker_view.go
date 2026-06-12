package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// File picker chrome: title line + 4 lines of border/hint padding. Sized so
// the picker stays compact even on terminals that report tiny heights.
const (
	filePickerChromeRows = 4 // top border, bottom border, hint, spacer
	filePickerMaxWidth   = 80
	filePickerMinHeight  = 3
)

// openFilePicker seeds the modal centered on the file the cursor is currently
// inside. Empty diffs short-circuit — without items the picker would absorb
// keystrokes (j/k/Enter) without doing anything, which is worse than refusing
// to open. Single-file diffs are allowed: the user can still confirm with
// Enter as a no-op, which is consistent with the help screen never lying
// about which keys are bound.
func (m *Model) openFilePicker() {
	if len(m.Files) == 0 {
		return
	}
	cur := fileIndexAtCursor(*m)
	h := m.filePickerHeight()
	m.filePicker = newFilePicker(m.Files, cur, h)
}

// filePickerHeight returns how many item rows we can show inside the picker
// modal. The window height has to swallow the picker chrome (title, border,
// hint, spacer) plus the underlying status / hint lines that mainView normally
// shows; we clamp to at least filePickerMinHeight so a tiny terminal still
// renders something usable.
func (m Model) filePickerHeight() int {
	avail := m.height - filePickerChromeRows
	if avail < filePickerMinHeight {
		avail = filePickerMinHeight
	}
	if avail > len(m.Files) {
		avail = len(m.Files)
	}
	if avail < 1 {
		avail = 1
	}
	return avail
}

// updateFilePicker handles input while the picker is open. The contract is
// intentionally narrow: arrow keys + j/k move the highlight, Enter confirms
// the jump, Esc closes without moving the cursor. Every other key is
// swallowed so the underlying diff bindings don't fire by accident — `q`
// here must not quit the program because the user expects the picker to
// take focus.
func (m Model) updateFilePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Resync the picker's own height to the new window size so the
		// visible window after a resize still tracks the selection.
		if m.filePicker != nil {
			m.filePicker.height = m.filePickerHeight()
			m.filePicker.scrollToIdx()
		}
		// Keep the underlying diff viewport(s) in sync with the new height
		// even though we don't render them while the picker is open. The
		// non-picker WindowSizeMsg path (model.go) calls these on every
		// resize; skipping them here would leave m.top pointing at a row
		// outside [m.top, m.top+viewportHeight()) once the picker closes,
		// so the diff would appear scrolled into a stale region on Esc.
		m.scrollToCursor()
		if m.layout == LayoutSplit {
			m.scrollSplitToCursor()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case KeyDown, "down":
			m.filePicker.moveBy(1)
			return m, nil
		case KeyUp, "up":
			m.filePicker.moveBy(-1)
			return m, nil
		case "enter":
			m.confirmFilePicker()
			return m, nil
		case KeyEsc:
			m.cancelFilePicker()
			return m, nil
		}
	}
	return m, nil
}

// confirmFilePicker jumps the cursor to the selected file's header row, then
// closes the picker. We mirror jumpFile's "land on the rowFileHeader and pin
// top to it" contract so Tab and other layout switches see the same anchor
// state the user expects from n/p navigation.
func (m *Model) confirmFilePicker() {
	if m.filePicker == nil {
		return
	}
	sel := m.filePicker.selected()
	m.filePicker = nil
	idx := fileHeaderRowIndex(m.rows, sel.FileIdx)
	if idx < 0 {
		// Defensive: the selected FileIdx came from m.Files, which is the
		// same slice buildRows consumed, so a missing header would mean a
		// row-stream/file-list mismatch. We choose to just close the picker
		// rather than panic — the user would otherwise be stuck inside the
		// modal with no way out except q.
		return
	}
	m.cursor = idx
	m.top = idx
	// The sticky resolve target is row-local; jumping into a different file
	// invalidates it for the same reason n/p does (toggleResolvedAtCursor
	// godoc).
	m.invalidateLastToggle()
	// Range selection is also row-local — anchored in the previous file,
	// it would silently follow the cursor into the new file's row stream
	// and produce a comment whose Anchor.Path no longer matches the
	// Selection.FileIdx. Clear it explicitly to keep the post-jump state
	// indistinguishable from a fresh `n`-driven file change.
	m.clearSelection()
}

func (m *Model) cancelFilePicker() {
	m.filePicker = nil
}

// fileHeaderRowIndex returns the row stream index of the rowFileHeader for the
// given fileIdx, or -1 if not found. Linear scan: row counts are bounded by
// the diff size, and this only runs on Enter — not in a hot path.
func fileHeaderRowIndex(rows []row, fileIdx int) int {
	for i, r := range rows {
		if r.kind == rowFileHeader && r.fileIdx == fileIdx {
			return i
		}
	}
	return -1
}

// filePickerView renders the modal. We deliberately replace the full screen
// (rather than overlaying on top of the diff) because:
//   - the underlying diff would compete for the same cells, requiring
//     transparent compositing the rest of the TUI doesn't otherwise need;
//   - the user can still glean the current file's context from the title bar
//     above the items, which carries the selected entry.
//
// Layout:
//
//	┌─ Files (N) ──────────────────────┐
//	│ > M cmd/root.go            +12 -3│
//	│   M internal/tui/model.go  +28 -5│
//	│   M README.md              +1 -0 │
//	└─ j/k or up/down · Enter jump · Esc close ─┘
func filePickerView(m Model) string {
	fp := m.filePicker
	if fp == nil {
		return ""
	}
	width := filePickerWidth(m.width)
	var b strings.Builder
	// Title row: "Files (N)" + border padding.
	title := fmt.Sprintf(" Files (%d) ", len(fp.items))
	b.WriteString(renderBorderTop(title, width))
	b.WriteByte('\n')

	start, end := fp.visibleRange()
	for i := start; i < end; i++ {
		b.WriteString(renderPickerRow(fp.items[i], i == fp.idx, width))
		b.WriteByte('\n')
	}
	// Pad remaining viewport rows so the bottom border stays anchored even
	// when the diff has fewer files than the picker can hold.
	for i := end - start; i < fp.height; i++ {
		b.WriteString(renderPickerBlank(width))
		b.WriteByte('\n')
	}

	hint := pickerHintForWidth(width)
	b.WriteString(renderBorderBottom(hint, width))
	return b.String()
}

// pickerHintForWidth picks the longest hint variant that fits inside the modal
// chrome at the given width. The bottom border is "+-<hint><dashes>+", so the
// hint must satisfy `2 + hintW + 1 <= width` (i.e. hintW <= width - 3) to leave
// room for the two corner '+' and at least one trailing dash. When even the
// shortest legend would overflow we return "" so the bottom border degrades to
// plain "+---...+" rather than wrapping past the modal width.
//
// Variants are ordered longest → shortest; each carries a leading + trailing
// space because renderBorderBottom embeds the hint verbatim between dashes and
// the spaces give the text visual breathing room against the corner pieces.
func pickerHintForWidth(width int) string {
	variants := []string{
		" j/k or up/down select - Enter jump - Esc close ",
		" j/k select - Enter - Esc ",
		" jk Ent Esc ",
	}
	budget := width - 3
	for _, v := range variants {
		if ColWidth(v) <= budget {
			return v
		}
	}
	return ""
}

// filePickerWidth picks a friendly column count for the modal. We cap at
// filePickerMaxWidth so long paths don't make the picker stretch the whole
// terminal width, and at the window's actual width when that's smaller so
// the right border doesn't fall off-screen.
//
// On very narrow terminals (windowWidth < ~22) we deliberately let the modal
// shrink below the "desired minimum" of 20 columns. The window-width cap is a
// hard ceiling: returning anything larger than windowWidth would make the
// border lines overflow and wrap, collapsing the modal layout. The trade-off
// is that the picker text may be heavily clipped on tiny terminals — that is
// strictly preferable to a wrapped border which destroys the whole frame.
// We never return less than 1 because builders downstream assume at least one
// column for the inner area; the caller is expected to refuse to render at
// windowWidth <= 0 in practice.
func filePickerWidth(windowWidth int) int {
	const desiredMin = 20
	w := desiredMin
	if windowWidth-2 > w {
		w = min(windowWidth-2, filePickerMaxWidth)
	}
	if w > windowWidth && windowWidth > 0 {
		w = windowWidth
	}
	if w < 1 {
		w = 1
	}
	return w
}

// renderBorderTop returns "+- title --------+", padded to width. Plain ASCII
// to keep the snapshot tests deterministic across terminal locales.
//
// On narrow widths (< chrome + title) we clip the whole line to `width` so
// the rendered modal never overflows the terminal. The trade-off is a chopped
// title — pickerHintForWidth already handles the bottom variant; here we just
// trim, because shrinking "Files (N)" mid-token would mislead the user about
// which file count they're seeing.
func renderBorderTop(title string, width int) string {
	if width <= 0 {
		return ""
	}
	titleW := ColWidth(title)
	dash := width - 2 - titleW
	if dash < 0 {
		dash = 0
	}
	line := "+-" + title + strings.Repeat("-", dash) + "+"
	if ColWidth(line) > width {
		line = clipForBudget(line, width)
	}
	return line
}

func renderBorderBottom(hint string, width int) string {
	if width <= 0 {
		return ""
	}
	hintW := ColWidth(hint)
	dash := width - 2 - hintW
	if dash < 0 {
		dash = 0
	}
	line := "+-" + hint + strings.Repeat("-", dash) + "+"
	if ColWidth(line) > width {
		line = clipForBudget(line, width)
	}
	return line
}

func renderPickerBlank(width int) string {
	if width <= 0 {
		return ""
	}
	inner := width - 2
	if inner < 0 {
		inner = 0
	}
	line := "|" + strings.Repeat(" ", inner) + "|"
	if ColWidth(line) > width {
		line = clipForBudget(line, width)
	}
	return line
}

// renderPickerRow lays out one item line:
//
//	"| > M <path>                +A -D |"
//
// The path is truncated with a trailing ellipsis if needed so the +A -D
// counts always survive at the right edge. Centralised here so the
// truncation budget stays consistent with the surrounding chrome math.
func renderPickerRow(it filePickItem, selected bool, width int) string {
	if width <= 0 {
		return ""
	}
	marker := "  "
	if selected {
		marker = "> "
	}
	statusCol := it.Status + " "
	counts := fmt.Sprintf("+%d -%d", it.Adds, it.Dels)
	// inner width = total - "|" - "|" - "marker" - " status " - " " - counts
	inner := width - 2
	if inner < 0 {
		inner = 0
	}
	// Reserve columns for marker (2), status (2), trailing space + counts.
	overhead := ColWidth(marker) + ColWidth(statusCol) + 1 + ColWidth(counts)
	pathBudget := inner - overhead
	if pathBudget < 1 {
		pathBudget = 1
	}
	path := clipForBudget(it.Path, pathBudget)
	// Pad the path to fill the budget so the counts column aligns even when
	// paths differ in length.
	pad := pathBudget - ColWidth(path)
	if pad < 0 {
		pad = 0
	}
	body := marker + statusCol + path + strings.Repeat(" ", pad) + " " + counts
	bodyW := ColWidth(body)
	if bodyW > inner {
		// Defensive truncation in case ColWidth disagreed with our budget
		// math (e.g. multi-byte counts). Without this, the body would
		// overflow past the right border on narrow windows.
		body = clipForBudget(body, inner)
	} else if bodyW < inner {
		body = body + strings.Repeat(" ", inner-bodyW)
	}
	line := "|" + body + "|"
	// Final guard: at widths < 2 there's no room for both borders, so trim
	// the assembled line to the requested column count. Without this the
	// "||" sentinel would overflow a 1-column terminal.
	if ColWidth(line) > width {
		line = clipForBudget(line, width)
	}
	return line
}

// clipForBudget truncates s to budget display columns, appending an ellipsis
// when truncation occurred. Mirrors writeBody's ellipsis policy in render.go
// but doesn't drop control bytes — picker text is sourced from File.Path /
// File.Status, which are already sanitized by the diffmodel layer.
func clipForBudget(s string, budget int) string {
	if budget <= 0 {
		return ""
	}
	if ColWidth(s) <= budget {
		return s
	}
	if budget == 1 {
		return "…"
	}
	cap := budget - 1
	used := 0
	var b strings.Builder
	for _, r := range s {
		w := widthCond.RuneWidth(r)
		if used+w > cap {
			break
		}
		b.WriteRune(r)
		used += w
	}
	b.WriteRune('…')
	return b.String()
}
