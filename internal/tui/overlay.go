package tui

import (
	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// buildOverlay maps row indices to the indices of review.Comments that anchor
// to that row. A comment may attach to multiple rows (range kind) and a row
// may carry multiple comments. KindReview is ignored — review-level comments
// have no per-row anchor.
//
// Rows must already have their parent file's hunk lines populated with
// BaseLine / HeadLine via diffmodel.AssignLineNumbers; otherwise line-anchored
// comments cannot be located.
func buildOverlay(rows []row, files []diffmodel.File, comments []review.Comment) map[int][]int {
	out := map[int][]int{}
	if len(comments) == 0 {
		return out
	}

	type lineKey struct {
		fileIdx int
		side    review.Side
		line    int
	}
	rowByLine := map[lineKey]int{}
	fileHeaderRow := map[int]int{}
	for i, r := range rows {
		switch r.kind {
		case rowFileHeader:
			if _, ok := fileHeaderRow[r.fileIdx]; !ok {
				fileHeaderRow[r.fileIdx] = i
			}
		case rowLine:
			if r.fileIdx < 0 || r.fileIdx >= len(files) {
				continue
			}
			f := files[r.fileIdx]
			if r.hunkIdx < 0 || r.hunkIdx >= len(f.Hunks) {
				continue
			}
			h := f.Hunks[r.hunkIdx]
			if r.lineIdx < 0 || r.lineIdx >= len(h.Lines) {
				continue
			}
			l := h.Lines[r.lineIdx]
			if l.BaseLine != 0 {
				rowByLine[lineKey{r.fileIdx, review.SideBase, l.BaseLine}] = i
			}
			if l.HeadLine != 0 {
				rowByLine[lineKey{r.fileIdx, review.SideHead, l.HeadLine}] = i
			}
		}
	}

	fileIdxByPath := map[string]int{}
	for fi, f := range files {
		if p := f.DisplayPath(); p != "" {
			fileIdxByPath[p] = fi
		}
		if f.PrePath != "" {
			if _, ok := fileIdxByPath[f.PrePath]; !ok {
				fileIdxByPath[f.PrePath] = fi
			}
		}
		if f.RenameFrom != "" {
			if _, ok := fileIdxByPath[f.RenameFrom]; !ok {
				fileIdxByPath[f.RenameFrom] = fi
			}
		}
	}

	for ci, c := range comments {
		if c.Kind == review.KindReview {
			continue
		}
		fi, ok := fileIdxByPath[c.Path]
		if !ok {
			continue
		}
		side := c.Side
		if side == "" {
			side = review.SideHead
		}
		switch c.Kind {
		case review.KindLine:
			if c.Line == 0 {
				if i, ok := fileHeaderRow[fi]; ok {
					out[i] = append(out[i], ci)
				}
				continue
			}
			if i, ok := rowByLine[lineKey{fi, side, c.Line}]; ok {
				out[i] = append(out[i], ci)
			}
		case review.KindRange:
			start, end := c.LineStart, c.LineEnd
			if start == 0 || end == 0 {
				continue
			}
			if start > end {
				start, end = end, start
			}
			seen := map[int]bool{}
			for ln := start; ln <= end; ln++ {
				if i, ok := rowByLine[lineKey{fi, side, ln}]; ok && !seen[i] {
					out[i] = append(out[i], ci)
					seen[i] = true
				}
			}
		case review.KindFile:
			if i, ok := fileHeaderRow[fi]; ok {
				out[i] = append(out[i], ci)
			}
		}
	}
	return out
}

// overlayMarker returns the gutter marker for a row given its attached
// comments. Stale wins over open so reviewers notice drifted anchors at a
// glance.
const (
	markerOpen  = "*"
	markerStale = "~"
	markerNone  = " "
)

func overlayMarker(commentIdxs []int, comments []review.Comment) string {
	if len(commentIdxs) == 0 {
		return markerNone
	}
	hasStale, hasOpen := false, false
	for _, ci := range commentIdxs {
		if ci < 0 || ci >= len(comments) {
			continue
		}
		switch comments[ci].State {
		case review.StateStale:
			hasStale = true
		case review.StateOpen:
			hasOpen = true
		}
	}
	switch {
	case hasStale:
		return markerStale
	case hasOpen:
		return markerOpen
	default:
		return markerNone
	}
}
