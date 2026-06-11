package replay_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fumiyatani/sitatame/internal/replay"
)

// buildBinary compiles the sitatame CLI once per test process and returns the
// resulting binary path. We deliberately rebuild rather than relying on a
// pre-built artefact so the smoke tests always exercise the current source
// tree without the caller having to remember to `go build` first.
func buildBinary(t *testing.T) string {
	t.Helper()
	binPathOnce.Do(func() {
		dir, err := os.MkdirTemp("", "sitatame-bin-*")
		if err != nil {
			binPathErr = err
			return
		}
		out := filepath.Join(dir, "sitatame")
		// Walk up to the repository root: this test file lives at
		// <repo>/internal/replay/, so two parents up gives the module root
		// containing main.go. Anchoring on go.mod would also work but `..` keeps
		// the helper short and is safe inside a fixed-layout repo.
		repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			binPathErr = err
			return
		}
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = repoRoot
		// Inherit the test process's environment so GOCACHE / GOMODCACHE
		// overrides set by the harness propagate to the child build.
		cmd.Env = os.Environ()
		if combined, err := cmd.CombinedOutput(); err != nil {
			binPathErr = err
			binPathBuild = string(combined)
			return
		}
		binPath = out
	})
	if binPathErr != nil {
		t.Fatalf("replay: build sitatame binary: %v\n%s", binPathErr, binPathBuild)
	}
	return binPath
}

var (
	binPathOnce  sync.Once
	binPath      string
	binPathErr   error
	binPathBuild string
)

// setupRepo creates a temporary git repository with one committed file and
// one staged change, plus an isolated SITATAME_HOME so the smoke tests never
// pollute the developer's real ~/.sitatame. The returned dir is suitable as
// repoDir for replay.Start; the returned home is the SITATAME_HOME root used
// by sitatame to resolve draft/promoted review locations.
func setupRepo(t *testing.T) (dir, home string) {
	t.Helper()
	dir = t.TempDir()
	home = t.TempDir()
	t.Setenv("SITATAME_HOME", home)

	gitInit := exec.Command("git", "init", "-q", "-b", "main")
	gitInit.Dir = dir
	gitInit.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("first\nsecond\nthird\n"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "init")

	// Add a working-tree change so `sitatame --working` has something to render.
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("first\nsecond changed\nthird\nfourth added\n"), 0o644); err != nil {
		t.Fatalf("write a (modified): %v", err)
	}

	return dir, home
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// waitForBoot waits until the TUI has drawn at least one row containing the
// header "sitatame". The startup path is async (bubbletea spins up an alt
// screen + goroutines), so polling avoids a fragile fixed sleep.
func waitForBoot(t *testing.T, s *replay.Session) {
	t.Helper()
	if err := s.WaitFor("sitatame", 5*time.Second); err != nil {
		t.Fatalf("waiting for TUI boot: %v\nscreen=\n%s", err, s.Screen())
	}
}

// TestBootAndQuitCreatesDraft launches sitatame, sends `q`, expects exit code
// 1, and confirms at least one draft .md was written under SITATAME_HOME.
func TestBootAndQuitCreatesDraft(t *testing.T) {
	replay.SkipIfNoPTY(t)
	bin := buildBinary(t)
	dir, home := setupRepo(t)

	s := replay.Start(t, bin, dir, "--working")
	waitForBoot(t, s)

	s.Send("q")

	code, err := s.WaitForExit(5 * time.Second)
	if err != nil {
		t.Fatalf("waiting for exit: %v\nscreen=\n%s", err, s.Screen())
	}
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (QuitDraft)\nscreen=\n%s", code, s.Screen())
	}

	// drafts/ is below <home>/<project-slug>/drafts/<branch-slug>/. Rather than
	// reverse-engineer the slug here, walk the tree for any .md.
	found := findFirstMarkdown(t, filepath.Join(home), "drafts")
	if found == "" {
		t.Fatalf("no draft .md file found under %s", home)
	}
}

// TestBootSavePromotePrintsEnv launches sitatame, sends `s`, expects exit
// code 0, and confirms the stdout contains `SITATAME_REVIEW=` followed by an
// absolute path to a .md file under the reviews/ tree.
func TestBootSavePromotePrintsEnv(t *testing.T) {
	replay.SkipIfNoPTY(t)
	bin := buildBinary(t)
	dir, _ := setupRepo(t)

	s := replay.Start(t, bin, dir, "--working")
	waitForBoot(t, s)

	s.Send("s")

	code, err := s.WaitForExit(5 * time.Second)
	if err != nil {
		t.Fatalf("waiting for exit: %v\nscreen=\n%s", err, s.Screen())
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (QuitPromote)\nscreen=\n%s\nstdout=\n%s", code, s.Screen(), s.Stdout())
	}

	out := s.Stdout()
	idx := strings.Index(out, "SITATAME_REVIEW=")
	if idx < 0 {
		t.Fatalf("SITATAME_REVIEW= not in stdout:\n%s", out)
	}
	rest := out[idx+len("SITATAME_REVIEW="):]
	end := strings.IndexAny(rest, "\r\n")
	if end < 0 {
		end = len(rest)
	}
	path := strings.TrimSpace(rest[:end])
	if !strings.HasSuffix(path, ".md") {
		t.Fatalf("SITATAME_REVIEW path is not an .md file: %q", path)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("SITATAME_REVIEW path is not absolute: %q", path)
	}
}

