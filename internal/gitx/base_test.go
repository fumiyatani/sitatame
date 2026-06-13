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

// TestResolveBaseWithCandidates_OverridesChain documents the config-aware
// path: when the caller supplies a non-empty candidate list, it fully
// replaces the built-in BaseCandidates. This is what cmd/root.go relies on
// to honor `base.candidates` from <repo>/.sitatame/config.yaml.
func TestResolveBaseWithCandidates_OverridesChain(t *testing.T) {
	dir, mainSHA, _ := repoWithBranches(t)
	r, _ := Discover(dir)
	// Only `main` resolves — drop everything else so the test fails loudly
	// if the override is ignored and the built-in chain is used.
	b, err := ResolveBaseWithCandidates(r, "", []string{"missing-ref", "main"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref != "main" || b.SHA != mainSHA {
		t.Errorf("got %+v, want ref=main sha=%s", b, mainSHA)
	}
}

// TestResolveBaseWithCandidates_NilFallsBackToDefault confirms that passing
// nil keeps the existing auto-detect behavior intact — important so callers
// without a config file see no regression.
func TestResolveBaseWithCandidates_NilFallsBackToDefault(t *testing.T) {
	dir, mainSHA, _ := repoWithBranches(t)
	r, _ := Discover(dir)
	b, err := ResolveBaseWithCandidates(r, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref != "main" || b.SHA != mainSHA {
		t.Errorf("got %+v, want main", b)
	}
}

// TestResolveBaseWithCandidates_ExplicitWinsOverList confirms the CLI
// argument continues to take precedence over the supplied candidate list.
func TestResolveBaseWithCandidates_ExplicitWinsOverList(t *testing.T) {
	dir, mainSHA, _ := repoWithBranches(t)
	r, _ := Discover(dir)
	b, err := ResolveBaseWithCandidates(r, "main", []string{"origin/develop"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref != "main" || b.SHA != mainSHA {
		t.Errorf("got %+v, want main", b)
	}
}
