package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// modal holds the state for the comment input dialog.
//
// kind is decided at open time from the cursor / selection / explicit `R`
// trigger; it doesn't change while the modal is open. anchor carries enough
// context to materialize a review.Comment on confirm.
type modal struct {
	kind   review.Kind
	anchor review.Anchor
	file   diffmodel.File
	ta     textarea.Model
}

func newModal(kind review.Kind, anchor review.Anchor, file diffmodel.File, initial string) modal {
	ta := textarea.New()
	ta.Placeholder = "comment body — Ctrl+S to save, Esc to cancel"
	ta.SetValue(initial)
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(8)
	return modal{kind: kind, anchor: anchor, file: file, ta: ta}
}

// openCommentModal seeds a modal based on the current cursor / selection.
// Returns false when no anchor location can be derived (e.g. empty diff).
func (m *Model) openCommentModal() bool {
	if len(m.rows) == 0 || m.cursor >= len(m.rows) {
		return false
	}
	r := m.rows[m.cursor]
	if r.fileIdx < 0 || r.fileIdx >= len(m.Files) {
		return false
	}
	f := m.Files[r.fileIdx]

	var kind review.Kind
	anchor := review.Anchor{
		AnchorID: uuid.NewString(),
		Path:     f.DisplayPath(),
		Side:     review.SideHead,
	}
	if f.Status == diffmodel.StatusDeleted {
		anchor.Side = review.SideBase
	}

	switch {
	case m.selection != nil && r.fileIdx == m.selection.FileIdx:
		kind = review.KindRange
		startLine, endLine := selectionLines(m.rows, *m.selection, f, anchor.Side)
		anchor.Kind = review.KindRange
		anchor.LineStart = startLine
		anchor.LineEnd = endLine
		anchor.Blob = blobForSide(f, anchor.Side)
	case f.Binary, r.kind == rowFileHeader:
		kind = review.KindFile
		anchor.Kind = review.KindFile
		anchor.Blob = blobForSide(f, anchor.Side)
	case r.kind == rowLine:
		kind = review.KindLine
		anchor.Kind = review.KindLine
		// Use the corresponding side's line number when available.
		ln := lineNumberAt(f, r.hunkIdx, r.lineIdx, anchor.Side)
		anchor.Line = ln
		anchor.Blob = blobForSide(f, anchor.Side)
	default:
		// Hunk header etc. — fall back to file scope.
		kind = review.KindFile
		anchor.Kind = review.KindFile
		anchor.Blob = blobForSide(f, anchor.Side)
	}
	if f.RenameFrom != "" {
		anchor.RenameFrom = f.RenameFrom
		anchor.RenameTo = f.RenameTo
		anchor.Similarity = f.Similarity
	}

	mm := newModal(kind, anchor, f, "")
	m.modal = &mm
	return true
}

// openReviewModal opens the `R` review-level comment editor and pre-loads the
// existing top-level comment so the user can edit in place.
func (m *Model) openReviewModal() {
	mm := newModal(review.KindReview, review.Anchor{Kind: review.KindReview}, diffmodel.File{}, m.Review.ReviewComment)
	m.modal = &mm
}

// confirmModal turns the textarea content into a Comment and appends it to the
// review (or stores it as ReviewComment for kind=review). Returns the saved
// Comment for tests.
func (m *Model) confirmModal() *review.Comment {
	if m.modal == nil {
		return nil
	}
	body := strings.TrimRight(m.modal.ta.Value(), "\n")
	if m.modal.kind == review.KindReview {
		m.Review.ReviewComment = body
		m.modal = nil
		return nil
	}
	c := review.Comment{
		Anchor: m.modal.anchor,
		State:  review.StateOpen,
		Body:   body,
	}
	c.Kind = m.modal.kind
	m.Review.Comments = append(m.Review.Comments, c)
	// overlay を再構築しないと、追加したコメントのマーカー / 着色が
	// 次の View() に反映されない。
	m.overlay = buildOverlay(m.rows, m.Files, m.Review.Comments)
	m.modal = nil
	// Range selections finish on confirm.
	m.selection = nil
	return &m.Review.Comments[len(m.Review.Comments)-1]
}

func (m *Model) cancelModal() { m.modal = nil }

// Modal returns the open modal or nil. Test-only accessor.
func (m Model) Modal() *modal { return m.modal }

// Kind returns the modal's intended comment kind. Test-only accessor.
func (mo *modal) Kind() review.Kind { return mo.kind }

// AnchorState returns the modal's pending anchor. Test-only accessor.
func (mo *modal) AnchorState() review.Anchor { return mo.anchor }

// Body returns the textarea content. Test-only accessor.
func (mo *modal) Body() string { return mo.ta.Value() }

