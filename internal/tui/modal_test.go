package tui

import (
	"strings"
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

// TestModal_RangePersistsRealDiffLineNumbers locks in that range anchors carry
// the head-side diff line numbers (not row indices). Regression guard for the
// earlier bug where LineStart/LineEnd were row offsets and would drift once
// hunk headers / multiple files shifted the row stream.
func TestModal_RangePersistsRealDiffLineNumbers(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := New(files, review.Review{})
	// rows: 0 file header, 1 hunk header, 2 ` x` (head=1), 3 `+y` (head=2).
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "V")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	m = typeBody(m, "rng")
	m = modalSendCtrlS(m)

	if got := len(m.Review.Comments); got != 1 {
		t.Fatalf("expected 1 comment, got %d", got)
	}
	c := m.Review.Comments[0]
	if c.Kind != review.KindRange {
		t.Fatalf("kind=%q, want range", c.Kind)
	}
	if c.LineStart != 1 || c.LineEnd != 2 {
		t.Errorf("range lines = (%d,%d), want (1,2) — head-side diff numbers",
			c.LineStart, c.LineEnd)
	}
	if c.Side != review.SideHead {
		t.Errorf("side=%q, want head", c.Side)
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

func TestModal_LineExcerptShowsAnchoredRow(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := New(files, review.Review{})
	m, _ = applyKey(m, "j") // hunk header
	m, _ = applyKey(m, "j") // first content line (head=1, " x")
	m, _ = applyKey(m, "c")
	if m.Modal() == nil {
		t.Fatal("modal precondition failed")
	}
	v := stripANSI(m.View())
	if !strings.Contains(v, "1   x") {
		t.Errorf("line excerpt missing the anchored row in modal view:\n%s", v)
	}
}

func TestModal_RangeExcerptCoversSelection(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{twoLineHunkFile()}
	m := New(files, review.Review{})
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "V")
	m, _ = applyKey(m, "j") // extend over head=1..2
	m, _ = applyKey(m, "c")
	if m.Modal() == nil {
		t.Fatal("modal precondition failed")
	}
	v := stripANSI(m.View())
	if !strings.Contains(v, "1   x") {
		t.Errorf("range excerpt missing line 1 in modal view:\n%s", v)
	}
	if !strings.Contains(v, "2 + y") {
		t.Errorf("range excerpt missing line 2 in modal view:\n%s", v)
	}
}

func TestModal_FileKindHasNoExcerpt(t *testing.T) {
	t.Parallel()
	f := twoLineHunkFile()
	a := review.Anchor{Kind: review.KindFile, Path: f.DisplayPath(), Side: review.SideHead}
	if got := commentExcerpt(f, a); got != nil {
		t.Errorf("file-kind anchor should have no excerpt, got %+v", got)
	}
	if got := commentExcerpt(f, review.Anchor{Kind: review.KindReview}); got != nil {
		t.Errorf("review-kind anchor should have no excerpt, got %+v", got)
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
