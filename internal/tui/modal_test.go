package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// modalSendSave confirms an open modal via the same key path Update uses.
func modalSendSave(m Model) Model {
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
	m = modalSendSave(m)
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
	m, _ = applyKey(m, "r")
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
	m = modalSendSave(m)
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
	m, _ = applyKey(m, "r")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	m = typeBody(m, "rng")
	m = modalSendSave(m)

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
	m = modalSendSave(m)
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
	m, _ = applyKey(m, "r")
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

// modifiedFileWithBlobs is a diffmodel.File modeling a real
// "modified, context + add + delete" hunk with non-empty blob IDs on both
// sides. Layout:
//
//	row 0: file header
//	row 1: hunk header (@@ -1,2 +1,2 @@)
//	row 2: ` x`  → BaseLine=1, HeadLine=1
//	row 3: `+y`  → BaseLine=0, HeadLine=2
//	row 4: `-z`  → BaseLine=2, HeadLine=0
//
// Distinct BlobBase / BlobHead lets the new Side-derivation logic prove it's
// picking the right blob for the row's prefix (issues #36 + #19).
func modifiedFileWithBlobs() diffmodel.File {
	f := diffmodel.File{
		Status:   diffmodel.StatusModified,
		PrePath:  "a.go", PostPath: "a.go",
		BlobBase: "blob-base", BlobHead: "blob-head",
		Hunks: []diffmodel.Hunk{{
			BaseStart: 1, BaseLines: 2, HeadStart: 1, HeadLines: 2,
			Lines: []diffmodel.Line{
				{Prefix: ' ', Text: "x"},
				{Prefix: '+', Text: "y"},
				{Prefix: '-', Text: "z"},
			},
		}},
	}
	diffmodel.AssignLineNumbers(&f.Hunks[0])
	return f
}

// renamedFileWithDeletion mirrors a "rename + edit" scenario where the user
// might want to comment on a deleted line in the renamed file. RenameFrom /
// RenameTo + distinct blobs lets us assert the anchor carries the right
// rename metadata on the base side.
func renamedFileWithDeletion() diffmodel.File {
	f := diffmodel.File{
		Status:     diffmodel.StatusRenamed,
		PrePath:    "old.go",
		PostPath:   "new.go",
		BlobBase:   "blob-old",
		BlobHead:   "blob-new",
		RenameFrom: "old.go",
		RenameTo:   "new.go",
		Similarity: 80,
		Hunks: []diffmodel.Hunk{{
			BaseStart: 1, BaseLines: 2, HeadStart: 1, HeadLines: 2,
			Lines: []diffmodel.Line{
				{Prefix: ' ', Text: "ctx"},
				{Prefix: '-', Text: "gone"},
				{Prefix: '+', Text: "kept"},
			},
		}},
	}
	diffmodel.AssignLineNumbers(&f.Hunks[0])
	return f
}

// TestModal_LineOnDeletedRowAnchorsToBase pins issue #36/#19: commenting on a
// `-` row in a modified file must record Side=base, Line=BaseLine, Blob=BlobBase
// so the anchor is internally consistent (no Side=head + BaseLine mismatch).
func TestModal_LineOnDeletedRowAnchorsToBase(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{modifiedFileWithBlobs()}
	m := New(files, review.Review{})
	// rows: 0 header, 1 hunk hdr, 2 ` x`, 3 `+y`, 4 `-z`.
	for i := 0; i < 4; i++ {
		m, _ = applyKey(m, "j")
	}
	m, _ = applyKey(m, "c")
	mo := m.Modal()
	if mo == nil {
		t.Fatalf("c on deleted row should open modal")
	}
	a := mo.AnchorState()
	if a.Side != review.SideBase {
		t.Errorf("Side=%q, want base for `-` row", a.Side)
	}
	if a.Line != 2 {
		t.Errorf("Line=%d, want BaseLine=2 for `-z` row", a.Line)
	}
	if a.Blob != "blob-base" {
		t.Errorf("Blob=%q, want BlobBase=%q", a.Blob, "blob-base")
	}

	m = typeBody(m, "undo this")
	m = modalSendSave(m)
	if got := len(m.Review.Comments); got != 1 {
		t.Fatalf("expected 1 comment, got %d", got)
	}
	c := m.Review.Comments[0]
	if c.Side != review.SideBase || c.Line != 2 || c.Blob != "blob-base" {
		t.Errorf("saved comment anchor wrong: %+v", c.Anchor)
	}
}

// TestModal_LineOnAddedRowAnchorsToHead pins the `+` row symmetric: Side=head,
// Line=HeadLine, Blob=BlobHead.
func TestModal_LineOnAddedRowAnchorsToHead(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{modifiedFileWithBlobs()}
	m := New(files, review.Review{})
	// rows: 0 header, 1 hunk hdr, 2 ` x`, 3 `+y`.
	for i := 0; i < 3; i++ {
		m, _ = applyKey(m, "j")
	}
	m, _ = applyKey(m, "c")
	mo := m.Modal()
	if mo == nil {
		t.Fatalf("c on added row should open modal")
	}
	a := mo.AnchorState()
	if a.Side != review.SideHead {
		t.Errorf("Side=%q, want head for `+` row", a.Side)
	}
	if a.Line != 2 {
		t.Errorf("Line=%d, want HeadLine=2 for `+y` row", a.Line)
	}
	if a.Blob != "blob-head" {
		t.Errorf("Blob=%q, want BlobHead=%q", a.Blob, "blob-head")
	}
}

// TestModal_LineOnContextRowAnchorsToHead pins context-row behavior: keep the
// existing SideHead default so cursor-on-context comments anchor to the
// current revision.
func TestModal_LineOnContextRowAnchorsToHead(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{modifiedFileWithBlobs()}
	m := New(files, review.Review{})
	// rows: 0 header, 1 hunk hdr, 2 ` x` (context).
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	a := m.Modal().AnchorState()
	if a.Side != review.SideHead {
		t.Errorf("Side=%q, want head for context row", a.Side)
	}
	if a.Line != 1 {
		t.Errorf("Line=%d, want HeadLine=1 for ` x` context row", a.Line)
	}
	if a.Blob != "blob-head" {
		t.Errorf("Blob=%q, want BlobHead=%q", a.Blob, "blob-head")
	}
}

// TestModal_LineOnRenamedDeletedRowCarriesRenameMeta pins rename + `-` row:
// the anchor must carry RenameFrom (the pre-rename path) so the validator can
// follow blob movement, and Side=base.
func TestModal_LineOnRenamedDeletedRowCarriesRenameMeta(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{renamedFileWithDeletion()}
	m := New(files, review.Review{})
	// rows: 0 header, 1 hunk hdr, 2 ` ctx`, 3 `-gone`.
	for i := 0; i < 3; i++ {
		m, _ = applyKey(m, "j")
	}
	m, _ = applyKey(m, "c")
	a := m.Modal().AnchorState()
	if a.Side != review.SideBase {
		t.Errorf("Side=%q, want base for renamed file's `-` row", a.Side)
	}
	if a.Line != 2 {
		t.Errorf("Line=%d, want BaseLine=2 for `-gone` row", a.Line)
	}
	if a.Blob != "blob-old" {
		t.Errorf("Blob=%q, want BlobBase=%q", a.Blob, "blob-old")
	}
	if a.RenameFrom != "old.go" || a.RenameTo != "new.go" {
		t.Errorf("rename meta missing: from=%q to=%q", a.RenameFrom, a.RenameTo)
	}
}

// TestModal_RangeAllDeletedAnchorsToBase: selecting only `-` rows must yield
// Side=base with LineStart..LineEnd referring to BaseLine numbers.
func TestModal_RangeAllDeletedAnchorsToBase(t *testing.T) {
	t.Parallel()
	// Hunk: ` ctx` (b=1,h=1), `-d1` (b=2), `-d2` (b=3), `+a1` (h=2).
	f := diffmodel.File{
		Status:   diffmodel.StatusModified,
		PrePath:  "a.go", PostPath: "a.go",
		BlobBase: "bb", BlobHead: "bh",
		Hunks: []diffmodel.Hunk{{
			BaseStart: 1, BaseLines: 3, HeadStart: 1, HeadLines: 2,
			Lines: []diffmodel.Line{
				{Prefix: ' ', Text: "ctx"},
				{Prefix: '-', Text: "d1"},
				{Prefix: '-', Text: "d2"},
				{Prefix: '+', Text: "a1"},
			},
		}},
	}
	diffmodel.AssignLineNumbers(&f.Hunks[0])
	m := New([]diffmodel.File{f}, review.Review{})
	// rows: 0 hdr, 1 hunk hdr, 2 ctx, 3 -d1, 4 -d2, 5 +a1.
	for i := 0; i < 3; i++ {
		m, _ = applyKey(m, "j")
	}
	// Now on `-d1`. Start range and extend to `-d2`.
	m, _ = applyKey(m, "r")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	a := m.Modal().AnchorState()
	if a.Side != review.SideBase {
		t.Errorf("Side=%q, want base for all-`-` range", a.Side)
	}
	if a.LineStart != 2 || a.LineEnd != 3 {
		t.Errorf("range = (%d,%d), want (2,3) on base side", a.LineStart, a.LineEnd)
	}
	if a.Blob != "bb" {
		t.Errorf("Blob=%q, want BlobBase=%q", a.Blob, "bb")
	}
}

// TestModal_RangeMixedFallsBackToHead pins the mixed-prefix selection rule:
// a range that spans `-` and `+` lines cannot live on one side cleanly, so we
// keep the head default and surface a status-bar warning. Behavior chosen so
// the saved anchor stays self-consistent (Side=head + HeadLine numbers).
func TestModal_RangeMixedFallsBackToHead(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{modifiedFileWithBlobs()}
	m := New(files, review.Review{})
	// rows: 0 hdr, 1 hunk hdr, 2 ` x`, 3 `+y`, 4 `-z`.
	for i := 0; i < 3; i++ {
		m, _ = applyKey(m, "j")
	}
	// Now on `+y`. Start range and extend down into `-z` (mixed).
	m, _ = applyKey(m, "r")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	a := m.Modal().AnchorState()
	if a.Side != review.SideHead {
		t.Errorf("Side=%q, want head for mixed +/- range", a.Side)
	}
	// LineStart must reflect HeadLine on `+y` (=2). LineEnd must be the head
	// number reachable from `-z`; since `-z` has no HeadLine, the helper falls
	// back to BaseLine (existing lineNumberAt behavior). Either way the range
	// is at minimum [2, 2].
	if a.LineStart == 0 {
		t.Errorf("LineStart=0, want non-zero head-side number")
	}
}

// TestModal_DeletedRowSelectionStatus pins the warning surfaced when a mixed
// `-` + `+` range is committed: the status bar carries an explanation so the
// user understands why Side stayed on head.
func TestModal_DeletedRowSelectionStatus(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{modifiedFileWithBlobs()}
	m := New(files, review.Review{})
	for i := 0; i < 3; i++ {
		m, _ = applyKey(m, "j")
	}
	// On `+y`. Range into `-z`.
	m, _ = applyKey(m, "r")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "c")
	if m.statusMsg != mixedRangeMsg {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, mixedRangeMsg)
	}
}