// modalView renders the modal as a header line + textarea body. Kept simple:
// the underlying diff is hidden while the modal is open so tests can assert
// modal state without competing with the viewport.
func modalView(m Model) string {
	var header string
	switch m.modal.kind {
	case review.KindReview:
		header = "review-level comment"
	case review.KindFile:
		header = "file comment: " + m.modal.anchor.Path
	case review.KindLine:
		header = "line comment: " + m.modal.anchor.Path
	case review.KindRange:
		header = "range comment: " + m.modal.anchor.Path
	default:
		header = "comment"
	}
	var b strings.Builder
	b.WriteString(header)
	b.WriteByte('\n')
	if ex := commentExcerpt(m.modal.file, m.modal.anchor); len(ex) > 0 {
		b.WriteString(renderExcerpt(ex))
		b.WriteString("\n\n")
	}
	b.WriteString(m.modal.ta.View())
	b.WriteString("\nCtrl+S save · Esc cancel")
	return b.String()
}

// excerptLine carries one row of the modal's "what you commented on" preview.
type excerptLine struct {
	number int
	prefix byte
	text   string
}

// commentExcerpt gathers the diff lines covered by the modal's anchor so the
// user can see what they're commenting on without leaving the input. Returns
// nil for review/file kinds since those don't pin to specific rows.
func commentExcerpt(f diffmodel.File, a review.Anchor) []excerptLine {
	var lo, hi int
	switch a.Kind {
	case review.KindLine:
		lo, hi = a.Line, a.Line
	case review.KindRange:
		lo, hi = a.LineStart, a.LineEnd
	default:
		return nil
	}
	if lo == 0 || hi == 0 {
		return nil
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	if out := collectExcerpt(f, a.Side, lo, hi); len(out) > 0 {
		return out
	}
	// Fall back to the opposite side: a line comment on a deleted-only row
	// stores anchor.Side=head with anchor.Line=base_line because lineNumberAt
	// borrows the available number. Without this, the excerpt would be empty
	// even though the user pointed at a real row.
	other := review.SideBase
	if a.Side == review.SideBase {
		other = review.SideHead
	}
	return collectExcerpt(f, other, lo, hi)
}

func collectExcerpt(f diffmodel.File, side review.Side, lo, hi int) []excerptLine {
	var out []excerptLine
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			ln := l.HeadLine
			if side == review.SideBase {
				ln = l.BaseLine
			}
			if ln == 0 || ln < lo || ln > hi {
				continue
			}
			prefix := l.Prefix
			if prefix == 0 {
				prefix = ' '
			}
			out = append(out, excerptLine{number: ln, prefix: prefix, text: l.Text})
		}
	}
	return out
}

func renderExcerpt(lines []excerptLine) string {
	width := 1
	for _, l := range lines {
		if n := len(strconv.Itoa(l.number)); n > width {
			width = n
		}
	}
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%*d %c %s", width, l.number, l.prefix, l.text)
	}
	return b.String()
}

// updateModal forwards key events to the embedded textarea while the modal is
// open. Ctrl+S confirms; Esc cancels. Confirm stays modifier-bound so the
// textarea can accept any printable rune in the comment body.
func (m *Model) updateModal(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+s":
			m.confirmModal()
			return nil
		case "esc":
			m.cancelModal()
			return nil
		}
	}
	var cmd tea.Cmd
	m.modal.ta, cmd = m.modal.ta.Update(msg)
	return cmd
}

func selectionLines(rows []row, sel Selection, f diffmodel.File, side review.Side) (int, int) {
	lo, hi := sel.Range()
	startLine := rowLineNumber(rows, lo, f, side)
	endLine := rowLineNumber(rows, hi, f, side)
	if startLine == 0 || endLine == 0 {
		return startLine, endLine
	}
	if startLine > endLine {
		startLine, endLine = endLine, startLine
	}
	return startLine, endLine
}

// rowLineNumber resolves a row index to a real diff line number on the given
// side. Returns 0 for non-line rows or when the side has no number (e.g. a
// head-side query on a deleted-only row).
func rowLineNumber(rows []row, idx int, f diffmodel.File, side review.Side) int {
	if idx < 0 || idx >= len(rows) {
		return 0
	}
	r := rows[idx]
	if r.kind != rowLine {
		return 0
	}
	return lineNumberAt(f, r.hunkIdx, r.lineIdx, side)
}

func lineNumberAt(f diffmodel.File, hunkIdx, lineIdx int, side review.Side) int {
	if hunkIdx < 0 || hunkIdx >= len(f.Hunks) {
		return 0
	}
	h := f.Hunks[hunkIdx]
	if lineIdx < 0 || lineIdx >= len(h.Lines) {
		return 0
	}
	l := h.Lines[lineIdx]
	if side == review.SideBase {
		if l.BaseLine != 0 {
			return l.BaseLine
		}
		return l.HeadLine
	}
	if l.HeadLine != 0 {
		return l.HeadLine
	}
	return l.BaseLine
}

func blobForSide(f diffmodel.File, side review.Side) string {
	if side == review.SideBase {
		return f.BlobBase
	}
	return f.BlobHead
}
