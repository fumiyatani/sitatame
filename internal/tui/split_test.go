package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// hunkFile builds a single-file, single-hunk diff with the given line
// prefixes. Useful for split pairing tests where line numbers don't matter.
func hunkFile(path string, prefixes []byte) diffmodel.File {
	hunk := diffmodel.Hunk{BaseStart: 1, HeadStart: 1, BaseLines: len(prefixes), HeadLines: len(prefixes)}
	for i, p := range prefixes {
		hunk.Lines = append(hunk.Lines, diffmodel.Line{Prefix: p, Text: string('a' + byte(i))})
	}
	diffmodel.AssignLineNumbers(&hunk)
	return diffmodel.File{
		Status: diffmodel.StatusModified, PrePath: path, PostPath: path,
		Hunks: []diffmodel.Hunk{hunk},
	}
}

// linesOnly slices off header rows so pairing assertions can ignore the
// rowFileHeader / rowHunkHeader noise that buildSplitRows passes through 1:1.
func linesOnly(srs []splitRow) []splitRow {
	var out []splitRow
	for _, sr := range srs {
		if sr.kind == rowLine {
			out = append(out, sr)
		}
	}
	return out
}

func TestBuildSplitRows_EqualMinusPlus(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a", []byte{'-', '-', '+', '+'})}
	rows := buildRows(files)
	srs := linesOnly(buildSplitRows(rows))
	if len(srs) != 2 {
		t.Fatalf("split rows = %d, want 2 paired", len(srs))
	}
	for i, sr := range srs {
		if sr.base < 0 || sr.head < 0 {
			t.Errorf("pair %d not fully paired: %+v", i, sr)
		}
	}
}

func TestBuildSplitRows_MoreMinusThanPlus(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a", []byte{'-', '-', '-', '+'})}
	rows := buildRows(files)
	srs := linesOnly(buildSplitRows(rows))
	if len(srs) != 3 {
		t.Fatalf("split rows = %d, want 3", len(srs))
	}
	if srs[0].base < 0 || srs[0].head < 0 {
		t.Errorf("row 0 should be paired: %+v", srs[0])
	}
	if srs[1].base < 0 || srs[1].head != -1 {
		t.Errorf("row 1 should be base-only: %+v", srs[1])
	}
	if srs[2].base < 0 || srs[2].head != -1 {
		t.Errorf("row 2 should be base-only: %+v", srs[2])
	}
}

func TestBuildSplitRows_MorePlusThanMinus(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a", []byte{'-', '+', '+', '+'})}
	rows := buildRows(files)
	srs := linesOnly(buildSplitRows(rows))
	if len(srs) != 3 {
		t.Fatalf("split rows = %d, want 3", len(srs))
	}
	if srs[0].base < 0 || srs[0].head < 0 {
		t.Errorf("row 0 should be paired: %+v", srs[0])
	}
	if srs[1].base != -1 || srs[1].head < 0 {
		t.Errorf("row 1 should be head-only: %+v", srs[1])
	}
	if srs[2].base != -1 || srs[2].head < 0 {
		t.Errorf("row 2 should be head-only: %+v", srs[2])
	}
}

func TestBuildSplitRows_PureAddition(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a", []byte{' ', '+', '+'})}
	rows := buildRows(files)
	srs := linesOnly(buildSplitRows(rows))
	if len(srs) != 3 {
		t.Fatalf("split rows = %d, want 3", len(srs))
	}
	// context row: both sides point at the same unified row
	if srs[0].base != srs[0].head || srs[0].base < 0 {
		t.Errorf("context row malformed: %+v", srs[0])
	}
	if srs[1].base != -1 || srs[1].head < 0 {
		t.Errorf("addition row 1 should be head-only: %+v", srs[1])
	}
	if srs[2].base != -1 || srs[2].head < 0 {
		t.Errorf("addition row 2 should be head-only: %+v", srs[2])
	}
}

func TestCursorRoundTrip_PairedBaseStaysOnMinus(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a", []byte{'-', '+'})}
	rows := buildRows(files)
	srs := buildSplitRows(rows)
	// find the `-` unified row
	minusIdx := -1
	for i, r := range rows {
		if r.kind == rowLine && len(r.text) > 0 && r.text[0] == '-' {
			minusIdx = i
			break
		}
	}
	if minusIdx < 0 {
		t.Fatal("no `-` row found")
	}
	splitIdx, side := unifiedToSplitCursor(srs, minusIdx)
	if side != review.SideBase {
		t.Errorf("side = %v, want SideBase", side)
	}
	got := splitToUnifiedCursor(srs, splitIdx, side)
	if got != minusIdx {
		t.Errorf("round-trip = %d, want %d (`-` row)", got, minusIdx)
	}
}

func TestCursorRoundTrip_PairedHeadStaysOnPlus(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a", []byte{'-', '+'})}
	rows := buildRows(files)
	srs := buildSplitRows(rows)
	plusIdx := -1
	for i, r := range rows {
		if r.kind == rowLine && len(r.text) > 0 && r.text[0] == '+' {
			plusIdx = i
			break
		}
	}
	if plusIdx < 0 {
		t.Fatal("no `+` row found")
	}
	splitIdx, side := unifiedToSplitCursor(srs, plusIdx)
	if side != review.SideHead {
		t.Errorf("side = %v, want SideHead", side)
	}
	got := splitToUnifiedCursor(srs, splitIdx, side)
	if got != plusIdx {
		t.Errorf("round-trip = %d, want %d (`+` row)", got, plusIdx)
	}
}

func TestCursorRoundTrip_SingleSideIgnoresPreferred(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a", []byte{'-'})}
	rows := buildRows(files)
	srs := buildSplitRows(rows)
	minusIdx := -1
	for i, r := range rows {
		if r.kind == rowLine && len(r.text) > 0 && r.text[0] == '-' {
			minusIdx = i
			break
		}
	}
	splitIdx, _ := unifiedToSplitCursor(srs, minusIdx)
	// Even with a misleading SideHead preference, base-only row falls back
	// to the only available side rather than returning 0.
	if got := splitToUnifiedCursor(srs, splitIdx, review.SideHead); got != minusIdx {
		t.Errorf("base-only round-trip ignoring SideHead = %d, want %d", got, minusIdx)
	}
}

func TestToggleLayout_PairedMinusRoundTrip(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{hunkFile("a", []byte{'-', '+'})}
	m := setSize(New(files, review.Review{}), 60, 12)
	// move cursor to the `-` row
	for i, r := range m.rows {
		if r.kind == rowLine && len(r.text) > 0 && r.text[0] == '-' {
			m.cursor = i
			break
		}
	}
	want := m.cursor
	m = sendNamedKey(m, tea.KeyTab)
	if m.layout != LayoutSplit {
		t.Fatalf("layout = %v, want LayoutSplit", m.layout)
	}
	m = sendNamedKey(m, tea.KeyTab)
	if m.layout != LayoutUnified {
		t.Fatalf("layout = %v, want LayoutUnified", m.layout)
	}
	if m.cursor != want {
		t.Errorf("cursor after round-trip = %d, want %d (`-` row)", m.cursor, want)
	}
}