// TestHelpToggleVisible verifies `?` opens the help modal (rendering known
// help-line text) and a second `?` hides it again.
func TestHelpToggleVisible(t *testing.T) {
	replay.SkipIfNoPTY(t)
	bin := buildBinary(t)
	dir, _ := setupRepo(t)

	s := replay.Start(t, bin, dir, "--working")
	waitForBoot(t, s)

	s.Send("?")
	// "j / k" is a literal substring in helpLines (internal/tui/help.go).
	if err := s.WaitFor("j / k", 3*time.Second); err != nil {
		t.Fatalf("help did not appear: %v\nscreen=\n%s", err, s.Screen())
	}

	s.Send("?")
	// Wait for the help modal to be torn down. Poll for absence rather than
	// asserting a single snapshot — the bubbletea re-render is async.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.Contains(s.Screen(), "j / k") {
			s.Send("q")
			_, _ = s.WaitForExit(3 * time.Second)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("help did not close; screen still has 'j / k':\n%s", s.Screen())
}

// TestResizeDuringHelp opens the help modal, resizes the pty, and asserts the
// "sitatame" header still appears in the top region. We avoid asserting on
// exact column / row counts because vt10x and bubbletea both reflow on resize
// and the precise layout depends on lipgloss heuristics.
func TestResizeDuringHelp(t *testing.T) {
	replay.SkipIfNoPTY(t)
	bin := buildBinary(t)
	dir, _ := setupRepo(t)

	s := replay.Start(t, bin, dir, "--working")
	waitForBoot(t, s)

	s.Send("?")
	if err := s.WaitFor("j / k", 3*time.Second); err != nil {
		t.Fatalf("help did not appear: %v\nscreen=\n%s", err, s.Screen())
	}

	if err := s.Resize(140, 50); err != nil {
		t.Fatalf("resize: %v", err)
	}

	// Give bubbletea a moment to re-render on SIGWINCH. We then assert that
	// the header is still on screen somewhere — that's a stable signal the
	// app survived the resize rather than getting stuck.
	if err := s.WaitFor("sitatame", 3*time.Second); err != nil {
		t.Fatalf("header missing after resize: %v\nscreen=\n%s", err, s.Screen())
	}

	s.Send("q")
	_, _ = s.WaitForExit(3 * time.Second)
}

// TestWheelEmitsScroll sends an xterm SGR-mode mouse wheel down event and
// asserts the screen changes. The diff fixture has more rows than fit on
// screen, so a wheel scroll must visibly shift the viewport.
func TestWheelEmitsScroll(t *testing.T) {
	replay.SkipIfNoPTY(t)
	bin := buildBinary(t)
	dir, _ := setupRepo(t)

	// Make the diff long enough that scrolling is observable: replace the
	// single working-tree change with one that touches many lines.
	long := strings.Builder{}
	for i := 0; i < 200; i++ {
		long.WriteString("changed line ")
		long.WriteString(strings.Repeat("x", 4))
		long.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte(long.String()), 0o644); err != nil {
		t.Fatalf("write long a: %v", err)
	}

	s := replay.Start(t, bin, dir, "--working")
	waitForBoot(t, s)

	before := s.Screen()
	// xterm SGR mode wheel-down: ESC [ < 65 ; 1 ; 1 M. Bubble Tea, configured
	// with tea.WithMouseCellMotion, understands this without enabling SGR via
	// DECSET first because the program writes that DECSET on startup.
	for i := 0; i < 30; i++ {
		s.Send("\x1b[<65;1;1M")
		time.Sleep(15 * time.Millisecond)
	}

	deadline := time.Now().Add(2 * time.Second)
	changed := false
	for time.Now().Before(deadline) {
		if s.Screen() != before {
			changed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !changed {
		t.Fatalf("screen did not change after wheel scroll\nbefore=\n%s", before)
	}

	s.Send("q")
	_, _ = s.WaitForExit(3 * time.Second)
}

// findFirstMarkdown walks root looking for the first .md file whose ancestor
// path includes mustContain. Returns "" if none found.
func findFirstMarkdown(t *testing.T, root, mustContain string) string {
	t.Helper()
	var hit string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		if mustContain != "" && !strings.Contains(path, mustContain) {
			return nil
		}
		if hit == "" {
			hit = path
		}
		return nil
	})
	return hit
}
