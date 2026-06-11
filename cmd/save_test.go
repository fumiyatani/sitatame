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

func TestRunRoot_SaveAndPromote_PrintsMachineLine(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	env, stdout, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		// Add a comment so SaveDraft serialises non-trivial state.
		opts.Review.ReviewComment = "looks good"
		return TUIResult{Review: opts.Review, Reason: tui.QuitPromote}, nil
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
	// Promotion lands the file under <SITATAME_HOME>/<project-slug>/reviews/,
	// not drafts/, and not anywhere inside the repository tree.
	wantReviewsRoot := filepath.Join(projectRoot, "reviews")
	if !strings.HasPrefix(pathLine, wantReviewsRoot+string(filepath.Separator)) {
		t.Errorf("promoted path should live under %s, got %q", wantReviewsRoot, pathLine)
	}
	if strings.Contains(pathLine, filepath.Join(dir, ".sitatame")) {
		t.Errorf("promoted path leaked into repo tree: %q", pathLine)
	}
}

func TestRunRoot_QuitDraft_KeepsFileUnderDrafts(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	env, _, _ := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		return TUIResult{Review: opts.Review, Reason: tui.QuitDraft}, nil
	})
	if code := RunRoot(env, nil); code != 1 {
		t.Fatalf("exit = %d, want 1 on q", code)
	}
	draftDir := filepath.Join(projectRoot, "drafts")
	found := false
	_ = filepath.Walk(draftDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && strings.HasSuffix(p, ".md") {
			found = true
		}
		return nil
	})
	if !found {
		t.Errorf("expected at least one .md under %s after quit-with-draft", draftDir)
	}
}

func TestRunRoot_PanicSavesDraftAndPropagates(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	_, projectRoot := withSitatameHome(t, dir)

	// Runner panics partway through: shutdown wrapper must still write a draft
	// using whatever state we can recover (the initial Review here).
	env, _, _ := envWithRunner(os.Stdin, func(_ Env, _ TUIOptions) (TUIResult, error) {
		panic("simulated bubbletea crash")
	})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic to propagate from RunRoot")
		}
		// And a draft file must exist under <SITATAME_HOME>/<project>/drafts/<slug>/.
		draftDir := filepath.Join(projectRoot, "drafts")
		found := false
		_ = filepath.Walk(draftDir, func(p string, info os.FileInfo, _ error) error {
			if info != nil && !info.IsDir() && strings.HasSuffix(p, ".md") {
				found = true
			}
			return nil
		})
		if !found {
			t.Errorf("panic-path failed to leave a draft under %s", draftDir)
		}
	}()
	_ = RunRoot(env, nil)
	t.Fatal("RunRoot should have panicked")
}

func TestModel_KeySSetsPromote(t *testing.T) {
	t.Parallel()
	m := tui.New(nil, review.Review{})
	updated, _ := m.Update(teaKeyRunes("s"))
	mm := updated.(tui.Model)
	if !mm.Quitting() {
		t.Errorf("`s` must set quitting=true")
	}
	if got := mm.QuitReason(); got != tui.QuitPromote {
		t.Errorf("QuitReason = %v, want QuitPromote", got)
	}
}

func TestModel_KeyQSetsDraft(t *testing.T) {
	t.Parallel()
	m := tui.New(nil, review.Review{})
	updated, _ := m.Update(teaKeyRunes("q"))
	mm := updated.(tui.Model)
	if got := mm.QuitReason(); got != tui.QuitDraft {
		t.Errorf("QuitReason = %v, want QuitDraft", got)
	}
}
