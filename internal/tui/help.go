package tui

import "strings"

// helpLines is the body of the `?` modal. Keys here mirror keys.go and any
// future bindings; visible-only entries.
var helpLines = []string{
	"sitatame — keys",
	"",
	"  j / k     cursor down / up",
	"  n / p     next / previous file",
	"  ↑ ↓ ← →  arrow-key aliases for k / j / p / n",
	"  wheel     scroll the diff (hold Option/Fn to text-select)",
	"  Tab       toggle unified ↔ split (preview)",
	"  r         start range selection (j/k extend, Esc clear) — unified only",
	"  c         comment at the cursor — unified only",
	"  x         toggle resolved on the comment under cursor (stale skipped) — unified only",
	"  Shift+R   review-level comment — unified only",
	"  s         save & promote, print SITATAME_REVIEW=<path>",
	"  q         save as draft and exit 1",
	"  ?         toggle this help",
	"  Esc       close modal / cancel selection",
	"",
	"  Ctrl+S    confirm comment (inside modal)",
}

func helpView() string {
	return strings.Join(helpLines, "\n")
}
