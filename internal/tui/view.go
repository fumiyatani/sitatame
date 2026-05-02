package tui

import (
	"fmt"
	"strings"
)

// mainView renders the placeholder diff body for T11. Full diff rendering with
// virtual scroll, ANSI sanitization, and CJK width handling lands in T12-T13.
func mainView(m Model) string {
	var b strings.Builder
	if len(m.Files) == 0 {
		b.WriteString("(no diff)\n")
	} else {
		fmt.Fprintf(&b, "%d files changed\n", len(m.Files))
		for _, f := range m.Files {
			fmt.Fprintf(&b, "  %s %s\n", f.Status, f.DisplayPath())
		}
	}
	b.WriteString("\n")
	b.WriteString("? help · q quit")
	return b.String()
}
