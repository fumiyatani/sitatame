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
			marker := overlayMarker(m.overlay[i], m.Review.Comments)
			gutter := lineNumberGutter(m.rows[i], m.Files, m.lnBaseW, m.lnHeadW)
			body := colorizeRow(m.rows[i], renderRow(m.rows[i], bodyMax))
			if hasComment(m.overlay[i]) {
				// lineNumberGutter は rowLine 以外やガター幅 0 で空白を返すため、
				// 文字色だけだと不可視になる。そのケースは marker/body 側に逃がす。
				if m.rows[i].kind == rowLine && gw > 0 {
					gutter = applyCommentHighlight(gutter)
				} else {
					marker = applyCommentHighlight(marker)
					body = applyCommentHighlight(body)
				}
			}
			b.WriteString(marker)
			b.WriteString(gutter)
			b.WriteString(body)
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
	cursorRow, totalRows := m.cursor, len(m.rows)
	fileIdx := fileIndexAtCursor(m)
	if m.layout == LayoutSplit {
		cursorRow, totalRows = m.splitCursor, len(m.splitRows)
		if totalRows > 0 && m.splitCursor < totalRows {
			fileIdx = m.splitRows[m.splitCursor].fileIdx
		}
	}
	if fileIdx >= 0 && fileIdx < len(m.Files) {
		path = m.Files[fileIdx].DisplayPath()
		binary = m.Files[fileIdx].Binary
	}
	if path == "" {
		path = "(none)"
	}
	tag := ""
	if binary {
		tag = "  [binary] file-comment only"
	}
	mode := "[unified]"
	if m.layout == LayoutSplit {
		mode = "[split: preview]"
	}
	trailing := mode
	if m.statusMsg != "" {
		trailing = m.statusMsg
	}
	return fmt.Sprintf("sitatame %s  [%d/%d files]  row %d/%d%s  %s",
		path, fileIdx+1, len(m.Files), cursorRow+1, totalRows, tag, trailing)
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

// modeTagHasComment surfaces "this row already carries comments" so the
// reviewer knows `x` / `c` will act on existing anchors. Uses the same
// `-- TAG --` shape as modeTagRange so the eye sees it as one family.
const modeTagHasComment = "-- has comment --"

// modeTagHasCommentShort is the fallback tag used when the long form would
// not fit beside the hint within m.width.
const modeTagHasCommentShort = "-- has cmt --"

// hintVariant pairs a left hint with an optional right tag. selectHint
// iterates the variants in order and returns the first one that fits within
// m.width when joined by a single space. The last variant is the unconditional
// fallback — it is emitted even if it overflows so the hint line is never empty.
type hintVariant struct {
	left  string
	right string
}

// hintMode picks the active mode for the hint line. Priority matters because
// selection and comment-row can both be true at the same time; the spec calls
// for selection to win in that case.
type hintMode int

const (
	hintModeNormal hintMode = iota
	hintModeSelection
	hintModeHasComment
	hintModeSplit
)

// resolveHintMode encodes the mode-priority rules:
//
//  1. split layout dominates — selection / comment-row keys are blocked there.
//  2. an active selection beats a comment-row indicator. (The selection might
//     happen to cover a row with existing comments; the right tag should still
//     surface the selection mode.)
//  3. an existing comment on the cursor row (action label = RESOLVE/REOPEN).
//  4. fallthrough to the default.
//
// 第 2 戻り値はカーソル行で `x` が実行する動作のラベル。hintModeHasComment 以外では
// 空文字でよい。stale のみの行は ok=false で hintModeNormal に倒れる。
func resolveHintMode(m Model) (hintMode, string) {
	if m.layout == LayoutSplit {
		return hintModeSplit, ""
	}
	if m.selection != nil {
		return hintModeSelection, ""
	}
	// stale のみの overlay は has-comment hint を出さない。`x` で何も起きないのに
	// RESOLVE/REOPEN を案内すると誤誘導になるため、stale-only 行は通常 hint へ。
	if m.cursor >= 0 {
		if label, ok := resolveActionLabel(m.overlay[m.cursor], m.Review.Comments); ok {
			return hintModeHasComment, label
		}
	}
	return hintModeNormal, ""
}

// hintVariantsFor returns the candidate hint variants for `mode`, ordered from
// the richest expression to the shortest fallback. selectHint walks the slice
// and picks the first variant that fits in `width`; the final entry is always
// kept short enough that an 80-col viewport can hold it.
//
// `action` は hintModeHasComment 限定で `x` の挙動ラベル ("RESOLVE" / "REOPEN") を
// 受け取り、rich/short の両 variant に同じ文字列を埋め込む。両方で一致させるのは
// 80 桁未満に縮退した際にラベルと action が食い違わないようにするため。
func hintVariantsFor(mode hintMode, action string) []hintVariant {
	switch mode {
	case hintModeSelection:
		return []hintVariant{
			{left: "j/k extend · c COMMENT · esc cancel", right: modeTagRange},
			{left: "extend · c · esc", right: modeTagRange},
		}
	case hintModeHasComment:
		return []hintVariant{
			{left: "j/k move · x " + action + " · c add", right: modeTagHasComment},
			{left: "j/k · x · c", right: modeTagHasCommentShort},
		}
	case hintModeSplit:
		return []hintVariant{
			{left: "j/k move · n/p file · Tab unified · ? help", right: ""},
			{left: "j/k · n/p · Tab · ?", right: ""},
		}
	default: // hintModeNormal
		return []hintVariant{
			{left: "j/k move · n/p file · c cmt · ? help · q quit", right: ""},
			{left: "j/k · n/p · c · ? · q", right: ""},
		}
	}
}

// formatHint joins the left/right halves of a variant. When width is large
// enough the right tag is right-aligned with space padding; otherwise the two
// halves are joined by a single space. width <= 0 falls back to the single-
// space form too so non-resized models (width == 0 before WindowSizeMsg) still
// render something sensible.
func formatHint(v hintVariant, width int) string {
	if v.right == "" {
		return v.left
	}
	leftW := ColWidth(v.left)
	rightW := ColWidth(v.right)
	if width <= 0 || leftW+1+rightW > width {
		return v.left + " " + v.right
	}
	return v.left + strings.Repeat(" ", width-leftW-rightW) + v.right
}

// hintFits reports whether the variant rendered onto a single row of `width`
// would stay inside the viewport. width <= 0 means "unknown size" and is
// treated as fitting so the very first paint (before any WindowSizeMsg)
// surfaces the richest variant.
func hintFits(v hintVariant, width int) bool {
	if width <= 0 {
		return true
	}
	w := ColWidth(v.left)
	if v.right != "" {
		w += 1 + ColWidth(v.right)
	}
	return w <= width
}

// selectHint walks the variants in priority order and returns the first one
// that fits. The last variant is always emitted as a last resort so the hint
// line never disappears entirely when the viewport is extremely narrow.
func selectHint(variants []hintVariant, width int) hintVariant {
	for i, v := range variants {
		if i == len(variants)-1 || hintFits(v, width) {
			return v
		}
	}
	return variants[len(variants)-1]
}

func hintLine(m Model) string {
	mode, action := resolveHintMode(m)
	variants := hintVariantsFor(mode, action)
	v := selectHint(variants, m.width)
	return formatHint(v, m.width)
}
