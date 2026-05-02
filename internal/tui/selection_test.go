package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

func twoLineHunkFile() diffmodel.File {
	return diffmodel.File{
		Status:   diffmodel.StatusModified,
		PrePath:  "a.go", PostPath: "a.go",
		Hunks: []diffmodel.Hunk{{
			BaseStart: 1, BaseLines: 2, HeadStart: 1, HeadLines: 2,
			Lines: []diffmodel.Line{
				{Prefix: ' ', Text: "x"},
				{Prefix: '+', Text: "y"},
				{Prefix: '-', Text: "z"},
			},
		}},
	}
}

func TestSelection_StartOnLineRowOnly(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := New(files, review.Review{})
	// cursor starts at row 0 (file header) — V should be a no-op.
	m, _ = applyKey(m, "V")
	if m.SelectionState() != nil {
		t.Errorf("V on file header should not start selection: %+v", m.SelectionState())
	}

	// move into the hunk (row 1 = hunk header, row 2 = first line)
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "V")
	sel := m.SelectionState()
	if sel == nil {
		t.Fatalf("V on content line should start selection")
	}
	if sel.Anchor != 2 || sel.Extent != 2 {
		t.Errorf("seed selection wrong: %+v", sel)
	}

	// j extends downward, k retracts.
	m, _ = applyKey(m, "j")
	if s := m.SelectionState(); s.Anchor != 2 || s.Extent != 3 {
		t.Errorf("after j: %+v", s)
	}
	m, _ = applyKey(m, "k")
	if s := m.SelectionState(); s.Anchor != 2 || s.Extent != 2 {
		t.Errorf("after k: %+v", s)
	}

	// Esc clears.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.SelectionState() != nil {
		t.Errorf("Esc should clear selection, got %+v", m.SelectionState())
	}
}

func TestSelection_BinarySuppressed(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		{Status: diffmodel.StatusModified, PrePath: "img.bin", PostPath: "img.bin", Binary: true},
	}
	m := New(files, review.Review{})
	// cursor row 0 = file header, row 1 = binary placeholder.
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "V")
	if m.SelectionState() != nil {
		t.Errorf("V on binary placeholder must not start selection: %+v", m.SelectionState())
	}
}

func TestSelection_StaysWithinHunk(t *testing.T) {
	t.Parallel()
	// Two hunks; selecting in the first must clamp at the boundary.
	f := diffmodel.File{
		Status: diffmodel.StatusModified, PrePath: "a", PostPath: "a",
		Hunks: []diffmodel.Hunk{
			{
				BaseStart: 1, BaseLines: 2, HeadStart: 1, HeadLines: 2,
				Lines: []diffmodel.Line{{Prefix: ' ', Text: "a"}, {Prefix: ' ', Text: "b"}},
			},
			{
				BaseStart: 10, BaseLines: 1, HeadStart: 10, HeadLines: 1,
				Lines: []diffmodel.Line{{Prefix: ' ', Text: "c"}},
			},
		},
	}
	m := New([]diffmodel.File{f}, review.Review{})
	// rows: 0=file, 1=hunk1, 2=line a, 3=line b, 4=hunk2, 5=line c
	for i := 0; i < 2; i++ {
		m, _ = applyKey(m, "j")
	}
	// cursor=2 (line a)
	m, _ = applyKey(m, "V")
	for i := 0; i < 5; i++ {
		m, _ = applyKey(m, "j")
	}
	sel := m.SelectionState()
	if sel == nil {
		t.Fatal("selection lost")
	}
	if sel.Extent != 3 {
		t.Errorf("selection escaped hunk: extent=%d, want 3", sel.Extent)
	}
}
