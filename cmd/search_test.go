package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// searchEnv builds an Env wired so the Go fallback path is exercised
// regardless of whether ripgrep is installed on the host.
func searchEnv() (Env, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return Env{
		Stdin:      os.Stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		IsTerminal: func(uintptr) bool { return true },
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
	}, stdout, stderr
}

func seedReviews(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	root := filepath.Join(dir, ".sitatame", "reviews", "feature")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunSearch_RequiresPattern(t *testing.T) {
	env, _, stderr := searchEnv()
	if got := RunSearch(env, nil); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr should show usage: %q", stderr.String())
	}
}

func TestRunSearch_GoFallback_FindsHit(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	seedReviews(t, dir, map[string]string{
		"r1.md": "front matter\nbody mentions a TODO\n",
		"r2.md": "no relevant text here\n",
	})
	env, stdout, _ := searchEnv()
	if got := RunSearch(env, []string{"TODO"}); got != 0 {
		t.Errorf("exit = %d, want 0 on hit", got)
	}
	out := stdout.String()
	if !strings.Contains(out, "r1.md") || !strings.Contains(out, ":2:") {
		t.Errorf("output missing path:line for hit: %q", out)
	}
}

func TestRunSearch_NoHit_ReturnsOne(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	seedReviews(t, dir, map[string]string{
		"r1.md": "boring content only\n",
	})
	env, stdout, _ := searchEnv()
	if got := RunSearch(env, []string{"NEEDLE"}); got != 1 {
		t.Errorf("exit = %d, want 1 on no-hit", got)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty on no-hit, got %q", stdout.String())
	}
}

func TestRunSearch_NoReviewsDir_Succeeds(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	env, _, _ := searchEnv()
	// No .sitatame/reviews/ exists yet. Searching shouldn't error.
	if got := RunSearch(env, []string{"anything"}); got != 0 {
		t.Errorf("exit = %d, want 0 when reviews dir is absent", got)
	}
}

func TestRunSearch_InvalidRegex(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	seedReviews(t, dir, map[string]string{"r1.md": "x\n"})
	env, _, stderr := searchEnv()
	if got := RunSearch(env, []string{"["}); got != 2 {
		t.Errorf("exit = %d, want 2 on invalid regex", got)
	}
	if !strings.Contains(stderr.String(), "invalid pattern") {
		t.Errorf("stderr missing invalid-pattern message: %q", stderr.String())
	}
}
