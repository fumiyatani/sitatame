//go:build !darwin && !linux

package termcheck

// IsTerminal returns false on unsupported platforms; the MVP targets
// only darwin and linux per PRD non-goals.
func IsTerminal(fd uintptr) bool { return false }
