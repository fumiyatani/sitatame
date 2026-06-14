package cmd

import (
	"bytes"
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
func envWithRunner(stdin *os.File, run func(Env, TUIOptions) (TUIResult, error)) (Env, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return Env{
		Stdin:      stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		IsTerminal: func(uintptr) bool { return true },
		RunTUI:     run,
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
