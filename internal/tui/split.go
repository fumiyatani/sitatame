package tui

import "github.com/fumiyatani/sitatame/internal/review"

// LayoutMode selects between the unified and split (preview) views.
type LayoutMode int

const (
	LayoutUnified LayoutMode = iota
	LayoutSplit
)

// splitRow is one row in the side-by-side stream. base and head reference
// indexes in the unified row stream (Model.rows); -1 marks an empty side.
// File / hunk headers and binary placeholders are emitted with both sides
// pointing at the same unified row.
type splitRow struct {
	kind    rowKind
	fileIdx int
	hunkIdx int
	base    int
	head    int
}

// buildSplitRows pairs each hunk's deletions with insertions so the split
// view can render base / head columns side-by-side. Context lines emit a
// single paired row referencing the same unified row from both columns.
//
// Pairing rule: a contiguous run of `-` followed by a contiguous run of `+`
// inside one hunk is zipped index-by-index. The longer side overflows into
// single-side rows. Pure `-` or pure `+` runs (no opposite side) become
// single-side rows directly.
func buildSplitRows(rows []row) []splitRow {
	out := make([]splitRow, 0, len(rows))
	i := 0
	for i < len(rows) {
		r := rows[i]
		if r.kind != rowLine {
			out = append(out, splitRow{kind: r.kind, fileIdx: r.fileIdx, hunkIdx: r.hunkIdx, base: i, head: i})
			i++
			continue
		}
		prefix := lineRowPrefix(r)
		switch prefix {
		case '-':
			minuses, pluses, end := collectChange(rows, i)
			out = append(out, pairChange(r, minuses, pluses)...)
			i = end
		case '+':
			pluses, end := collectRunOfPrefix(rows, i, '+')
			for _, k := range pluses {
				out = append(out, splitRow{kind: rowLine, fileIdx: r.fileIdx, hunkIdx: r.hunkIdx, base: -1, head: k})
			}
			i = end
		default:
			out = append(out, splitRow{kind: rowLine, fileIdx: r.fileIdx, hunkIdx: r.hunkIdx, base: i, head: i})
			i++
		}
	}
	return out
}

// collectChange gathers a run of `-` lines starting at start, then any
// immediately-following run of `+` lines in the same hunk. Returns the row
// indexes for each side and the index just past the change block.
func collectChange(rows []row, start int) (minuses, pluses []int, end int) {
	minuses, mid := collectRunOfPrefix(rows, start, '-')
	pluses, end = collectRunOfPrefix(rows, mid, '+')
	return
}

func collectRunOfPrefix(rows []row, start int, prefix byte) ([]int, int) {
	if start >= len(rows) {
		return nil, start
	}
	first := rows[start]
	var idxs []int
	i := start
	for i < len(rows) {
		r := rows[i]
		if r.kind != rowLine || r.fileIdx != first.fileIdx || r.hunkIdx != first.hunkIdx {
			break
		}
		if lineRowPrefix(r) != prefix {
			break
		}
		idxs = append(idxs, i)
		i++
	}
	return idxs, i
}

func pairChange(r row, minuses, pluses []int) []splitRow {
	n := len(minuses)
	if len(pluses) > n {
		n = len(pluses)
	}
	out := make([]splitRow, 0, n)
	for k := 0; k < n; k++ {
		sr := splitRow{kind: rowLine, fileIdx: r.fileIdx, hunkIdx: r.hunkIdx, base: -1, head: -1}
		if k < len(minuses) {
			sr.base = minuses[k]
		}
		if k < len(pluses) {
			sr.head = pluses[k]
		}
		out = append(out, sr)
	}
	return out
}

func lineRowPrefix(r row) byte {
	if len(r.text) == 0 {
		return ' '
	}
	return r.text[0]
}

// unifiedToSplitCursor finds the split row corresponding to a unified row
// index and returns the side affinity to remember for the round-trip back.
// Falls back to (0, SideHead) when the index isn't found, which only happens
// in malformed input.
func unifiedToSplitCursor(splitRows []splitRow, unifiedIdx int) (int, review.Side) {
	for i, sr := range splitRows {
		if sr.base == unifiedIdx && sr.head == unifiedIdx {
			return i, review.SideHead
		}
		if sr.base == unifiedIdx {
			return i, review.SideBase
		}
		if sr.head == unifiedIdx {
			return i, review.SideHead
		}
	}
	return 0, review.SideHead
}

