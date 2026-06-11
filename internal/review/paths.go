package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// RepoRoot is the canonical absolute path of the repository working
	// directory (after EvalSymlinks + Abs). It is retained for ProjectSlug
	// derivation and to keep the public NewPaths signature stable. Canonicalising
	// here means that referring to the same checkout through e.g. /tmp/x
	// versus /private/tmp/x on macOS, or through a symlinked ~/work, all map to
	// the same ProjectSlug and therefore the same on-disk drafts/reviews
	// directory.
	RepoRoot string
	// ProjectSlug identifies the repository under OutputRoot. Built from
	// basename(RepoRoot) + sha1(RepoRoot)[:8] so distinct checkouts of the
	// same repo (e.g. worktrees) get distinct directories.
	ProjectSlug string
	// Branch is the original branch name; Slug is its BranchSlug. Branch may
	// be empty for branch-independent callers like `sitatame search`, which
	// only touch ReviewsRoot()/DraftsRoot(); the Slug then falls back to
	// BranchSlug("") = "branch__da39a3ee" so detached-HEAD writes still stay
	// branch-scoped instead of landing directly under reviews/ or drafts/.
	// Detached HEAD is normalised by cmd/root.go into "detached/<sha[:12]>"
	// so each detached session gets its own slug rather than colliding on the
	// empty-branch fallback.
	Branch string
	Slug   string
}

// NewPaths builds Paths using the default OutputRoot resolution order:
//  1. $SITATAME_HOME if set and non-empty
//  2. <user home>/.sitatame
//  3. <os.TempDir>/sitatame (with a one-line stderr warning)
//
// Branch may be empty when the caller is not branch-scoped (e.g. search) or
// when the repo is in a detached HEAD. The branch-scoped helpers then resolve
// to BranchSlug("") = "branch__da39a3ee" so detached-HEAD writes still stay
// under a stable branch-scoped directory instead of polluting reviews/ or
// drafts/ directly. Distinguishing individual detached HEADs (e.g. by HEAD
// SHA) is intentionally out of scope here; that would be a separate issue.
func NewPaths(repoRoot, branch string) Paths {
	return NewPathsWithRoot(resolveOutputRoot(), repoRoot, branch)
}

// NewPathsWithRoot is the test-friendly constructor: it takes an explicit
// OutputRoot instead of going through environment + user-home resolution.
// Production code should prefer NewPaths.
//
// repoRoot is canonicalised via EvalSymlinks + Abs before being hashed into a
// ProjectSlug. Without this, two paths that name the same checkout (e.g.
// /tmp/x and /private/tmp/x on macOS, or ~/work and its symlink target) would
// hash to different slugs and therefore drop drafts into two unrelated
// directories. Canonicalisation failures (broken symlink, non-existent path)
// are non-fatal: we fall back to the input untouched so callers are not
// blocked by best-effort hardening.
//
// BranchSlug is always invoked, even when branch == "". This is deliberate:
// BranchSlug("") returns the deterministic "branch__da39a3ee", which keeps
// detached-HEAD sessions inside a branch-scoped directory rather than letting
// SaveDraft / DetectDraft / Promote collapse onto ReviewsRoot()/DraftsRoot()
// directly and share state across unrelated sessions.
func NewPathsWithRoot(outputRoot, repoRoot, branch string) Paths {
	canonical := canonicaliseRepoRoot(repoRoot)
	return Paths{
		OutputRoot:  outputRoot,
		RepoRoot:    canonical,
		ProjectSlug: ProjectSlug(canonical),
		Branch:      branch,
		Slug:        BranchSlug(branch),
	}
}

// canonicaliseRepoRoot resolves symlinks and absolutises the input so the
// project slug stays stable across path aliases. Errors are swallowed because
// this is best-effort hardening: tests use synthetic paths like "/repo" that
// don't exist on disk, and any callable production input that we cannot
// canonicalise (broken symlink, permission error) should still get a usable
// slug rather than aborting.
func canonicaliseRepoRoot(repoRoot string) string {
	if repoRoot == "" {
		return repoRoot
	}
	canonical := repoRoot
	if r, err := filepath.EvalSymlinks(repoRoot); err == nil && r != "" {
		canonical = r
	}
	if abs, err := filepath.Abs(canonical); err == nil && abs != "" {
		canonical = abs
	}
	return canonical
}

// resolveOutputRoot is split out so NewPaths stays trivially readable. The
// stderr warning on the TempDir fallback is intentionally one line and only
// fires when both env and home lookup fail — typical desktop sessions never
// hit it.
//
// SITATAME_HOME goes through a small validation pipeline before being adopted:
//   - leading/trailing whitespace is trimmed; an all-whitespace value is
//     treated as unset (otherwise a stray `export SITATAME_HOME=" "` would
//     silently land everything under "  /<project-slug>/...").
//   - a leading "~/" or bare "~" is expanded via UserHomeDir so users can
//     point the env var at "~/work" without shell expansion gotchas.
//   - relative paths are absolutised via filepath.Abs, with a one-line stderr
//     warning so callers know which directory was actually picked.
func resolveOutputRoot() string {
	if v := strings.TrimSpace(os.Getenv(EnvOutputRoot)); v != "" {
		return normaliseEnvOutputRoot(v)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, rootDir)
	}
	fallback := filepath.Join(os.TempDir(), "sitatame")
	fmt.Fprintf(os.Stderr, "sitatame: could not resolve user home; falling back to %s\n", fallback)
	return fallback
}

// normaliseEnvOutputRoot expands "~/" and absolutises relative paths supplied
// via $SITATAME_HOME. It is split out so the env-set branch in
// resolveOutputRoot stays a single expression, and so tests can exercise the
// normalisation independently if needed.
func normaliseEnvOutputRoot(v string) string {
	if v == "~" || strings.HasPrefix(v, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			if v == "~" {
				v = home
			} else {
				v = filepath.Join(home, v[2:])
			}
		}
	}
	if filepath.IsAbs(v) {
		return v
	}
	abs, err := filepath.Abs(v)
	if err != nil || abs == "" {
		// Abs only fails when Getwd fails, in which case there is no better
		// answer than the literal value the user supplied. The earlier warning
		// path already covers the home-resolution failure case.
		return v
	}
	fmt.Fprintf(os.Stderr, "sitatame: %s was relative; using %s\n", EnvOutputRoot, abs)
	return abs
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
