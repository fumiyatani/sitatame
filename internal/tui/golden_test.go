package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite golden files instead of comparing")

// goldenSnapshot returns m.View() with ANSI removed and per-line trailing
// whitespace trimmed so platform / textarea cursor noise doesn't make goldens
// flaky.
func goldenSnapshot(m Model) string {
	v := stripANSI(m.View())
	lines := strings.Split(v, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

func setSize(m Model, w, h int) Model {
	upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return upd.(Model)
}

func runGolden(t *testing.T, name string, m Model) {
	t.Helper()
	got := goldenSnapshot(m)
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s — run `go test -update-golden ./internal/tui/...` to seed", path)
	}
	if string(want) != got {
		t.Errorf("golden mismatch for %s\nwant:\n%s\n----\ngot:\n%s", name, string(want), got)
	}
}

func TestGolden_SmallDiff(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{makeFile("a.go", []byte{' ', '+', '-'})}
	m := setSize(New(files, review.Review{}), 60, 12)
	runGolden(t, "small_diff", m)
}

func TestGolden_RangeSelection(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{makeFile("a.go", []byte{' ', ' ', ' '})}
	m := setSize(New(files, review.Review{}), 60, 12)
	m, _ = applyKey(m, "j") // hunk header
	m, _ = applyKey(m, "j") // first line
	m, _ = applyKey(m, "r")
	m, _ = applyKey(m, "j") // extend
	runGolden(t, "range_selection", m)
}

func TestGolden_LargeScroll(t *testing.T) {
	t.Parallel()
	var lines []diffmodel.Line
	for i := 0; i < 200; i++ {
		lines = append(lines, diffmodel.Line{Prefix: ' ', Text: "ctx"})
	}
	f := diffmodel.File{
		Status: diffmodel.StatusModified, PrePath: "big", PostPath: "big",
		Hunks: []diffmodel.Hunk{{
			BaseStart: 1, BaseLines: 200, HeadStart: 1, HeadLines: 200, Lines: lines,
		}},
	}
	diffmodel.AssignLineNumbers(&f.Hunks[0])
	m := setSize(New([]diffmodel.File{f}, review.Review{}), 50, 10)
	for i := 0; i < 100; i++ {
		m, _ = applyKey(m, "j")
	}
	runGolden(t, "large_scroll", m)
}

func TestGolden_BinaryFile(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		{Status: diffmodel.StatusModified, PrePath: "img.bin", PostPath: "img.bin", Binary: true},
	}
	m := setSize(New(files, review.Review{}), 60, 8)
	runGolden(t, "binary_file", m)
}

func TestGolden_StaleOverlay(t *testing.T) {
	t.Parallel()
	f := numberedFile("a.go", "a.go", "b1", "b2", 3)
	r := review.Review{Comments: []review.Comment{{
		Anchor: review.Anchor{Kind: review.KindLine, Path: "a.go", Side: review.SideHead, Line: 2, Blob: "missing"},
		State:  review.StateStale,
		Body:   "drifted",
	}}}
	m := setSize(New([]diffmodel.File{f}, r), 60, 12)
	runGolden(t, "stale_overlay", m)
}

// TestGolden_LongLineWrap pins the wrap layout for a source line that is wider
// than the viewport body budget. Width=80, the single context line has 160
// chars, so the renderer wraps it across multiple screen rows instead of
// clipping with an ellipsis. The golden captures the exact wrap boundaries and
// the continuation-line indent convention (space prefix on continuation rows).
func TestGolden_LongLineWrap(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("a", 80) + strings.Repeat("b", 80)
	h := diffmodel.Hunk{
		BaseStart: 1, BaseLines: 1, HeadStart: 1, HeadLines: 1,
		Lines: []diffmodel.Line{{Prefix: ' ', Text: body}},
	}
	diffmodel.AssignLineNumbers(&h)
	f := diffmodel.File{
		Status:  diffmodel.StatusModified,
		PrePath: "wide.go", PostPath: "wide.go",
		Hunks: []diffmodel.Hunk{h},
	}
	m := setSize(New([]diffmodel.File{f}, review.Review{}), 80, 12)
	runGolden(t, "long_wrap", m)
}

func BenchmarkUpdate_LargeDiff(b *testing.B) {
	var hunks []diffmodel.Hunk
	for hi := 0; hi < 500; hi++ {
		var lines []diffmodel.Line
		for j := 0; j < 10; j++ {
			lines = append(lines, diffmodel.Line{Prefix: ' ', Text: "context line content"})
		}
		hunks = append(hunks, diffmodel.Hunk{
			BaseStart: hi*10 + 1, BaseLines: 10, HeadStart: hi*10 + 1, HeadLines: 10, Lines: lines,
		})
	}
	f := diffmodel.File{
		Status: diffmodel.StatusModified, PrePath: "big", PostPath: "big", Hunks: hunks,
	}
	for i := range f.Hunks {
		diffmodel.AssignLineNumbers(&f.Hunks[i])
	}
	m := New([]diffmodel.File{f}, review.Review{})
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = upd.(Model)

	jKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updated, _ := m.Update(jKey)
		m = updated.(Model)
		_ = m.View()
	}
}
