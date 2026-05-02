package tui

// Selection records an active range selection inside a single hunk. Both
// endpoints are row indices into Model.rows; Anchor is the row where `V` was
// first pressed and Extent follows the cursor. Either may be smaller.
//
// MVP rule: selection is constrained to one hunk. Cursor moves that would
// leave the hunk are clamped (the cursor still moves, but Extent stops at the
// last in-hunk row).
type Selection struct {
	FileIdx int
	HunkIdx int
	Anchor  int
	Extent  int
}

// Range returns the (low, high) inclusive bounds of the selection.
func (s Selection) Range() (int, int) {
	if s.Anchor <= s.Extent {
		return s.Anchor, s.Extent
	}
	return s.Extent, s.Anchor
}

// Contains reports whether row index i is inside the selection.
func (s Selection) Contains(i int) bool {
	lo, hi := s.Range()
	return i >= lo && i <= hi
}

// startSelection seeds Selection from the cursor row, returning false if the
// cursor isn't on a content line (file header, hunk header, or binary
// placeholder cannot anchor a range).
func (m *Model) startSelection() bool {
	if len(m.rows) == 0 || m.cursor >= len(m.rows) {
		return false
	}
	r := m.rows[m.cursor]
	if r.kind != rowLine {
		return false
	}
	if r.fileIdx >= 0 && r.fileIdx < len(m.Files) && m.Files[r.fileIdx].Binary {
		return false
	}
	m.selection = &Selection{
		FileIdx: r.fileIdx,
		HunkIdx: r.hunkIdx,
		Anchor:  m.cursor,
		Extent:  m.cursor,
	}
	return true
}

// extendSelection follows the cursor while keeping Extent on a row inside the
// original hunk. Cursor positions outside the hunk leave Extent at the last
// matching row.
func (m *Model) extendSelection() {
	if m.selection == nil {
		return
	}
	dir := 0
	if m.cursor > m.selection.Extent {
		dir = 1
	} else if m.cursor < m.selection.Extent {
		dir = -1
	}
	if dir == 0 {
		return
	}
	last := m.selection.Extent
	for i := m.selection.Extent + dir; ; i += dir {
		if i < 0 || i >= len(m.rows) {
			break
		}
		r := m.rows[i]
		if r.kind != rowLine || r.fileIdx != m.selection.FileIdx || r.hunkIdx != m.selection.HunkIdx {
			break
		}
		last = i
		if i == m.cursor {
			break
		}
	}
	m.selection.Extent = last
}

func (m *Model) clearSelection() { m.selection = nil }
