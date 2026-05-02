package tui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// BinaryPlaceholder is the canonical placeholder for binary files.
const BinaryPlaceholder = "[binary]"

// widthCond fixes EastAsianWidth=false so column math is identical regardless
// of the user's LANG / LC_CTYPE. Without this, ambiguous-width chars like `…`
// would flip between 1 and 2 columns and break truncation invariants under
// JP locales.
var widthCond = func() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = false
	return c
}()

// ColWidth measures display columns under the same condition renderLine uses.
func ColWidth(s string) int { return widthCond.StringWidth(s) }

// ansiCSI matches the bulk of ANSI/VT escape sequences:
//
//	ESC [ <params> <terminator>
//
// where params is digits, separators, and `?`, and terminator is any letter.
// Bare ESC (no `[`) and OSC sequences are not modeled — the diff content we
// consume comes from `git diff --no-color`, so they shouldn't appear.
var ansiCSI = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

func stripANSI(s string) string {
	if !strings.ContainsRune(s, '\x1b') {
		return s
	}
	return ansiCSI.ReplaceAllString(s, "")
}

// renderLine sanitizes a diff content line and clips it to maxWidth columns.
// `prefix` is one of ' ', '+', '-' and is always emitted; the remaining width
// is filled with the body. Tabs are expanded to a single space and other
// control bytes are dropped — keeping the model deterministic without taking
// a hard dependency on user terminal settings.
//
// When the content is wider than maxWidth, the trailing column is replaced
// with `…` (single column). maxWidth <= 0 disables clipping.
func renderLine(prefix byte, body string, maxWidth int) string {
	body = stripANSI(body)
	var b strings.Builder
	b.Grow(len(body) + 1)
	b.WriteByte(prefix)

	if maxWidth <= 0 || maxWidth == 1 {
		// No room for body, or no clipping requested.
		if maxWidth == 1 {
			return b.String()
		}
		writeBody(&b, body, -1)
		return b.String()
	}

	writeBody(&b, body, maxWidth-1)
	return b.String()
}

type rw struct {
	r rune
	w int
}

// sanitizeRunes filters control bytes and tabs and pre-computes column widths.
// The two-pass design is required for budget-correct truncation: we need the
// total width before deciding whether to reserve a column for the ellipsis.
func sanitizeRunes(body string) (runes []rw, total int) {
	for i := 0; i < len(body); {
		r, sz := utf8.DecodeRuneInString(body[i:])
		i += sz
		if r == utf8.RuneError && sz == 1 {
			continue
		}
		if r == '\t' {
			r = ' '
		}
		if r < ' ' {
			continue
		}
		w := widthCond.RuneWidth(r)
		runes = append(runes, rw{r, w})
		total += w
	}
	return runes, total
}

// writeBody appends body to b clipped to budget columns. Tabs become spaces
// and control bytes are dropped. budget < 0 disables clipping.
//
// When clipping kicks in, we reserve the final column for `…` and emit as
// many leading runes as fit in (budget - 1) columns.
func writeBody(b *strings.Builder, body string, budget int) {
	runes, total := sanitizeRunes(body)
	if budget < 0 || total <= budget {
		for _, e := range runes {
			b.WriteRune(e.r)
		}
		return
	}
	cap := budget - 1
	if cap < 0 {
		cap = 0
	}
	used := 0
	for _, e := range runes {
		if used+e.w > cap {
			break
		}
		b.WriteRune(e.r)
		used += e.w
	}
	b.WriteRune('…')
}
