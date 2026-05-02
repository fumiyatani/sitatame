package tui

// Key bindings for the TUI. Strings are matched against tea.KeyMsg.String().
//
// Bindings active so far. Selection / comment / save bindings land in T15-T17.
const (
	KeyQuit     = "q"
	KeyQuitCtrl = "ctrl+c"
	KeyHelp     = "?"
	KeyEsc      = "esc"
	KeyDown     = "j"
	KeyUp       = "k"
	KeyNextFile = "n"
	KeyPrevFile = "p"
)
