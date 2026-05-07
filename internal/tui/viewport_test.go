package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

func makeFile(path string, prefixes []byte) diffmodel.File {
	hunk := diffmodel.Hunk{BaseStart: 1, BaseLines: len(prefixes), HeadStart: 1, HeadLines: len(prefixes)}
	for i, p := range prefixes {
		hunk.Lines = append(hunk.Lines, diffmodel.Line{Prefix: p, Text: string('a' + byte(i))})
	}
	diffmodel.AssignLineNumbers(&hunk)
	return diffmodel.File{
		Status:   diffmodel.StatusModified,
		PrePath:  path, PostPath: path,
		Hunks: []diffmodel.Hunk{hunk},
	}
}

func TestBuildRows_FlatStream(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		makeFile("a", []byte{' ', '+', '-'}),
		makeFile("b", []byte{' '}),
	}
	rows := buildRows(files)
	// File A: header + hunk header + 3 lines = 5
	// File B: header + hunk header + 1 line = 3
	if got, want := len(rows), 8; got != want {
		t.Fatalf("rows = %d, want %d", got, want)
	}
	if rows[0].kind != rowFileHeader || rows[1].kind != rowHunkHeader {
		t.Errorf("expected file then hunk header; got %+v %+v", rows[0], rows[1])
	}
	if rows[5].kind != rowFileHeader {
		t.Errorf("file B header at wrong index: %+v", rows[5])
	}
}

func TestBuildRows_BinaryPlaceholder(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{{
		Status: diffmodel.StatusModified, PrePath: "x.bin", PostPath: "x.bin", Binary: true,
	}}
	rows := buildRows(files)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[1].kind != rowBinary || rows[1].text != "[binary]" {
		t.Errorf("binary row wrong: %+v", rows[1])
	}
}

func TestModel_NavigationKeys(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		makeFile("a", []byte{' ', ' '}),
		makeFile("b", []byte{' ', ' '}),
	}
	m := New(files, review.Review{})
	if m.Cursor() != 0 {
		t.Fatalf("initial cursor = %d", m.Cursor())
	}

	m, _ = applyKey(m, "j")
	if m.Cursor() != 1 {
		t.Errorf("j: cursor = %d, want 1", m.Cursor())
	}
	m, _ = applyKey(m, "k")
	if m.Cursor() != 0 {
		t.Errorf("k: cursor = %d, want 0", m.Cursor())
	}

	// `n` jumps to the next file header (row index 4 for 2x2 setup) and the
	// header lands at the top of the viewport so the file name is visible.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = updated.(Model)
	m, _ = applyKey(m, "n")
	if r := m.rows[m.Cursor()]; r.kind != rowFileHeader || r.fileIdx != 1 {
		t.Errorf("n: landed on %+v", r)
	}
	if m.top != m.Cursor() {
		t.Errorf("n: top = %d, cursor = %d, want top == cursor", m.top, m.Cursor())
	}

	// `p` returns to file A's header, also top-aligned.
	m, _ = applyKey(m, "p")
	if r := m.rows[m.Cursor()]; r.kind != rowFileHeader || r.fileIdx != 0 {
		t.Errorf("p: landed on %+v", r)
	}
	if m.top != m.Cursor() {
		t.Errorf("p: top = %d, cursor = %d, want top == cursor", m.top, m.Cursor())
	}
}

func TestModel_VirtualScroll(t *testing.T) {
	t.Parallel()
	// 100 hunks of 10 ctx lines each ~ 1.1k rows including headers.
	var hunks []diffmodel.Hunk
	for i := 0; i < 100; i++ {
		var lines []diffmodel.Line
		for j := 0; j < 10; j++ {
			lines = append(lines, diffmodel.Line{Prefix: ' ', Text: "ctx"})
		}
		hunks = append(hunks, diffmodel.Hunk{BaseStart: i*10 + 1, BaseLines: 10, HeadStart: i*10 + 1, HeadLines: 10, Lines: lines})
	}
	f := diffmodel.File{Status: diffmodel.StatusModified, PrePath: "big", PostPath: "big", Hunks: hunks}
	m := New([]diffmodel.File{f}, review.Review{})
	// Simulate a fixed-size terminal.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = updated.(Model)
	if m.Rows() < 1100 {
		t.Fatalf("expected >=1100 rows, got %d", m.Rows())
	}

	// Hammer 500 j's; cursor must stay in bounds and view must contain marker.
	for i := 0; i < 500; i++ {
		m, _ = applyKey(m, "j")
	}
	if m.Cursor() < 0 || m.Cursor() >= m.Rows() {
		t.Fatalf("cursor out of bounds: %d / %d", m.Cursor(), m.Rows())
	}
	view := m.View()
	if !strings.Contains(view, cursorMarker) {
		t.Errorf("cursor marker missing in view")
	}
	// Visible region must be small (<= viewportHeight + chrome).
	if lc := strings.Count(view, "\n"); lc > 25 {
		t.Errorf("view too tall (%d lines) — virtual scroll broken", lc)
	}
}

func applyKey(m Model, s string) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return updated.(Model), cmd
}
