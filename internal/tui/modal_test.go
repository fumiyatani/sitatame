package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// modalSendCtrlS confirms an open modal via the same key path Update uses.
func modalSendCtrlS(m Model) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	return updated.(Model)
}

func modalSendEsc(m Model) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	return updated.(Model)
}

// typeBody injects characters into the textarea by sending KeyRunes for each
// rune. Each call to Update goes through updateModal -> textarea.Update.
func typeBody(m Model, body string) Model {
	for _, r := range body {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	return m
}

func TestModal_FileKindOnFileHeader(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := New(files, review.Review{})
	// cursor starts at row 0 (file header).
	m, _ = applyKey(m, "c")
	mo := m.Modal()
	if mo == nil {
		t.Fatalf("c on file header should open modal")
	}
	if mo.Kind() != review.KindFile {
		t.Errorf("kind=%q, want file", mo.Kind())
	}
	if mo.AnchorState().Path != "a.go" {
		t.Errorf("anchor path=%q", mo.AnchorState().Path)
	}
}

func TestModal_LineKindOnContentLine(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := New(files, review.Review{})
	// rows: 0 file header, 1 hunk header, 2 first content line.
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	mo := m.Modal()
	if mo == nil {
		t.Fatalf("c on line should open modal")
	}
	if mo.Kind() != review.KindLine {
		t.Errorf("kind=%q, want line", mo.Kind())
	}

	m = typeBody(m, "looks ok")
	m = modalSendCtrlS(m)
	if m.Modal() != nil {
		t.Errorf("Ctrl+S should close modal")
	}
	if got := len(m.Review.Comments); got != 1 {
		t.Fatalf("expected 1 comment appended, got %d", got)
	}
	c := m.Review.Comments[0]
	if c.Kind != review.KindLine || c.Body != "looks ok" || c.State != review.StateOpen {
		t.Errorf("saved comment wrong: %+v", c)
	}
}

func TestModal_RangeKindAfterSelection(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := New(files, review.Review{})
	// Move into the first content line.
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "V")
	m, _ = applyKey(m, "j")
	if m.SelectionState() == nil {
		t.Fatalf("selection precondition failed")
	}
	m, _ = applyKey(m, "c")
	mo := m.Modal()
	if mo == nil {
		t.Fatalf("c with selection should open modal")
	}
	if mo.Kind() != review.KindRange {
		t.Errorf("kind=%q, want range", mo.Kind())
	}
	a := mo.AnchorState()
	if a.LineStart == 0 || a.LineEnd == 0 || a.LineStart > a.LineEnd {
		t.Errorf("range anchor lines invalid: start=%d end=%d", a.LineStart, a.LineEnd)
	}

	m = typeBody(m, "rng")
	m = modalSendCtrlS(m)
	if m.SelectionState() != nil {
		t.Errorf("range confirm should clear selection")
	}
	if len(m.Review.Comments) != 1 || m.Review.Comments[0].Kind != review.KindRange {
		t.Errorf("range comment not appended: %+v", m.Review.Comments)
	}
}

func TestModal_BinaryFileForcesFileKind(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		{Status: diffmodel.StatusModified, PrePath: "img.bin", PostPath: "img.bin", Binary: true},
	}
	m := New(files, review.Review{})
	// row 0 = file header, row 1 = binary placeholder.
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	mo := m.Modal()
	if mo == nil {
		t.Fatalf("c on binary placeholder should open modal")
	}
	if mo.Kind() != review.KindFile {
		t.Errorf("kind=%q, want file (binary cannot range/line)", mo.Kind())
	}
}

func TestModal_ReviewKindOnR(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	r := review.Review{ReviewComment: "draft body"}
	m := New(files, r)
	m, _ = applyKey(m, "R")
	mo := m.Modal()
	if mo == nil {
		t.Fatalf("R should open review modal")
	}
	if mo.Kind() != review.KindReview {
		t.Errorf("kind=%q, want review", mo.Kind())
	}
	if mo.Body() != "draft body" {
		t.Errorf("review modal must preload existing review_comment, got %q", mo.Body())
	}

	m = typeBody(m, " edited")
	m = modalSendCtrlS(m)
	if got := m.Review.ReviewComment; got != "draft body edited" {
		t.Errorf("ReviewComment=%q, want %q", got, "draft body edited")
	}
	if len(m.Review.Comments) != 0 {
		t.Errorf("review-kind must not append to Comments: %+v", m.Review.Comments)
	}
}

func TestModal_EscCancelsWithoutAppend(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := New(files, review.Review{})
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	if m.Modal() == nil {
		t.Fatal("modal precondition failed")
	}
	m = typeBody(m, "discarded")
	m = modalSendEsc(m)
	if m.Modal() != nil {
		t.Errorf("Esc should close modal")
	}
	if len(m.Review.Comments) != 0 {
		t.Errorf("Esc must not append: %+v", m.Review.Comments)
	}
}
