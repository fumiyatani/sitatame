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

// sanitizePath removes control bytes from a string that will be rendered as
// chrome text (e.g. file picker rows) instead of as a diff body. Two-step:
// stripANSI for CSI sequences, then drop bare ESC and other C0/C1 controls so
// nothing reaches the terminal that could move the cursor or rewrite cells.
//
// Why this exists separately from renderLine's writeBody: the file picker
// composes its own rows (markers, status column, paths, counts) and never
// goes through writeBody, so without this helper File.Path bytes coming from
// `git diff --raw -z` would render verbatim. Tab is preserved as a space so
// path widths stay visually similar to the original; every other byte below
// 0x20, plus DEL (0x7F) and the C1 control range U+0080–U+009F (notably
// 0x9B CSI / 0x9E PM / 0x9F APC, which xterm-compatible terminals will
// interpret as ESC-introduced sequences when received as raw UTF-8), is
// replaced with '?' to keep a visible signal that something was scrubbed
// (vs. silently dropping bytes which could collapse distinct paths to
// identical-looking strings).
func sanitizePath(s string) string {
	s = stripANSI(s)
	if !needsControlScrub(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		switch {
		case r == utf8.RuneError && sz == 1:
			b.WriteRune('?')
		case r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7F || (r >= 0x80 && r < 0xA0):
			b.WriteRune('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// needsControlScrub is a fast pre-check so the common case (clean ASCII path)
// avoids allocating a builder. Any byte >= 0x80 forces the slow path so the
// UTF-8 decoder can identify C1 controls (U+0080–U+009F) that would otherwise
// slip through a byte-wise check — e.g. 0x9B (CSI) acts as a single-byte
// escape introducer on xterm-compatible terminals.
func needsControlScrub(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7F || c >= 0x80 {
			return true
		}
	}
	return false
}

// wrapBody wraps a sanitized diff body string into multiple lines, each at
// most `budget` display columns wide. Tabs are expanded to spaces and control
// bytes are dropped (same rules as writeBody). budget <= 0 returns the whole
// body as a single element without any wrapping.
//
// The returned slice always has at least one element (even for an empty body).
// Every element has a column width <= budget.
func wrapBody(body string, budget int) []string {
	runes, total := sanitizeRunes(body)
	if budget <= 0 || total <= budget {
		var b strings.Builder
		for _, e := range runes {
			b.WriteRune(e.r)
		}
		return []string{b.String()}
	}
	var lines []string
	var b strings.Builder
	used := 0
	for _, e := range runes {
		if used+e.w > budget {
			lines = append(lines, b.String())
			b.Reset()
			used = 0
		}
		b.WriteRune(e.r)
		used += e.w
	}
	if b.Len() > 0 || len(lines) == 0 {
		lines = append(lines, b.String())
	}
	return lines
}

// wrapRow wraps a diff row into one or more screen lines each of at most
// bodyMax display columns. The prefix byte ('+'/'-'/' ') is prepended to the
// first screen line only; continuation lines receive a space prefix so they
// align with content. maxWidth covers the prefix + body together.
//
// Returns at least one element.
func wrapRow(r row, maxWidth int) []string {
	if maxWidth <= 1 {
		return []string{string([]byte{' '})}
	}
	var prefix byte = ' '
	var body string
	if r.kind == rowLine && len(r.text) > 0 {
		prefix = r.text[0]
		body = r.text[1:]
	} else {
		body = r.text
	}
	body = stripANSI(body)
	budget := maxWidth - 1 // 1 col for prefix
	segments := wrapBody(body, budget)
	out := make([]string, len(segments))
	for i, seg := range segments {
		var b strings.Builder
		if i == 0 {
			b.WriteByte(prefix)
		} else {
			b.WriteByte(' ') // continuation indent
		}
		b.WriteString(seg)
		out[i] = b.String()
	}
	return out
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
