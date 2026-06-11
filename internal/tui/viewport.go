package tui

import (
	"fmt"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
)

// rowKind classifies a flattened diff row so navigation and rendering can tell
// file headers, hunk headers, and content lines apart.
type rowKind byte

const (
	rowFileHeader rowKind = iota
	rowHunkHeader
	rowLine
	rowBinary
)

// row is one rendered line in the flat diff stream consumed by the viewport.
type row struct {
	kind    rowKind
	fileIdx int
	hunkIdx int
	lineIdx int
	text    string
}

func buildRows(files []diffmodel.File) []row {
	var out []row
	for fi, f := range files {
		header := fmt.Sprintf("%s %s", f.Status, f.DisplayPath())
		if f.RenameFrom != "" && f.RenameTo != "" && f.RenameFrom != f.RenameTo {
			header = fmt.Sprintf("%s %s -> %s", f.Status, f.RenameFrom, f.RenameTo)
		}
		out = append(out, row{kind: rowFileHeader, fileIdx: fi, text: header})
		if f.Binary {
			out = append(out, row{kind: rowBinary, fileIdx: fi, text: "[binary]"})
			continue
		}
		for hi, h := range f.Hunks {
			out = append(out, row{
				kind:    rowHunkHeader,
				fileIdx: fi,
				hunkIdx: hi,
				text:    fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s", h.BaseStart, h.BaseLines, h.HeadStart, h.HeadLines, h.Header),
			})
			for li, l := range h.Lines {
				out = append(out, row{
					kind:    rowLine,
					fileIdx: fi,
					hunkIdx: hi,
					lineIdx: li,
					text:    string(l.Prefix) + l.Text,
				})
			}
		}
	}
	return out
}

// viewportHeight returns the number of diff rows that fit on screen, excluding
// the 1-line status bar and 1-line hint.
func (m Model) viewportHeight() int {
	const chrome = 2
	h := m.height - chrome
	if h < 1 {
		return 1
	}
	return h
}

// scrollToCursor adjusts top so cursor sits inside [top, top+height).
func (m *Model) scrollToCursor() {
	h := m.viewportHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	} else if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

// scrollViewportBy moves the unified viewport's top by d rows (positive scrolls
// down, negative scrolls up). Used by mouse-wheel input where decoupling top
// from cursor matches user expectation. The cursor is clamped into the new
// viewport so that subsequent cursor moves (j/k) don't snap top back via
// scrollToCursor, and so cursor-driven actions like `c` don't target an
// off-screen row.
func (m *Model) scrollViewportBy(d int) {
	if len(m.rows) == 0 {
		return
	}
	h := m.viewportHeight()
	maxTop := len(m.rows) - h
	if maxTop < 0 {
		maxTop = 0
	}
	m.top += d
	if m.top < 0 {
		m.top = 0
	}
	if m.top > maxTop {
		m.top = maxTop
	}
	// Keep the cursor inside the visible window. Without this, wheel scrolling
	// leaves the cursor off-screen and the next moveCursorBy → scrollToCursor
	// snaps top back to where the cursor was.
	if m.cursor < m.top {
		m.cursor = m.top
	}
	if bottom := m.top + h - 1; m.cursor > bottom {
		m.cursor = bottom
	}
	if last := len(m.rows) - 1; m.cursor > last {
		m.cursor = last
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	// Range selection follows the cursor in unified mode (keyboard j/k already
	// does this via extendSelection). Without this, `r` → wheel → `c` would
	// save a comment against the pre-wheel Extent because the cursor moved
	// out from under the selection without updating it.
	if m.selection != nil {
		m.extendSelection()
	}
}

func (m *Model) moveCursorBy(d int) {
	if len(m.rows) == 0 {
		return
	}
	prev := m.cursor
	m.cursor += d
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor != prev {
		// Drop the sticky resolve target whenever the user leaves the row.
		// See toggleResolvedAtCursor for why we anchor by row.
		m.invalidateLastToggle()
	}
	m.scrollToCursor()
}

// renderRow turns a flat-stream row into a sanitized, width-clipped display
// string. File / hunk headers and the binary placeholder are emitted with a
// space prefix so they align with content lines (which carry +/-/space
// inside their text).
func renderRow(r row, maxWidth int) string {
	switch r.kind {
	case rowLine:
		if len(r.text) == 0 {
			return renderLine(' ', "", maxWidth)
		}
		return renderLine(r.text[0], r.text[1:], maxWidth)
	default:
		return renderLine(' ', r.text, maxWidth)
	}
}

// lineNumberWidths returns the digit count of the largest BaseLine / HeadLine
// across files. 0 on a side means no line on that side carries a number, so
// the gutter for that side collapses entirely.
func lineNumberWidths(files []diffmodel.File) (base, head int) {
	for _, f := range files {
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if w := digits(l.BaseLine); w > base {
					base = w
				}
				if w := digits(l.HeadLine); w > head {
					head = w
				}
			}
		}
	}
	return
}