// toggleLayout flips between unified and split, translating the cursor and
// remembering the side affinity so unified→split→unified lands on the same
// row (e.g. a `-` row stays a `-` row even though split paired it with a `+`).
//
// A layout switch resets the sticky resolve anchor: x is a unified-mode
// action and the user crossing layouts is a strong signal that the next x
// should re-evaluate the open-biased default rather than re-undo whatever
// was toggled before the trip into split.
func (m *Model) toggleLayout() {
	if len(m.rows) == 0 {
		return
	}
	if m.layout == LayoutUnified {
		if m.splitRows == nil {
			m.splitRows = buildSplitRows(m.rows)
		}
		// Comments may have been added/edited while in unified mode, so the
		// overlay rebuild is required for every entry to keep markers fresh.
		m.splitOverlay = buildSplitOverlay(m.splitRows, m.Review.Comments, m.overlay)
		idx, side := unifiedToSplitCursor(m.splitRows, m.cursor)
		m.splitCursor = idx
		m.splitPreferredSide = side
		m.layout = LayoutSplit
		m.invalidateLastToggle()
		m.scrollSplitToCursor()
		return
	}
	m.cursor = splitToUnifiedCursor(m.splitRows, m.splitCursor, m.splitPreferredSide)
	m.layout = LayoutUnified
	m.invalidateLastToggle()
	m.scrollToCursor()
}

func (m *Model) moveSplitCursorBy(d int) {
	if len(m.splitRows) == 0 {
		return
	}
	prev := m.splitCursor
	m.splitCursor += d
	if m.splitCursor < 0 {
		m.splitCursor = 0
	}
	if m.splitCursor >= len(m.splitRows) {
		m.splitCursor = len(m.splitRows) - 1
	}
	if m.splitCursor != prev {
		// Same rationale as unified moveCursorBy: leaving the row makes the
		// previous sticky anchor irrelevant. Without this, returning to
		// unified would carry the anchor across split navigation and the
		// next x would silently undo a range comment touched before the
		// split round-trip.
		m.invalidateLastToggle()
	}
	m.refreshSplitPreferredSide()
	m.scrollSplitToCursor()
}

// refreshSplitPreferredSide keeps splitPreferredSide consistent with the
// current splitCursor's available sides — paired rows preserve the user's
// last choice, single-side rows clamp to whichever side exists.
func (m *Model) refreshSplitPreferredSide() {
	if m.splitCursor < 0 || m.splitCursor >= len(m.splitRows) {
		return
	}
	sr := m.splitRows[m.splitCursor]
	switch {
	case sr.base >= 0 && sr.head >= 0:
		// keep current preferred side
	case sr.base >= 0:
		m.splitPreferredSide = review.SideBase
	case sr.head >= 0:
		m.splitPreferredSide = review.SideHead
	}
}

func (m *Model) jumpSplitFile(dir int) {
	if len(m.splitRows) == 0 {
		return
	}
	if dir > 0 {
		for i := m.splitCursor + 1; i < len(m.splitRows); i++ {
			if m.splitRows[i].kind == rowFileHeader {
				m.splitCursor = i
				m.invalidateLastToggle()
				m.refreshSplitPreferredSide()
				m.scrollSplitToCursor()
				return
			}
		}
		return
	}
	start := m.splitCursor
	if m.splitRows[m.splitCursor].kind == rowFileHeader {
		start--
	}
	for i := start; i >= 0; i-- {
		if m.splitRows[i].kind == rowFileHeader {
			m.splitCursor = i
			m.invalidateLastToggle()
			m.refreshSplitPreferredSide()
			m.scrollSplitToCursor()
			return
		}
	}
}

func (m *Model) scrollSplitToCursor() {
	h := m.viewportHeight()
	if m.splitCursor < m.splitTop {
		m.splitTop = m.splitCursor
	} else if m.splitCursor >= m.splitTop+h {
		m.splitTop = m.splitCursor - h + 1
	}
	if m.splitTop < 0 {
		m.splitTop = 0
	}
}

// splitToUnifiedCursor maps a split row + side affinity back to the unified
// row. Paired rows honor preferredSide; single-side rows return the only
// available side regardless.
func splitToUnifiedCursor(splitRows []splitRow, splitIdx int, preferredSide review.Side) int {
	if splitIdx < 0 || splitIdx >= len(splitRows) {
		return 0
	}
	sr := splitRows[splitIdx]
	switch {
	case sr.base >= 0 && sr.head >= 0:
		if preferredSide == review.SideBase {
			return sr.base
		}
		return sr.head
	case sr.base >= 0:
		return sr.base
	case sr.head >= 0:
		return sr.head
	}
	return 0
}
