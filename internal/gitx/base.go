package gitx

import (
	"errors"
	"fmt"
	"strings"
)

// BaseCandidates is the fallback chain used when the user does not pass an
// explicit base argument. The first candidate that resolves to a commit and
// is not the same as HEAD wins.
var BaseCandidates = []string{
	"origin/HEAD",
	"@{upstream}",
	"origin/main",
	"origin/master",
	"main",
	"master",
}

// Base captures the resolved base ref alongside its commit SHA.
type Base struct {
	Ref string // user-visible ref (e.g. "origin/main")
	SHA string // resolved commit SHA
}

// ResolveBase returns the base ref to diff against. If explicit is non-empty
// it takes precedence and must resolve to a commit. Otherwise the
// BaseCandidates chain is tried in order.
//
// Callers that want to honor a per-repo config file (issue #24) should call
// ResolveBaseWithCandidates instead; ResolveBase is preserved as a thin
// wrapper for existing call sites and tests that have no config plumbed in.
func ResolveBase(repo *Repo, explicit string) (Base, error) {
	return ResolveBaseWithCandidates(repo, explicit, nil)
}

// ResolveBaseWithCandidates is the config-aware variant of ResolveBase.
//
// candidates, when non-nil, fully replaces the BaseCandidates fallback chain
// for the auto-detect path. Passing nil (or an empty slice) preserves the
// existing behavior — i.e. the built-in BaseCandidates are tried. The
// explicit argument still wins over any config-supplied candidates, matching
// the documented priority order:
//
//  1. CLI explicit base
//  2. config.base.default (passed as the first entry of candidates)
//  3. config.base.candidates
//  4. built-in BaseCandidates (only appended by the caller; this function
//     trusts whatever list it is handed)
//
// Keeping the priority layering in the caller (cmd/root.go) rather than this
// function avoids re-deriving the merged list every place we want to resolve
// a base, and keeps this package free of any knowledge about the config
// package's shape.
func ResolveBaseWithCandidates(repo *Repo, explicit string, candidates []string) (Base, error) {
	if explicit != "" {
		sha, err := repo.RevParse(explicit)
		if err != nil {
			return Base{}, fmt.Errorf("base %q not found: %w", explicit, err)
		}
		return Base{Ref: explicit, SHA: sha}, nil
	}

	chain := candidates
	if len(chain) == 0 {
		chain = BaseCandidates
	}

	headSHA, _ := repo.HeadSHA()
	var tried []string
	for _, c := range chain {
		ref := normalizeCandidate(repo, c)
		if ref == "" {
			continue
		}
		sha, err := repo.RevParse(ref)
		if err != nil {
			tried = append(tried, ref)
			continue
		}
		if sha == headSHA {
			tried = append(tried, ref+" (==HEAD)")
			continue
		}
		return Base{Ref: ref, SHA: sha}, nil
	}
	return Base{}, errors.New("base not found; tried " + strings.Join(tried, ", ") +
		"; pass an explicit base via `sitatame <base>`")
}

// normalizeCandidate expands `origin/HEAD` to its concrete branch ref so the
// SHA we hand back matches the upstream default branch (e.g. `origin/main`).
// Returns empty string when the candidate doesn't apply (e.g. no upstream).
func normalizeCandidate(repo *Repo, c string) string {
	switch c {
	case "origin/HEAD":
		out, err := repo.run("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out)
	}
	return c
}
