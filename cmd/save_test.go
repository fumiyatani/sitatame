package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fumiyatani/sitatame/internal/review"
	"github.com/fumiyatani/sitatame/internal/tui"
)

// withSitatameHome points SITATAME_HOME at a fresh temp dir so the test does
// not write into the developer's real ~/.sitatame, and returns the resolved
// per-project root for that test repo (matching what RunRoot will use).
//
// On macOS t.TempDir() returns an unresolved /tmp/... path while `git
// rev-parse --show-toplevel` returns /private/tmp/... — the symlink
// resolution differs. RunRoot keys the project slug off the resolved repo
// root (via gitx.Discover), so we resolve symlinks here too to keep the
// test's expectation aligned with the runtime behaviour.
func withSitatameHome(t *testing.T, repoDir string) (homeDir, projectRoot string) {
	t.Helper()
	homeDir = t.TempDir()
	t.Setenv("SITATAME_HOME", homeDir)
	resolved, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		resolved = repoDir
	}
	projectRoot = filepath.Join(homeDir, review.ProjectSlug(resolved))
	return homeDir, projectRoot
}

func teaKeyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// envWithRunner wires a TUI runner stub plus capture buffers and TTY=true.
// Clipboard is stubbed to a no-op so tests do not touch the real system
// clipboard and do not fail in headless environments (Linux CI without
// wl-copy / xclip / xsel).
func envWithRunner(stdin *os.File, run func(Env, TUIOptions) (TUIResult, error)) (Env, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return Env{
		Stdin:      stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI:     run,
		Clipboard:  func(string) error { return nil },
	}, stdout, stderr
}

func TestRunRoot_SaveAndPrintsMachineLine(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		// Add a comment so SaveReview serialises non-trivial state.
		opts.Review.ReviewComment = "looks good"
		return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.HasPrefix(out, "SITATAME_REVIEW=") {
		t.Errorf("stdout missing machine-readable line: %q", out)
	}
	pathLine := strings.TrimSpace(strings.TrimPrefix(out, "SITATAME_REVIEW="))
	if !filepath.IsAbs(pathLine) {
		t.Errorf("printed path must be absolute: %q", pathLine)
	}
	if _, err := os.Stat(pathLine); err != nil {
		t.Errorf("review file missing at printed path: %v", err)
	}
	// Saved file lives under <SITATAME_HOME>/<project-slug>/<branch-slug>/review.md
	// (not under a reviews/ subdirectory).
	if !strings.HasPrefix(pathLine, projectRoot+string(filepath.Separator)) {
		t.Errorf("saved path should live under %s, got %q", projectRoot, pathLine)
	}
	if filepath.Base(pathLine) != "review.md" {
		t.Errorf("saved file must be named review.md; got %q", filepath.Base(pathLine))
	}
	if strings.Contains(pathLine, filepath.Join(dir, ".sitatame")) {
		t.Errorf("saved path leaked into repo tree: %q", pathLine)
	}
}

func TestRunRoot_QuitDiscard_WritesNothing(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	env, _, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		opts.Review.ReviewComment = "discarded"
		return TUIResult{Review: opts.Review, Reason: tui.QuitDiscard}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0 on q (discard)", code)
	}
	// No review file should exist under the project root.
	var found bool
	_ = filepath.Walk(projectRoot, func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && strings.HasSuffix(p, "review.md") {
			found = true
		}
		return nil
	})
	if found {
		t.Errorf("review.md should not be written on QuitDiscard")
	}
}

func TestRunRoot_QuitSave_EmptyReview_WritesNothing(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		// Empty Review: no comments, no review_comment.
		return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
	})
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0 on save of empty review", code)
	}
	// No SITATAME_REVIEW line and no file.
	if strings.Contains(stdout.String(), "SITATAME_REVIEW=") {
		t.Errorf("stdout should not have SITATAME_REVIEW for empty review: %q", stdout.String())
	}
	var found bool
	_ = filepath.Walk(projectRoot, func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && strings.HasSuffix(p, "review.md") {
			found = true
		}
		return nil
	})
	if found {
		t.Errorf("review.md should not be created for empty review")
	}
}

