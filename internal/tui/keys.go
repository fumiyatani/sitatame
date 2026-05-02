package tui

// Key bindings for the TUI. Strings are matched against tea.KeyMsg.String().
//
// Only the bindings active in T11 are listed here. T12+ will extend the set
// (j/k/n/p navigation, V selection, c / R comments, s save).
const (
	KeyQuit     = "q"
	KeyQuitCtrl = "ctrl+c"
	KeyHelp     = "?"
	KeyEsc      = "esc"
)
