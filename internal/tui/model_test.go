package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tanifumiya/sitatame/internal/diffmodel"
	"github.com/tanifumiya/sitatame/internal/review"
)

func sendKey(m Model, s string) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return updated.(Model)
}

func sendNamedKey(m Model, t tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: t})
	return updated.(Model)
}

func TestModel_HelpToggle(t *testing.T) {
	t.Parallel()
	m := New(nil, review.Review{})
	if m.ShowingHelp() {
		t.Fatal("help should start hidden")
	}
	m = sendKey(m, "?")
	if !m.ShowingHelp() {
		t.Fatal("`?` did not show help")
	}
	if !strings.Contains(m.View(), "sitatame") {
		t.Errorf("help view missing header: %q", m.View())
	}
	m = sendNamedKey(m, tea.KeyEsc)
	if m.ShowingHelp() {
		t.Fatal("Esc did not close help")
	}
}

func TestModel_QuitsOnQ(t *testing.T) {
	t.Parallel()
	m := New(nil, review.Review{})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !updated.(Model).Quitting() {
		t.Errorf("model not marked quitting")
	}
	if cmd == nil {
		t.Errorf("expected tea.Quit cmd")
	}
}

func TestModel_WindowSize(t *testing.T) {
	t.Parallel()
	m := New(nil, review.Review{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mm := updated.(Model)
	if mm.width != 120 || mm.height != 40 {
		t.Errorf("window size not stored: %+v", mm)
	}
}

func TestMainView_ShowsFiles(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		{Status: diffmodel.StatusModified, PostPath: "a.go"},
		{Status: diffmodel.StatusAdded, PostPath: "b.go"},
	}
	m := New(files, review.Review{})
	v := m.View()
	if !strings.Contains(v, "2 files changed") {
		t.Errorf("file count missing: %q", v)
	}
	if !strings.Contains(v, "a.go") || !strings.Contains(v, "b.go") {
		t.Errorf("file names missing: %q", v)
	}
}
