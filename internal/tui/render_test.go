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
