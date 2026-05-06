package tui

import "strings"

// helpLines is the body of the `?` modal. Keys here mirror keys.go and any
// future bindings; visible-only entries.
var helpLines = []string{
	"sitatame — keys",
	"",
	"  j / k     cursor down / up",
	"  n / p     next / previous file",
	"  r         start range selection (j/k extend, Esc clear)",
	"  c         comment at the cursor",
	"  R         review-level comment",
	"  s         save & promote, print SITATAME_REVIEW=<path>",
	"  q         save as draft and exit 1",
	"  ?         toggle this help",
	"  Esc       close modal / cancel selection",
	"",
	"  s         confirm comment (inside modal)",
}

func helpView() string {
	return strings.Join(helpLines, "\n")
}
