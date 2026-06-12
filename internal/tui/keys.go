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
	// Arrow-key aliases for the j/k/n/p navigation set. bubbletea v1.3.10
	// reports arrow keys as these literal strings via tea.KeyMsg.String(),
	// so they slot straight into the existing String()-based dispatch in
	// Update without any extra normalization.
	KeyDownArrow  = "down"
	KeyUpArrow    = "up"
	KeyRightArrow = "right"
	KeyLeftArrow  = "left"
	KeySelectKey   = "r"
	KeySave        = "s"
	KeyToggleLayout = "tab"
	KeyResolveToggle = "x"
)

// mouseWheelStep is the number of rows the viewport scrolls per wheel tick.
// Chosen to feel responsive without overshooting on a single notch.
const mouseWheelStep = 3
