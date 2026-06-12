package tui

import (
	"strings"
	"testing"

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
