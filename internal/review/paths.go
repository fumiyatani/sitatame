package review

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	rootDir    = ".sitatame"
	reviewsDir = "reviews"
	draftsDir  = "drafts"

	// EnvOutputRoot lets callers override the on-disk output root (which
	// normally resolves to ~/.sitatame). Tests and power users set this to
	// isolate review storage from the user-global location.
	EnvOutputRoot = "SITATAME_HOME"
)

// Paths resolves on-disk locations for review storage. As of issue #38, output
// lives under <OutputRoot>/<ProjectSlug>/{reviews,drafts}/<BranchSlug>/...
// rather than <repo>/.sitatame/... — this keeps generated artefacts out of the
// repository tree so users do not have to .gitignore them per-project.
type Paths struct {
	// OutputRoot is the resolved <output-root> (e.g. "/Users/me/.sitatame").
	OutputRoot string
	// RepoRoot is the absolute path of the repository working directory; it is
	// retained for ProjectSlug derivation and to keep the public NewPaths
	// signature stable.
	RepoRoot string
	// ProjectSlug identifies the repository under OutputRoot. Built from
	// basename(RepoRoot) + sha1(RepoRoot)[:8] so distinct checkouts of the
	// same repo (e.g. worktrees) get distinct directories.
	ProjectSlug string
	// Branch is the original branch name; Slug is its BranchSlug. When Branch
	// is empty (e.g. for `sitatame search`, which is branch-independent),
	// Slug is also empty and the per-branch helpers should not be used.
	Branch string
	Slug   string
}

// NewPaths builds Paths using the default OutputRoot resolution order:
//  1. $SITATAME_HOME if set and non-empty
//  2. <user home>/.sitatame
//  3. <os.TempDir>/sitatame (with a one-line stderr warning)
//
// Branch may be empty when the caller is not branch-scoped (e.g. search). In
// that case branch-scoped helpers return paths with an empty slug component
// and callers should restrict themselves to the project-wide roots.
func NewPaths(repoRoot, branch string) Paths {
	return NewPathsWithRoot(resolveOutputRoot(), repoRoot, branch)
}

// NewPathsWithRoot is the test-friendly constructor: it takes an explicit
// OutputRoot instead of going through environment + user-home resolution.
// Production code should prefer NewPaths.
func NewPathsWithRoot(outputRoot, repoRoot, branch string) Paths {
	p := Paths{
		OutputRoot:  outputRoot,
		RepoRoot:    repoRoot,
		ProjectSlug: ProjectSlug(repoRoot),
		Branch:      branch,
	}
	if branch != "" {
		p.Slug = BranchSlug(branch)
	}
	return p
}

// resolveOutputRoot is split out so NewPaths stays trivially readable. The
// stderr warning on the TempDir fallback is intentionally one line and only
// fires when both env and home lookup fail — typical desktop sessions never
// hit it.
func resolveOutputRoot() string {
	if v := os.Getenv(EnvOutputRoot); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, rootDir)
	}
	fallback := filepath.Join(os.TempDir(), "sitatame")
	fmt.Fprintf(os.Stderr, "sitatame: could not resolve user home; falling back to %s\n", fallback)
	return fallback
}

// Root is the per-project root: <OutputRoot>/<ProjectSlug>. Reviews and drafts
// for every branch of this checkout live underneath it.
func (p Paths) Root() string        { return filepath.Join(p.OutputRoot, p.ProjectSlug) }
func (p Paths) ReviewsRoot() string { return filepath.Join(p.Root(), reviewsDir) }
func (p Paths) DraftsRoot() string  { return filepath.Join(p.Root(), draftsDir) }
func (p Paths) ReviewsDir() string  { return filepath.Join(p.ReviewsRoot(), p.Slug) }
func (p Paths) DraftsDir() string   { return filepath.Join(p.DraftsRoot(), p.Slug) }
func (p Paths) ReviewFile(id string) string {
	return filepath.Join(p.ReviewsDir(), id+".md")
}
func (p Paths) DraftFile(id string) string {
	return filepath.Join(p.DraftsDir(), id+".md")
}

// LegacyRoot returns the pre-#38 in-repo storage path (<repo>/.sitatame). It
// exists so the CLI can warn users that the directory is no longer the source
// of truth; nothing is read from or written to it.
func (p Paths) LegacyRoot() string {
	if p.RepoRoot == "" {
		return ""
	}
	return filepath.Join(p.RepoRoot, rootDir)
}
