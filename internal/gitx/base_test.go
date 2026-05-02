package gitx

import (
	"path/filepath"
	"strings"
	"testing"
)

// repoWithBranches creates a repo with main + a feature branch with one extra
// commit. Returns the repo dir and the feature branch HEAD sha.
func repoWithBranches(t *testing.T) (dir, mainSHA, featureSHA string) {
	t.Helper()
	dir = initRepo(t)
	mainSHA = strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))

	runGit(t, dir, "checkout", "-q", "-b", "feature")
	if err := writeFileAt(filepath.Join(dir, "f.txt"), []byte("hi\n")); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "feature commit")
	featureSHA = strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	return
}

func TestResolveBase_Explicit(t *testing.T) {
	dir, mainSHA, _ := repoWithBranches(t)
	r, _ := Discover(dir)
	b, err := ResolveBase(r, "main")
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref != "main" || b.SHA != mainSHA {
		t.Errorf("got %+v, want ref=main sha=%s", b, mainSHA)
	}
}

func TestResolveBase_ExplicitMissing(t *testing.T) {
	dir := initRepo(t)
	r, _ := Discover(dir)
	if _, err := ResolveBase(r, "origin/develop"); err == nil {
		t.Fatal("expected error for missing explicit base")
	}
}

func TestResolveBase_FallsBackToMain(t *testing.T) {
	dir, mainSHA, _ := repoWithBranches(t)
	r, _ := Discover(dir)
	b, err := ResolveBase(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref != "main" || b.SHA != mainSHA {
		t.Errorf("got %+v, want main", b)
	}
}

func TestResolveBase_AllFail(t *testing.T) {
	dir := initRepo(t)
	// drop main so there's no candidate that differs from HEAD
	runGit(t, dir, "branch", "-m", "main", "trunk")
	r, _ := Discover(dir)
	if _, err := ResolveBase(r, ""); err == nil {
		t.Fatal("expected error when no candidate resolves")
	}
}

func TestResolveBase_SkipsHEADItself(t *testing.T) {
	dir := initRepo(t) // single-commit repo on main, HEAD == main
	r, _ := Discover(dir)
	if _, err := ResolveBase(r, ""); err == nil {
		t.Fatal("expected error since main == HEAD")
	}
}
