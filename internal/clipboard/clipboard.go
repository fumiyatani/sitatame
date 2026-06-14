// Package clipboard provides a single Copy function that writes text to the
// system clipboard using a platform-appropriate command-line tool.
//
// Supported backends (tried in order on Linux):
//
//	macOS:   pbcopy
//	Linux:   wl-copy (Wayland), xclip -selection clipboard, xsel --clipboard --input
//	Windows: clip.exe
//
// When no clipboard binary is found, Copy returns an explicit error rather
// than silently succeeding. Callers that treat clipboard copy as best-effort
// should log the error and continue.
package clipboard

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Copy writes text to the system clipboard. It returns an error if no
// clipboard command is available or if the command fails.
//
// The function is intentionally pure (no global state, no singletons) so that
// tests can swap the underlying exec.LookPath / exec.Command via the
// CopyWithLookup helper without patching package-level variables.
func Copy(text string) error {
	return CopyWithLookup(text, exec.LookPath, exec.Command)
}

// CopyWithLookup is the testable core of Copy. It accepts a lookPath function
// (exec.LookPath or a stub) and a commandFunc (exec.Command or a stub) so
// unit tests can avoid spawning real processes.
func CopyWithLookup(
	text string,
	lookPath func(string) (string, error),
	commandFunc func(string, ...string) *exec.Cmd,
) error {
	name, args, err := clipboardCommand(lookPath)
	if err != nil {
		return err
	}
	cmd := commandFunc(name, args...)
	cmd.Stdin = strings.NewReader(text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// clipboardCommand returns the binary name and arguments for the platform
// clipboard command available on the current system. It returns an error when
// no suitable binary can be found.
func clipboardCommand(lookPath func(string) (string, error)) (string, []string, error) {
	candidates := platformCandidates()
	for _, c := range candidates {
		if _, err := lookPath(c.name); err == nil {
			return c.name, c.args, nil
		}
	}
	return "", nil, fmt.Errorf("clipboard: no clipboard command found (tried: %s)", joinNames(candidates))
}

type candidate struct {
	name string
	args []string
}

// platformCandidates returns the ordered list of clipboard backends for the
// current OS. macOS and Windows each have a single well-known binary; Linux
// tries Wayland first (wl-copy), then X11 (xclip, xsel).
func platformCandidates() []candidate {
	switch runtime.GOOS {
	case "darwin":
		return []candidate{
			{name: "pbcopy"},
		}
	case "windows":
		return []candidate{
			{name: "clip.exe"},
		}
	default: // linux and others
		return []candidate{
			{name: "wl-copy"},
			{name: "xclip", args: []string{"-selection", "clipboard"}},
			{name: "xsel", args: []string{"--clipboard", "--input"}},
		}
	}
}

func joinNames(cs []candidate) string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.name
	}
	return strings.Join(names, ", ")
}
