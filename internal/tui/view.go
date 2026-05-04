package tui

import (
	"fmt"
	"strings"
)

const cursorMarker = "> "
const cursorPad = "  " // same width as cursorMarker so non-cursor lines align

// ANSI SGR sequences for diff line tinting. Stripped by goldenSnapshot before
// snapshot compare, so this stays decorative — never load-bearing for tests.
const (
	ansiGreen = "\x1b[32m"
	ansiRed   = "\x1b[31m"
	ansiReset = "\x1b[0m"
)

// colorizeRow tints rendered content lines: green for additions, red for
// deletions. Headers, context, and binary placeholders pass through untouched.
func colorizeRow(r row, rendered string) string {
	if r.kind != rowLine || len(rendered) == 0 {
		return rendered
	}
	switch rendered[0] {
	case '+':
		return ansiGreen + rendered + ansiReset
	case '-':
		return ansiRed + rendered + ansiReset
	default:
		return rendered
	}
}

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
		// Width budget for the row body excludes the 2-col cursor gutter, the
		// 1-col overlay marker gutter, and the line-number gutter (which is 0
		// when no side has any line numbers).
		gw := gutterWidth(m.lnBaseW, m.lnHeadW)
		bodyMax := m.width - len(cursorMarker) - 1 - gw
		for i := m.top; i < end; i++ {
			switch {
			case i == m.cursor:
				b.WriteString(cursorMarker)
			case m.selection != nil && m.selection.Contains(i):
				b.WriteString("| ")
			default:
				b.WriteString(cursorPad)
			}
			b.WriteString(overlayMarker(m.overlay[i], m.Review.Comments))
			b.WriteString(lineNumberGutter(m.rows[i], m.Files, m.lnBaseW, m.lnHeadW))
			b.WriteString(colorizeRow(m.rows[i], renderRow(m.rows[i], bodyMax)))
			b.WriteByte('\n')
		}
		for i := end - m.top; i < height; i++ {
			b.WriteByte('\n')
		}
	}
	b.WriteString(hintLine(m))
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

// modeTagRange mirrors Vim's `-- VISUAL --` so range mode is visible at a
// glance. Only emitted while a selection is active; otherwise the hint stays
// flush-left untouched.
const modeTagRange = "-- RANGE --"

func hintLine(m Model) string {
	left := "j/k move · n/p file · ? help · q quit"
	if m.selection == nil {
		return left
	}
	leftW := ColWidth(left)
	rightW := ColWidth(modeTagRange)
	if m.width <= 0 || leftW+1+rightW > m.width {
		return left + " " + modeTagRange
	}
	return left + strings.Repeat(" ", m.width-leftW-rightW) + modeTagRange
}
