package gitx

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit runs git in dir with extra args; failures call t.Fatalf with
// stdout/stderr.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// initRepo initializes an empty git repo with a single commit on `main`.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	if err := writeFile(filepath.Join(dir, "README.md"), "init\n"); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func writeFile(p, body string) error {
	return writeFileAt(p, []byte(body))
}

func TestDiscover_FindsRoot(t *testing.T) {
	dir := initRepo(t)
	sub := filepath.Join(dir, "sub")
	if err := mkdirAll(sub); err != nil {
		t.Fatal(err)
	}
	r, err := Discover(sub)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := filepath.EvalSymlinks(r.Workdir)
	want, _ := filepath.EvalSymlinks(dir)
	if got != want {
		t.Errorf("Workdir = %q, want %q", got, want)
	}
}

func TestDiscover_NotARepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := Discover(dir); err == nil {
		t.Fatal("expected error outside a git repo")
	}
}

func TestRepoHeadSHA(t *testing.T) {
	dir := initRepo(t)
	r, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	sha, err := r.HeadSHA()
	if err != nil {
		t.Fatal(err)
	}
	if len(sha) != 40 {
		t.Errorf("HeadSHA len = %d, want 40 (%q)", len(sha), sha)
	}
}

func TestRepoRevParse(t *testing.T) {
	dir := initRepo(t)
	r, _ := Discover(dir)
	if !r.RefExists("HEAD") {
		t.Error("HEAD should exist")
	}
	if r.RefExists("does-not-exist") {
		t.Error("nonexistent ref should not exist")
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := initRepo(t)
	r, _ := Discover(dir)
	got, err := r.CurrentBranch()
	if err != nil {
		t.Fatal(err)
	}
	if got != "main" {
		t.Errorf("CurrentBranch = %q, want main", got)
	}
}
