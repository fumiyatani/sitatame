package tui

import (
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

func TestStripANSI(t *testing.T) {
	t.Parallel()
	in := "\x1b[31mred\x1b[0m and plain"
	if got := stripANSI(in); got != "red and plain" {
		t.Errorf("stripANSI = %q", got)
	}
}

func TestRenderLine_Plain(t *testing.T) {
	t.Parallel()
	got := renderLine(' ', "hello", 80)
	if got != " hello" {
		t.Errorf("renderLine = %q", got)
	}
}

func TestRenderLine_Truncate(t *testing.T) {
	t.Parallel()
	got := renderLine('+', "abcdefghij", 6)
	// prefix '+' + 4 chars + '…' = 6 columns
	wantLen := 6
	if w := ColWidth(got); w != wantLen {
		t.Errorf("width = %d, want %d (got=%q)", w, wantLen, got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix: %q", got)
	}
}

func TestRenderLine_CJKWidth(t *testing.T) {
	t.Parallel()
	// `日本語` = 2 columns each = 6 columns; with prefix it's 7.
	got := renderLine(' ', "日本語", 80)
	if w := ColWidth(got); w != 7 {
		t.Errorf("CJK width wrong: got %q (%d cols)", got, w)
	}

	// Truncation must respect 2-col chars: max=4 means prefix + budget=3, but
	// next 2-col char would overflow at col=3 (used=2 + w=2 > budget=3).
	clipped := renderLine(' ', "日本語", 4)
	if w := ColWidth(clipped); w > 4 {
		t.Errorf("CJK clip overshot: %q (%d)", clipped, w)
	}
	if !strings.HasSuffix(clipped, "…") {
		t.Errorf("CJK clip missing ellipsis: %q", clipped)
	}
}

func TestRenderLine_StripsControl(t *testing.T) {
	t.Parallel()
	// \x1b[1m bold ON, \x07 BEL, then plain. After sanitize: " bold text".
	got := renderLine(' ', "\x1b[1mbold\x07 text", 80)
	if got != " bold text" {
		t.Errorf("renderLine = %q", got)
	}
}

func TestRenderLine_BinaryPlaceholder(t *testing.T) {
	t.Parallel()
	// The placeholder constant is what viewport.go emits; render it raw to
	// confirm width-fit and no mangling.
	got := renderLine(' ', BinaryPlaceholder, 80)
	if got != " "+BinaryPlaceholder {
		t.Errorf("binary placeholder mangled: %q", got)
	}
}

// TestSanitizePath_StripsC1ControlBytes verifies that every byte in the C1
// control range (U+0080–U+009F) is replaced with '?' when it appears in a
// path. xterm-compatible terminals treat 0x9B as a single-byte CSI
// introducer (equivalent to ESC `[`), so a path containing it could move
// the cursor or rewrite cells if rendered raw — the original PR54 round 3
// fast path only checked `< 0x20 || == 0x7F` and let these through.
func TestSanitizePath_StripsC1ControlBytes(t *testing.T) {
	t.Parallel()
	for b := 0x80; b <= 0x9F; b++ {
		// Encode as UTF-8 — a single rune in [0x80, 0x9F] occupies two
		// bytes (0xC2 0x80..0xC2 0x9F). We want the *rune* to be a C1
		// control, not the raw byte (which would be invalid UTF-8 and
		// caught separately by the RuneError branch).
		in := "ok/" + string(rune(b)) + "end"
		got := sanitizePath(in)
		want := "ok/?end"
		if got != want {
			t.Errorf("sanitizePath(C1 0x%02X) = %q, want %q", b, got, want)
		}
	}
	// Spot-check the named offenders so a regression on these specific
	// codepoints fails loudly.
	for _, tc := range []struct {
		name string
		r    rune
	}{
		{"CSI (0x9B)", 0x9B},
		{"PM (0x9E)", 0x9E},
		{"APC (0x9F)", 0x9F},
	} {
		in := "a" + string(tc.r) + "b"
		got := sanitizePath(in)
		if got != "a?b" {
			t.Errorf("sanitizePath(%s) = %q, want %q", tc.name, got, "a?b")
		}
	}
}

// TestSanitizePath_PreservesValidUTF8 confirms the C1 fix did not over-scrub
// ordinary multi-byte UTF-8 (Japanese filenames are the canonical case the
// fast-path widening could regress).
func TestSanitizePath_PreservesValidUTF8(t *testing.T) {
	t.Parallel()
	cases := []string{
		"日本語/ファイル.go",
		"café/menü.txt",
		"emoji/🎉.md",
		"plain/ascii.go",
		"", // empty path stays empty
	}
	for _, in := range cases {
		if got := sanitizePath(in); got != in {
			t.Errorf("sanitizePath(%q) = %q, want unchanged", in, got)
		}
	}
}

// TestSanitizePath_StripsC0AndEscape locks in the pre-existing C0 / ESC /
// DEL behavior so the C1 widening does not silently drop coverage of the
// original PR54 round 3 fix.
func TestSanitizePath_StripsC0AndEscape(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"safe/path.go", "safe/path.go"},
		// Full CSI sequence is dropped by stripANSI before sanitizePath
		// sees it (this is how the helper layers compose).
		{"esc\x1b[31mred", "escred"},
		// Bare ESC with no `[ ... letter` survives stripANSI and must be
		// scrubbed to '?' by sanitizePath.
		{"bare\x1bend", "bare?end"},
		{"bell\x07here", "bell?here"},
		{"del\x7fhere", "del?here"},
		{"tab\there", "tab here"}, // tab → single space
		{"\x00null", "?null"},
	}
	for _, tc := range cases {
		if got := sanitizePath(tc.in); got != tc.want {
			t.Errorf("sanitizePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestWrapBody_NoWrapNeeded confirms that a body fitting inside budget returns
// a single segment with the body unchanged.
func TestWrapBody_NoWrapNeeded(t *testing.T) {
	t.Parallel()
	segs := wrapBody("hello", 10)
	if len(segs) != 1 {
		t.Fatalf("len(segs) = %d, want 1", len(segs))
	}
	if segs[0] != "hello" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "hello")
	}
}

// TestWrapBody_ExactBudget confirms that a body exactly equal to budget width
// returns a single segment (no off-by-one wrap).
func TestWrapBody_ExactBudget(t *testing.T) {
	t.Parallel()
	segs := wrapBody("abcde", 5)
	if len(segs) != 1 {
		t.Fatalf("len(segs) = %d, want 1; body %q budget 5", len(segs), "abcde")
	}
}

// TestWrapBody_Wraps confirms that a body wider than budget is split into
// multiple segments, each at most budget columns wide and with no ellipsis.
func TestWrapBody_Wraps(t *testing.T) {
	t.Parallel()
	// 12 ASCII chars, budget=5 → 3 segments: "abcde", "fghij", "kl"
	segs := wrapBody("abcdefghijkl", 5)
	if len(segs) != 3 {
		t.Fatalf("len(segs) = %d, want 3; segs=%v", len(segs), segs)
	}
	if segs[0] != "abcde" || segs[1] != "fghij" || segs[2] != "kl" {
		t.Errorf("segs = %v, want [abcde fghij kl]", segs)
	}
	// No ellipsis in any segment.
	for _, s := range segs {
		if strings.Contains(s, "…") {
			t.Errorf("unexpected ellipsis in segment %q", s)
		}
	}
}

// TestWrapBody_CJK confirms that wrap respects 2-column CJK characters and
// never overflows the budget.
func TestWrapBody_CJK(t *testing.T) {
	t.Parallel()
	// "日本語" = 3×2 = 6 cols. budget=4 → "日本" (4 cols), "語" (2 cols)
	segs := wrapBody("日本語", 4)
	if len(segs) != 2 {
		t.Fatalf("len(segs) = %d, want 2; segs=%v", len(segs), segs)
	}
	for _, s := range segs {
		if w := ColWidth(s); w > 4 {
			t.Errorf("segment %q has width %d > 4", s, w)
		}
	}
}

// TestWrapBody_ZeroBudget confirms that a zero/negative budget disables
// wrapping and returns the entire body as a single segment.
func TestWrapBody_ZeroBudget(t *testing.T) {
	t.Parallel()
	segs := wrapBody("abcdef", 0)
	if len(segs) != 1 {
		t.Fatalf("len(segs) = %d, want 1 (no wrap at budget=0)", len(segs))
	}
}

// TestWrapBody_EmptyBody confirms that an empty body returns a single empty
// segment rather than panicking or returning nil.
func TestWrapBody_EmptyBody(t *testing.T) {
	t.Parallel()
	segs := wrapBody("", 10)
	if len(segs) != 1 || segs[0] != "" {
		t.Errorf("wrapBody(\"\", 10) = %v, want [\"\"]", segs)
	}
}

// TestWrapRow_PrefixOnFirstSegmentOnly confirms that wrapRow puts the diff
// prefix ('+'/'-'/' ') on the first screen line only, and a space on
// continuation lines.
func TestWrapRow_PrefixOnFirstSegmentOnly(t *testing.T) {
	t.Parallel()
	// 12-char body + '+' prefix; maxWidth=7 → budget=6 per segment body
	// segments: "abcdef" "ghijkl"
	r := row{
		kind: rowLine,
		text: "+abcdefghijkl",
	}
	segs := wrapRow(r, 7)
	if len(segs) != 2 {
		t.Fatalf("len(segs) = %d, want 2; segs=%v", len(segs), segs)
	}
	if segs[0][0] != '+' {
		t.Errorf("first segment prefix = %q, want '+'", string(segs[0][0]))
	}
	if segs[1][0] != ' ' {
		t.Errorf("continuation prefix = %q, want ' '", string(segs[1][0]))
	}
}

// TestWrapRow_SingleLine confirms that a row whose body fits in maxWidth
// returns exactly one segment.
func TestWrapRow_SingleLine(t *testing.T) {
	t.Parallel()
	r := row{kind: rowLine, text: " short"}
	segs := wrapRow(r, 80)
	if len(segs) != 1 {
		t.Fatalf("unexpected wrap for short row: %v", segs)
	}
}

// TestWrapBody_AlwaysOneElement confirms that wrapBody never returns an empty
// slice, even on a control-only input that sanitizeRunes strips entirely.
func TestWrapBody_AlwaysOneElement(t *testing.T) {
	t.Parallel()
	// All control bytes; sanitizeRunes drops them → runes=[], total=0.
	segs := wrapBody("\x01\x02\x03", 10)
	if len(segs) == 0 {
		t.Fatalf("wrapBody returned empty slice; want at least one element")
	}
}

// TestMainView_LongLineWraps_NoEllipsis verifies end-to-end that a 200-char
// source line rendered in a width=80 viewport produces no ellipsis character
// in the view output and that the full body text appears across wrapped lines.
func TestMainView_LongLineWraps_NoEllipsis(t *testing.T) {
	t.Parallel()
	longBody := strings.Repeat("x", 200)
	h := diffmodel.Hunk{
		BaseStart: 1, BaseLines: 1, HeadStart: 1, HeadLines: 1,
		Lines: []diffmodel.Line{{Prefix: ' ', Text: longBody}},
	}
	diffmodel.AssignLineNumbers(&h)
	f := diffmodel.File{
		Status:  diffmodel.StatusModified,
		PrePath: "a.go", PostPath: "a.go",
		Hunks: []diffmodel.Hunk{h},
	}
	m := setSize(New([]diffmodel.File{f}, review.Review{}), 80, 30)
	v := m.View()

	// No ellipsis in the view.
	if strings.Contains(v, "…") {
		t.Errorf("view contains ellipsis but should wrap instead:\n%s", v)
	}
	// All 200 'x' chars should appear in the view (no content lost).
	totalX := strings.Count(v, "x")
	if totalX < 200 {
		t.Errorf("view contains %d 'x' chars, want >= 200; content was truncated:\n%s", totalX, v)
	}
}

// TestMainView_LongLine_JMovesSourceRow verifies that j/k navigation after a
// wrapped long line still moves by source rows (not screen rows): one `j` from
// the long-line row must land on the next source row, not on a wrap continuation.
func TestMainView_LongLine_JMovesSourceRow(t *testing.T) {
	t.Parallel()
	longBody := strings.Repeat("y", 200)
	h := diffmodel.Hunk{
		BaseStart: 1, BaseLines: 2, HeadStart: 1, HeadLines: 2,
		Lines: []diffmodel.Line{
			{Prefix: ' ', Text: longBody},
			{Prefix: ' ', Text: "short"},
		},
	}
	diffmodel.AssignLineNumbers(&h)
	f := diffmodel.File{
		Status:  diffmodel.StatusModified,
		PrePath: "b.go", PostPath: "b.go",
		Hunks: []diffmodel.Hunk{h},
	}
	m := setSize(New([]diffmodel.File{f}, review.Review{}), 80, 30)
	// cursor starts at row 0 (file header). Row layout:
	//   0: file header
	//   1: hunk header
	//   2: long context line (200 chars)
	//   3: short context line
	// After j j j cursor should be at row 3 (short line), not a wrap continuation.
	m, _ = applyKey(m, "j") // row 1 (hunk header)
	m, _ = applyKey(m, "j") // row 2 (long line)
	m, _ = applyKey(m, "j") // row 3 (short line)
	if got := m.Cursor(); got != 3 {
		t.Errorf("cursor = %d after 3×j, want 3 (j moves by source row, not screen row)", got)
	}
	// The view should contain "short" (the next source line is visible).
	v := m.View()
	if !strings.Contains(v, "short") {
		t.Errorf("view missing 'short' after navigating past long line:\n%s", v)
	}
}