func digits(n int) int {
	if n <= 0 {
		return 0
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}

// gutterWidth is the total column count of the line-number gutter, including
// the trailing space separator. 0 when neither side has any line numbers
// (e.g. binary-only file lists).
func gutterWidth(baseW, headW int) int {
	if baseW == 0 && headW == 0 {
		return 0
	}
	w := baseW + headW
	if baseW > 0 && headW > 0 {
		w++ // single-space separator between base and head columns
	}
	w++ // trailing separator before content
	return w
}

// lineNumberGutter renders the per-row base/head number gutter. Non-line rows
// get blanks of the same width so headers and content stay vertically aligned.
func lineNumberGutter(r row, files []diffmodel.File, baseW, headW int) string {
	w := gutterWidth(baseW, headW)
	if w == 0 {
		return ""
	}
	if r.kind != rowLine || r.fileIdx < 0 || r.fileIdx >= len(files) {
		return blanks(w)
	}
	f := files[r.fileIdx]
	if r.hunkIdx < 0 || r.hunkIdx >= len(f.Hunks) {
		return blanks(w)
	}
	h := f.Hunks[r.hunkIdx]
	if r.lineIdx < 0 || r.lineIdx >= len(h.Lines) {
		return blanks(w)
	}
	l := h.Lines[r.lineIdx]
	var out []byte
	if baseW > 0 {
		out = appendNumOrBlank(out, l.BaseLine, baseW)
	}
	if baseW > 0 && headW > 0 {
		out = append(out, ' ')
	}
	if headW > 0 {
		out = appendNumOrBlank(out, l.HeadLine, headW)
	}
	out = append(out, ' ')
	return string(out)
}

func appendNumOrBlank(dst []byte, n, width int) []byte {
	if n <= 0 {
		for i := 0; i < width; i++ {
			dst = append(dst, ' ')
		}
		return dst
	}
	s := itoa(n)
	for i := len(s); i < width; i++ {
		dst = append(dst, ' ')
	}
	return append(dst, s...)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func blanks(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

// jumpFile moves the cursor to the next (dir>0) or previous (dir<0) file
// header. Files include rowFileHeader; we skip past the current file's header
// when searching forward so `n` always advances.
func (m *Model) jumpFile(dir int) {
	if len(m.rows) == 0 {
		return
	}
	if dir > 0 {
		for i := m.cursor + 1; i < len(m.rows); i++ {
			if m.rows[i].kind == rowFileHeader {
				m.cursor = i
				m.top = i
				m.invalidateLastToggle()
				return
			}
		}
		return
	}
	// Backwards: if cursor is already on a header, step to the previous one.
	start := m.cursor
	if m.rows[m.cursor].kind == rowFileHeader {
		start--
	}
	for i := start; i >= 0; i-- {
		if m.rows[i].kind == rowFileHeader {
			m.cursor = i
			m.top = i
			m.lastToggledAnchor = ""
			return
		}
	}
}
