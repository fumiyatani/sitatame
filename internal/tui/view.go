package tui

import (
	"fmt"
	"strings"
)

const cursorMarker = "> "
const cursorPad = "  " // same width as cursorMarker so non-cursor lines align

// mainView renders the visible viewport: status bar + diff window + hint.
// Only rows in [top, top+height) are emitted, so a 10k-line diff still costs
// O(visible-rows) per frame.
func mainView(m Model) string {
	var b strings.Builder
	b.WriteString(statusLine(m))
	b.WriteByte('\n')

	height := m.viewportHeight()
	if len(m.rows) == 0 {
		b.WriteString("(no diff)\n")
		// Pad to keep the hint pinned to the bottom-ish for stable rendering.
		for i := 1; i < height; i++ {
			b.WriteByte('\n')
		}
	} else {
		end := m.top + height
		if end > len(m.rows) {
			end = len(m.rows)
		}
		// Width budget for the row body excludes the 2-col cursor gutter.
		bodyMax := m.width - len(cursorMarker)
		for i := m.top; i < end; i++ {
			switch {
			case i == m.cursor:
				b.WriteString(cursorMarker)
			case m.selection != nil && m.selection.Contains(i):
				b.WriteString("| ")
			default:
				b.WriteString(cursorPad)
			}
			b.WriteString(renderRow(m.rows[i], bodyMax))
			b.WriteByte('\n')
		}
		for i := end - m.top; i < height; i++ {
			b.WriteByte('\n')
		}
	}
	b.WriteString(hintLine())
	return b.String()
}

func statusLine(m Model) string {
	var path string
	binary := false
	if len(m.rows) > 0 && m.cursor < len(m.rows) {
		fi := m.rows[m.cursor].fileIdx
		if fi >= 0 && fi < len(m.Files) {
			path = m.Files[fi].DisplayPath()
			binary = m.Files[fi].Binary
		}
	}
	if path == "" {
		path = "(none)"
	}
	tag := ""
	if binary {
		tag = "  [binary] file-comment only"
	}
	return fmt.Sprintf("sitatame %s  [%d/%d files]  row %d/%d%s",
		path, fileIndexAtCursor(m)+1, len(m.Files), m.cursor+1, len(m.rows), tag)
}

func fileIndexAtCursor(m Model) int {
	if len(m.rows) == 0 || m.cursor >= len(m.rows) {
		return 0
	}
	return m.rows[m.cursor].fileIdx
}

func hintLine() string {
	return "j/k move · n/p file · ? help · q quit"
}
