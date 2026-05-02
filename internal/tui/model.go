package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tanifumiya/sitatame/internal/diffmodel"
	"github.com/tanifumiya/sitatame/internal/review"
)

// Model is the bubbletea model for the diff review TUI.
//
// T11 covers the skeleton: window-size handling, q to quit, ? to toggle help.
// Navigation, selection, comment modal, and save flow arrive in T12+.
type Model struct {
	Files  []diffmodel.File
	Review review.Review

	rows      []row
	cursor    int
	top       int
	width     int
	height    int
	showHelp  bool
	quitting  bool
	selection *Selection
}

func New(files []diffmodel.File, r review.Review) Model {
	return Model{
		Files:  files,
		Review: r,
		rows:   buildRows(files),
		// Reasonable default until the first WindowSizeMsg arrives so View()
		// before the first resize still produces a non-empty body.
		height: 24,
		width:  80,
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	return mainView(m)
}

// Quitting reports whether the model has signaled tea.Quit. Exposed for tests.
func (m Model) Quitting() bool { return m.quitting }

// ShowingHelp reports whether the help modal is currently visible. Exposed for tests.
func (m Model) ShowingHelp() bool { return m.showHelp }