func TestModel_KeySSetsQuitSave(t *testing.T) {
	t.Parallel()
	m := tui.New(nil, review.Review{})
	updated, _ := m.Update(teaKeyRunes("s"))
	mm := updated.(tui.Model)
	if !mm.Quitting() {
		t.Errorf("`s` must set quitting=true")
	}
	if got := mm.QuitReason(); got != tui.QuitSave {
		t.Errorf("QuitReason = %v, want QuitSave", got)
	}
}

func TestModel_KeyQSetsQuitDiscard(t *testing.T) {
	t.Parallel()
	m := tui.New(nil, review.Review{})
	updated, _ := m.Update(teaKeyRunes("q"))
	mm := updated.(tui.Model)
	if got := mm.QuitReason(); got != tui.QuitDiscard {
		t.Errorf("QuitReason = %v, want QuitDiscard", got)
	}
}

// TestRunRoot_Save_CopiesPathToClipboard verifies that on QuitSave with a
// non-empty review, the clipboard function is called with the review path and
// a success message is written to stderr.
func TestRunRoot_Save_CopiesPathToClipboard(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)

	var copiedText string
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			opts.Review.ReviewComment = "lgtm"
			return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
		},
		Clipboard: func(text string) error {
			copiedText = text
			return nil
		},
	}
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// The copied text must match the path printed on stdout.
	pathLine := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "SITATAME_REVIEW="))
	if copiedText == "" {
		t.Fatal("clipboard function was not called")
	}
	if copiedText != pathLine {
		t.Errorf("clipboard text = %q, want %q", copiedText, pathLine)
	}
	if !strings.Contains(stderr.String(), "path copied to clipboard") {
		t.Errorf("stderr missing clipboard confirmation; got %q", stderr.String())
	}
}

// TestRunRoot_Save_NoClipboardFlag_SkipsClipboard verifies that --no-clipboard
// prevents the clipboard function from being called.
func TestRunRoot_Save_NoClipboardFlag_SkipsClipboard(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)

	clipboardCalled := false
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			opts.Review.ReviewComment = "lgtm"
			return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
		},
		Clipboard: func(string) error {
			clipboardCalled = true
			return nil
		},
	}
	if code := RunRoot(env, []string{"--no-clipboard"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if clipboardCalled {
		t.Error("clipboard must not be called when --no-clipboard is set")
	}
	if strings.Contains(stderr.String(), "clipboard") {
		t.Errorf("stderr must not mention clipboard when --no-clipboard is set; got %q", stderr.String())
	}
}

// TestRunRoot_Save_NoClipboardEnv_SkipsClipboard verifies that
// SITATAME_NO_CLIPBOARD=1 suppresses clipboard copy without needing the flag.
func TestRunRoot_Save_NoClipboardEnv_SkipsClipboard(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	t.Setenv("SITATAME_NO_CLIPBOARD", "1")

	clipboardCalled := false
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			opts.Review.ReviewComment = "lgtm"
			return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
		},
		Clipboard: func(string) error {
			clipboardCalled = true
			return nil
		},
	}
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if clipboardCalled {
		t.Error("clipboard must not be called when SITATAME_NO_CLIPBOARD is set")
	}
}

// TestRunRoot_Save_ClipboardError_DoesNotChangeExitCode verifies that a
// clipboard copy failure does not change the exit code (0) — clipboard is
// best-effort and must not fail the overall save operation.
func TestRunRoot_Save_ClipboardError_DoesNotChangeExitCode(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI: func(_ Env, opts TUIOptions) (TUIResult, error) {
			opts.Review.ReviewComment = "lgtm"
			return TUIResult{Review: opts.Review, Reason: tui.QuitSave}, nil
		},
		Clipboard: func(string) error {
			return fmt.Errorf("no clipboard command found")
		},
	}
	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("exit = %d, want 0 even when clipboard copy fails", code)
	}
	if !strings.Contains(stderr.String(), "clipboard copy failed") {
		t.Errorf("stderr should mention clipboard failure; got %q", stderr.String())
	}
	// SITATAME_REVIEW must still be printed even when clipboard fails.
	if !strings.HasPrefix(stdout.String(), "SITATAME_REVIEW=") {
		t.Errorf("stdout missing SITATAME_REVIEW even though save succeeded: %q", stdout.String())
	}
}
