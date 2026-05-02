package tui

import "strings"

// helpLines is the body of the `?` modal. Keys here mirror keys.go and any
// future bindings; visible-only entries.
var helpLines = []string{
	"sitatame — keys",
	"",
	"  ?         show this help",
	"  q         quit",
	"  esc       close modal / cancel",
	"",
	"(more keys land in upcoming tasks: j/k navigate,",
	" V select range, c comment, R review, s save)",
}

func helpView() string {
	return strings.Join(helpLines, "\n")
}
