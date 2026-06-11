package tui

// Key bindings for the TUI. Strings are matched against tea.KeyMsg.String().
//
// Bindings active so far. Selection / comment / save bindings land in T15-T17.
const (
	KeyQuit     = "q"
	KeyQuitCtrl = "ctrl+c"
	KeyHelp     = "?"
	KeyEsc      = "esc"
	KeyDown        = "j"
	KeyUp          = "k"
	KeyNextFile    = "n"
	KeyPrevFile    = "p"
	KeySelectKey   = "r"
	KeySave        = "s"
	KeyToggleLayout = "tab"
)

// mouseWheelStep is the number of rows the viewport scrolls per wheel tick.
// Chosen to feel responsive without overshooting on a single notch.
const mouseWheelStep = 3
