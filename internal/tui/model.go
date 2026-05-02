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

	width   int
	height  int
	showHelp bool
	quitting bool
}

func New(files []diffmodel.File, r review.Review) Model {
	return Model{Files: files, Review: r}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
			return m, nil
		}
	}
	return m, nil
}

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
