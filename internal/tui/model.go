package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// QuitReason carries why the TUI exited so the caller can decide what to do
// with the in-memory review. The runner exposes the final reason via the
// Model's QuitReason() accessor after tea.Quit.
type QuitReason int

const (
	// QuitNone is the initial value before the user has chosen to leave. The
	// runner treats it like QuitDraft: any uncommitted in-memory review should
	// still be persisted as a draft so work isn't lost on unexpected exits.
	QuitNone QuitReason = iota
	// QuitDraft is set by `q`; the caller saves the review to drafts/ and
	// returns a non-zero exit code.
	QuitDraft
	// QuitPromote is set by `s`; the caller saves a draft, atomically promotes
	// it to reviews/, and prints `SITATAME_REVIEW=<abs>` on stdout.
	QuitPromote
)

// Model is the bubbletea model for the diff review TUI.
//
// T11 covers the skeleton: window-size handling, q to quit, ? to toggle help.
// Navigation, selection, comment modal, and save flow arrive in T12+.
type Model struct {
	Files  []diffmodel.File
	Review review.Review

	rows       []row
	overlay    map[int][]int
	cursor     int
	top        int
	width      int
	height     int
	showHelp   bool
	quitting   bool
	quitReason QuitReason
	selection  *Selection
	modal      *modal
}

func New(files []diffmodel.File, r review.Review) Model {
	rows := buildRows(files)
	return Model{
		Files:   files,
		Review:  r,
		rows:    rows,
		overlay: buildOverlay(rows, files, r.Comments),
		// Reasonable default until the first WindowSizeMsg arrives so View()
		// before the first resize still produces a non-empty body.
		height: 24,
		width:  80,
	}
}

// Overlay returns the row-to-comment-index map. Test-only accessor.
func (m Model) Overlay() map[int][]int { return m.overlay }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.modal != nil {
		cmd := m.updateModal(msg)
		return m, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollToCursor()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case KeyQuit, KeyQuitCtrl:
			m.quitting = true
			m.quitReason = QuitDraft
			return m, tea.Quit
		case KeySave:
			m.quitting = true
			m.quitReason = QuitPromote
			return m, tea.Quit
		case KeyHelp:
			m.showHelp = !m.showHelp
			return m, nil
		case KeyEsc:
			if m.showHelp {
				m.showHelp = false
			}
			m.clearSelection()
			return m, nil
		case KeyDown:
			m.moveCursorBy(1)
			m.extendSelection()
			return m, nil
		case KeyUp:
			m.moveCursorBy(-1)
			m.extendSelection()
			return m, nil
		case KeyNextFile:
			m.jumpFile(1)
			m.clearSelection()
			return m, nil
		case KeyPrevFile:
			m.jumpFile(-1)
			m.clearSelection()
			return m, nil
		case KeySelectKey:
			m.startSelection()
			return m, nil
		case "c":
			m.openCommentModal()
			return m, nil
		case "R":
			m.openReviewModal()
			return m, nil
		}
	}
	return m, nil
}

// Cursor returns the current row index (test-only accessor).
func (m Model) Cursor() int { return m.cursor }

// Top returns the current scroll position (test-only accessor).
func (m Model) Top() int { return m.top }

// Rows returns the number of flat rows (test-only accessor).
func (m Model) Rows() int { return len(m.rows) }

// SelectionState returns the current selection or nil.
func (m Model) SelectionState() *Selection { return m.selection }

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.showHelp {
		return helpView()
	}
	if m.modal != nil {
		return modalView(m)
	}
	return mainView(m)
}

// Quitting reports whether the model has signaled tea.Quit. Exposed for tests.
func (m Model) Quitting() bool { return m.quitting }

// QuitReason returns why the model asked to quit. Defaults to QuitNone before
// any quit key is pressed.
func (m Model) QuitReason() QuitReason { return m.quitReason }

// ShowingHelp reports whether the help modal is currently visible. Exposed for tests.
func (m Model) ShowingHelp() bool { return m.showHelp }
