package tui

import (
	"github.com/fumiyatani/sitatame/internal/diffmodel"
)

// filePickItem is one row in the file-picker modal. FileIdx points back into
// Model.Files so the caller can resolve the chosen entry to its row stream
// location without rescanning by path (paths aren't unique under rename:
// RenameFrom and RenameTo can collide with another file's path in pathological
// diffs).
type filePickItem struct {
	FileIdx int
	Status  string // "M", "A", "D", "R", "C", "T", or "?" for invalid
	Path    string
	Adds    int
	Dels    int
}

// filePicker holds the modal's interaction state. items is built once at open
// time from Model.Files and is read-only after that — the user can't reorder
// or filter (yet), so a frozen snapshot keeps the indexing math obvious.
//
// `top` is the index of the first item rendered inside the picker's visible
// region; `height` is how many item rows fit. We deliberately keep these
// independent from the surrounding diff viewport because the picker overlays
// the diff and has its own chrome (border + hint line).
type filePicker struct {
	items  []filePickItem
	idx    int
	top    int
	height int
}

// newFilePicker builds the picker for the current diff. currentFileIdx selects
// the initially-highlighted row; if it's out of range, idx clamps to 0. height
// is the number of visible item rows (must be >= 1 to make any item visible).
//
// We never panic on empty input — a diff with zero files just produces an
// empty picker that absorbs key events without crashing. The caller decides
// whether to suppress opening (Model.openFilePicker does the early-return).
func newFilePicker(files []diffmodel.File, currentFileIdx, height int) *filePicker {
	items := make([]filePickItem, 0, len(files))
	for i, f := range files {
		adds, dels := countAddsDels(f)
		items = append(items, filePickItem{
			FileIdx: i,
			Status:  f.Status.String(),
			Path:    f.DisplayPath(),
			Adds:    adds,
			Dels:    dels,
		})
	}
	fp := &filePicker{items: items, height: height}
	if fp.height < 1 {
		fp.height = 1
	}
	fp.idx = currentFileIdx
	if fp.idx < 0 || fp.idx >= len(items) {
		fp.idx = 0
	}
	fp.scrollToIdx()
	return fp
}

// countAddsDels walks the file's hunks and totals lines whose Prefix marks an
// addition or deletion. Context lines and the binary placeholder don't count.
func countAddsDels(f diffmodel.File) (adds, dels int) {
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			switch l.Prefix {
			case '+':
				adds++
			case '-':
				dels++
			}
		}
	}
	return adds, dels
}

// moveBy shifts the selection by delta rows, clamping at both ends. The
// viewport top is recomputed so the selection stays on screen. Empty pickers
// are a no-op — without this guard, idx + delta on an empty slice would set
// idx to a non-zero value and the next selected() would index out of range.
func (fp *filePicker) moveBy(delta int) {
	if len(fp.items) == 0 {
		return
	}
	fp.idx += delta
	if fp.idx < 0 {
		fp.idx = 0
	}
	if fp.idx >= len(fp.items) {
		fp.idx = len(fp.items) - 1
	}
	fp.scrollToIdx()
}

// scrollToIdx adjusts top so idx sits inside [top, top+height). Mirrors the
// main viewport's scrollToCursor logic. Without this, moving past the bottom
// edge leaves the highlighted row off-screen with no visual indication that
// the user kept scrolling.
func (fp *filePicker) scrollToIdx() {
	if fp.height < 1 {
		fp.height = 1
	}
	if fp.idx < fp.top {
		fp.top = fp.idx
	} else if fp.idx >= fp.top+fp.height {
		fp.top = fp.idx - fp.height + 1
	}
	if fp.top < 0 {
		fp.top = 0
	}
	// Don't let top push past the last viewport-sized window. Otherwise a
	// shrink of `height` mid-session would leave blank rows at the bottom.
	maxTop := len(fp.items) - fp.height
	if maxTop < 0 {
		maxTop = 0
	}
	if fp.top > maxTop {
		fp.top = maxTop
	}
}

// selected returns the currently-highlighted item. Returns the zero value
// when the picker is empty so callers don't have to nil-check every site —
// the zero FileIdx (0) is still a valid Files index whenever Files is
// non-empty, and Model.openFilePicker refuses to open on empty Files.
func (fp *filePicker) selected() filePickItem {
	if len(fp.items) == 0 {
		return filePickItem{}
	}
	if fp.idx < 0 || fp.idx >= len(fp.items) {
		return fp.items[0]
	}
	return fp.items[fp.idx]
}

// visibleRange returns the [start, end) slice of items currently rendered.
// Centralized so the view doesn't have to recompute end clamping itself.
func (fp *filePicker) visibleRange() (int, int) {
	if len(fp.items) == 0 {
		return 0, 0
	}
	end := fp.top + fp.height
	if end > len(fp.items) {
		end = len(fp.items)
	}
	return fp.top, end
}
