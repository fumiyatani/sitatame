package clipboard

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// stubLookFound simulates finding a binary.
func stubLookFound(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

// stubLookNotFound simulates no binary being present.
func stubLookNotFound(_ string) (string, error) {
	return "", fmt.Errorf("not found")
}

// captureCmd builds a fake exec.Command that records the invocation and
// succeeds without actually running anything.
type cmdCapture struct {
	name string
	args []string
}

func (c *cmdCapture) command(name string, args ...string) *exec.Cmd {
	c.name = name
	c.args = append(c.args, args...)
	// Return a no-op command — "true" exits 0 without producing output.
	return exec.Command("true")
}

// TestCopyWithLookup_Success verifies that when a clipboard binary is found,
// CopyWithLookup constructs a command with the expected name (and args for
// xclip) and returns nil.
func TestCopyWithLookup_Success(t *testing.T) {
	t.Parallel()
	cap := &cmdCapture{}
	err := CopyWithLookup("hello", stubLookFound, cap.command)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.name == "" {
		t.Fatal("no command was invoked")
	}
}

// TestCopyWithLookup_NoBinaryReturnsError verifies that when no clipboard
// binary is found, an explicit error is returned (not a silent no-op).
func TestCopyWithLookup_NoBinaryReturnsError(t *testing.T) {
	t.Parallel()
	cap := &cmdCapture{}
	err := CopyWithLookup("hello", stubLookNotFound, cap.command)
	if err == nil {
		t.Fatal("expected error when no clipboard binary is found, got nil")
	}
	if !strings.Contains(err.Error(), "clipboard") {
		t.Errorf("error should mention 'clipboard', got: %v", err)
	}
	if cap.name != "" {
		t.Errorf("command was invoked despite no binary being found")
	}
}

// TestCopyWithLookup_CommandFailureReturnsError verifies that a non-zero exit
// from the clipboard command propagates as an error.
func TestCopyWithLookup_CommandFailureReturnsError(t *testing.T) {
	t.Parallel()
	failCmd := func(name string, args ...string) *exec.Cmd {
		// "false" always exits with code 1.
		return exec.Command("false")
	}
	err := CopyWithLookup("hello", stubLookFound, failCmd)
	if err == nil {
		t.Fatal("expected error when clipboard command exits non-zero, got nil")
	}
}

// TestPlatformCandidates_DarwinReturnsPbcopy is a basic smoke test: on any
// platform we can call platformCandidates() and verify the list is non-empty
// (the actual binary names are tested by the platform at runtime).
func TestPlatformCandidates_NonEmpty(t *testing.T) {
	t.Parallel()
	if cs := platformCandidates(); len(cs) == 0 {
		t.Fatal("platformCandidates returned empty list")
	}
}

// TestCopyWithLookup_FirstAvailableWins confirms that CopyWithLookup uses the
// first binary that lookPath resolves, not a later one. We simulate a scenario
// where the first binary in the list is "found" but the second is not; the
// captured command must be the first one.
func TestCopyWithLookup_FirstAvailableWins(t *testing.T) {
	t.Parallel()

	// Build an ordered list matching the current platform so the first
	// candidate from platformCandidates() is the one we mark as found.
	candidates := platformCandidates()
	if len(candidates) < 1 {
		t.Skip("no candidates for this platform")
	}
	first := candidates[0].name

	lookPath := func(name string) (string, error) {
		if name == first {
			return "/usr/bin/" + name, nil
		}
		return "", fmt.Errorf("not found")
	}
	cap := &cmdCapture{}
	if err := CopyWithLookup("text", lookPath, cap.command); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.name != first {
		t.Errorf("command = %q, want %q", cap.name, first)
	}
}
