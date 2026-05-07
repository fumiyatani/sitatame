package tui

import (
	"strings"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

const splitColumnSep = " │ "

// splitOverlayEntry collects the comment indices that should render a marker
// on each side of a split row. A comment may show on only one side (line /
// range comments respect anchor.Side) or both (file / hunk / binary headers
// span both columns).
type splitOverlayEntry struct {
	base []int
	head []int
}

// buildSplitOverlay translates the unified overlay into per-side entries.
// Header rows (file / hunk / binary) inherit comments on both sides so the
// marker is visible regardless of which column the eye lands on.
func buildSplitOverlay(srs []splitRow, comments []review.Comment, unified map[int][]int) map[int]splitOverlayEntry {
	out := map[int]splitOverlayEntry{}
	for i, sr := range srs {
		entry := splitOverlayEntry{
			base: collectSideComments(unified, comments, sr.base, sr.kind, review.SideBase),
			head: collectSideComments(unified, comments, sr.head, sr.kind, review.SideHead),
		}
		if len(entry.base) > 0 || len(entry.head) > 0 {
			out[i] = entry
		}
	}
	return out
}

func collectSideComments(unified map[int][]int, comments []review.Comment, unifiedIdx int, kind rowKind, side review.Side) []int {
	if unifiedIdx < 0 {
		return nil
	}
	var out []int
	for _, ci := range unified[unifiedIdx] {
		if ci < 0 || ci >= len(comments) {
			continue
		}
		c := comments[ci]
		if kind != rowLine {
			// Headers carry file-level / hunk-level comments on both columns.
			out = append(out, ci)
			continue
		}
		cs := c.Side
		if cs == "" {
			cs = review.SideHead
		}
		if cs == side {
			out = append(out, ci)
		}
	}
	return out
}

// mainViewSplit renders the side-by-side viewport. The status bar and hint
// are rebuilt from the unified `view.go` helpers, which read m.layout to
// emit the right tag — no separate status helpers needed.
func mainViewSplit(m Model) string {
	var b strings.Builder
	b.WriteString(statusLine(m))
	b.WriteByte('\n')

	height := m.viewportHeight()
	if len(m.splitRows) == 0 {
		b.WriteString("(no diff)\n")
		for i := 1; i < height; i++ {
			b.WriteByte('\n')
		}
		b.WriteString(hintLine(m))
		return b.String()
	}

	end := m.splitTop + height
	if end > len(m.splitRows) {
		end = len(m.splitRows)
	}

	contentW := m.width - len(cursorMarker) - ColWidth(splitColumnSep)
	if contentW < 0 {
		contentW = 0
	}
	sideW := contentW / 2

	for i := m.splitTop; i < end; i++ {
		sr := m.splitRows[i]
		if i == m.splitCursor {
			b.WriteString(cursorMarker)
		} else {
			b.WriteString(cursorPad)
		}
		switch sr.kind {
		case rowFileHeader, rowHunkHeader, rowBinary:
			b.WriteString(splitFullWidthRow(m, sr, m.splitOverlay[i], m.width-len(cursorMarker)))
		default:
			entry := m.splitOverlay[i]
			left := splitColumnContent(m, sr.base, review.SideBase, entry.base, sideW)
			right := splitColumnContent(m, sr.head, review.SideHead, entry.head, sideW)
			b.WriteString(left)
			b.WriteString(splitColumnSep)
			b.WriteString(right)
		}
		b.WriteByte('\n')
	}
	for i := end - m.splitTop; i < height; i++ {
		b.WriteByte('\n')
	}
	b.WriteString(hintLine(m))
	return b.String()
}

// splitFullWidthRow renders headers and binary placeholders across both
// columns. The underlying row is taken from whichever side has a valid
// reference; for headers / binary, both sides point at the same unified row.
func splitFullWidthRow(m Model, sr splitRow, entry splitOverlayEntry, width int) string {
	idx := sr.head
	if idx < 0 {
		idx = sr.base
	}
	if idx < 0 {
		return blanks(width)
	}
	r := m.rows[idx]
	combined := append([]int{}, entry.base...)
	combined = append(combined, entry.head...)
	marker := overlayMarker(combined, m.Review.Comments)
	bodyW := width - 1
	if bodyW < 0 {
		bodyW = 0
	}
	body := colorizeRow(r, renderRow(r, bodyW))
	if hasComment(combined) {
		marker = applyCommentHighlight(marker)
		body = applyCommentHighlight(body)
	}
	return padToCols(marker+body, width)
}

// splitColumnContent renders one side (base or head) of a split line row.
// Empty side (-1) becomes blanks so the column separator stays aligned.
func splitColumnContent(m Model, unifiedIdx int, side review.Side, commentIdxs []int, width int) string {
	if width <= 0 {
		return ""
	}
	if unifiedIdx < 0 {
		return blanks(width)
	}
	r := m.rows[unifiedIdx]
	marker := overlayMarker(commentIdxs, m.Review.Comments)
	gutter := splitSideGutter(r, m.Files, side, m.lnBaseW, m.lnHeadW)
	rest := width - 1 - ColWidth(gutter)
	if rest < 0 {
		rest = 0
	}
	body := colorizeRow(r, renderRow(r, rest))
	if hasComment(commentIdxs) {
		if r.kind == rowLine && ColWidth(gutter) > 0 {
			gutter = applyCommentHighlight(gutter)
		} else {
			marker = applyCommentHighlight(marker)
			body = applyCommentHighlight(body)
		}
	}
	return padToCols(marker+gutter+body, width)
}

// splitSideGutter is the per-side line-number gutter — half of the unified
// gutter, since each column shows only one side's number.
func splitSideGutter(r row, files []diffmodel.File, side review.Side, baseW, headW int) string {
	w := headW
	if side == review.SideBase {
		w = baseW
	}
	if w == 0 {
		return ""
	}
	gw := w + 1
	if r.kind != rowLine || r.fileIdx < 0 || r.fileIdx >= len(files) {
		return blanks(gw)
	}
	f := files[r.fileIdx]
	if r.hunkIdx < 0 || r.hunkIdx >= len(f.Hunks) {
		return blanks(gw)
	}
	h := f.Hunks[r.hunkIdx]
	if r.lineIdx < 0 || r.lineIdx >= len(h.Lines) {
		return blanks(gw)
	}
	l := h.Lines[r.lineIdx]
	n := l.HeadLine
	if side == review.SideBase {
		n = l.BaseLine
	}
	out := appendNumOrBlank(nil, n, w)
	out = append(out, ' ')
	return string(out)
}

// padToCols right-pads s with spaces to `cols` display columns. ANSI escapes
// are stripped before measuring so coloring doesn't inflate the width.
func padToCols(s string, cols int) string {
	if cols <= 0 {
		return ""
	}
	w := ColWidth(stripANSI(s))
	if w >= cols {
		return s
	}
	return s + blanks(cols-w)
}
