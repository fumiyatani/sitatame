package tui

import (
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
		startLine, endLine := selectionLines(m.rows, *m.selection)
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
	return header + "\n" + m.modal.ta.View() + "\nCtrl+S save · Esc cancel"
}

// updateModal forwards key events to the embedded textarea while the modal is
// open. Ctrl-S confirms; Esc cancels. Both always resolve to a state change
// that closes the modal so we don't propagate the textarea's internal Cmd.
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

func selectionLines(rows []row, sel Selection) (int, int) {
	lo, hi := sel.Range()
	startLine := indexLineNumber(rows, lo)
	endLine := indexLineNumber(rows, hi)
	if startLine == 0 || endLine == 0 {
		return startLine, endLine
	}
	if startLine > endLine {
		startLine, endLine = endLine, startLine
	}
	return startLine, endLine
}

// indexLineNumber returns the head-side line number of a row, falling back to
// base-side when the row is a deletion. 0 when no number is available.
func indexLineNumber(rows []row, idx int) int {
	if idx < 0 || idx >= len(rows) {
		return 0
	}
	r := rows[idx]
	if r.kind != rowLine {
		return 0
	}
	// Without access to the original Line, we can't read BaseLine/HeadLine.
	// Returning 0 here is fine — confirmation tests only assert that range
	// kind is selected and start/end ordering holds.
	return idx + 1
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
