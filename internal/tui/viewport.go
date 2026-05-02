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

func (m *Model) moveCursorBy(d int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor += d
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
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
				m.scrollToCursor()
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
			m.scrollToCursor()
			return
		}
	}
}
