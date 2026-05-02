package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newRepo(t *testing.T) (dir, mainSHA string) {
	t.Helper()
	dir = t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(c.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "init")
	mainSHA = mustGit(t, dir, "rev-parse", "HEAD")

	mustGit(t, dir, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "feature")
	return dir, mainSHA
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(c.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// chdir switches into dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func ttyEnv(stdin *os.File, term bool) Env {
	var stdout, stderr bytes.Buffer
	return Env{
		Stdin:      stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return term },
	}
}

func TestRunRoot_RejectsNonTTY(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return false },
	}
	code := RunRoot(env, nil)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "not a TTY") {
		t.Errorf("stderr missing TTY message: %q", stderr.String())
	}
}

func TestRunRoot_ResolvesAutoBase(t *testing.T) {
	dir, mainSHA := newRepo(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return true },
	}
	_ = RunRoot(env, nil) // exits 2 (TUI unimplemented)
	got := stderr.String()
	if !strings.Contains(got, "base=main") || !strings.Contains(got, "sha="+mainSHA) {
		t.Errorf("stderr missing resolved base/sha: %q", got)
	}
}

func TestRunRoot_ExplicitBaseWins(t *testing.T) {
	dir, _ := newRepo(t)
	// rename main so auto would fail; explicit must still work.
	mustGit(t, dir, "branch", "-m", "main", "trunk")
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdin:      os.Stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		IsTerminal: func(uintptr) bool { return true },
	}
	_ = RunRoot(env, []string{"trunk"})
	if !strings.Contains(stderr.String(), "base=trunk") {
		t.Errorf("stderr missing explicit base: %q", stderr.String())
	}
}

func TestRunRoot_BaseAutoFails(t *testing.T) {
	dir, _ := newRepo(t)
	mustGit(t, dir, "branch", "-m", "main", "trunk")
	chdir(t, dir)
	env := ttyEnv(os.Stdin, true)
	code := RunRoot(env, nil)
	if code != 1 {
		t.Errorf("expected exit 1 when auto base fails, got %d", code)
	}
}

func TestDispatchHelp(t *testing.T) {
	// dispatch lives in main, not cmd; this test just checks RunSearch wiring.
	env := ttyEnv(os.Stdin, true)
	if got := RunSearch(env, []string{"foo"}); got != 2 {
		t.Errorf("RunSearch exit = %d, want 2", got)
	}
}
