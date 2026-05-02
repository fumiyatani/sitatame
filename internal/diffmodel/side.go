package diffmodel

// SideFromPrefix maps a unified-diff line prefix to the side it belongs to.
// `+` is head (postimage), `-` is base (preimage), and context ` ` is treated
// as head so cursor-on-context comments anchor to the current revision.
func SideFromPrefix(p byte) Side {
	if p == '-' {
		return SideBase
	}
	return SideHead
}

// AssignLineNumbers fills BaseLine / HeadLine for every Line in the hunk based
// on the hunk header's BaseStart / HeadStart and the line prefixes.
//
//   - ' ' (context): both base and head advance
//   - '-' (deletion): only base advances; HeadLine is left zero
//   - '+' (addition): only head advances; BaseLine is left zero
//
// Lines with an unrecognized prefix are skipped without advancing counters.
func AssignLineNumbers(h *Hunk) {
	base := h.BaseStart
	head := h.HeadStart
	for i := range h.Lines {
		l := &h.Lines[i]
		switch l.Prefix {
		case ' ':
			l.BaseLine = base
			l.HeadLine = head
			base++
			head++
		case '-':
			l.BaseLine = base
			base++
		case '+':
			l.HeadLine = head
			head++
		}
	}
}
