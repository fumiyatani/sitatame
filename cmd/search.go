package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/fumiyatani/sitatame/internal/gitx"
	"github.com/fumiyatani/sitatame/internal/review"
	"github.com/fumiyatani/sitatame/internal/search"
)

// RunSearch implements `sitatame search <pattern>`. The pattern is treated as
// a Go regexp. Results are printed in `path:line:text` form, one per line, so
// downstream tools can pipe into editors.
//
// When ripgrep is on $PATH we shell out to `rg -n -- <pattern> <projectRoot>`
// for parity with the user's grep workflow; otherwise we fall back to a Go
// implementation that walks the per-project root under SITATAME_HOME.
// Both code paths are deliberately exclusive — tests can disable rg via
// Env.LookPath to exercise the fallback even when rg is locally installed.
//
// Search is branch-independent: we resolve the per-project root (which
// contains <branch-slug>/review.md for every branch reviewed) and walk it
// as a whole. Building Paths with an empty branch is intentional — Paths.Slug
// is unused here. As of issue #76 the layout is 1-branch-1-file so there is
// no separate reviews/ subtree; searching the project root covers all branches.
func RunSearch(env Env, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(env.Stderr, "usage: sitatame search <pattern>")
		return 2
	}
	pattern := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: cwd: %v\n", err)
		return 1
	}
	repo, err := gitx.Discover(cwd)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: %v\n", err)
		return 1
	}
	reviewsRoot := review.NewPaths(repo.Workdir, "").Root()
	if _, err := os.Stat(reviewsRoot); err != nil {
		// Nothing to search yet — don't treat that as an error.
		return 0
	}

	lookPath := env.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if rgPath, err := lookPath("rg"); err == nil && rgPath != "" {
		return runSearchRg(env, rgPath, pattern, reviewsRoot)
	}
	return runSearchGo(env, pattern, reviewsRoot)
}

func runSearchRg(env Env, rgPath, pattern, root string) int {
	cmd := exec.Command(rgPath, "-n", "--no-heading", "--color=never", "--", pattern, root)
	cmd.Stdout = env.Stdout
	cmd.Stderr = env.Stderr
	if err := cmd.Run(); err != nil {
		// rg exits 1 on "no matches"; surface that as exit 1 to the caller too,
		// matching grep semantics.
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(env.Stderr, "sitatame: rg: %v\n", err)
		return 2
	}
	return 0
}

func runSearchGo(env Env, pattern, root string) int {
	re, err := regexp.Compile(pattern)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: invalid pattern: %v\n", err)
		return 2
	}
	hits, err := search.Walk(root, re)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: walk: %v\n", err)
		return 1
	}
	if len(hits) == 0 {
		return 1
	}
	// Sort by path then line for deterministic output.
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Line < hits[j].Line
	})
	w := bufio.NewWriter(env.Stdout)
	defer w.Flush()
	for _, h := range hits {
		fmt.Fprintf(w, "%s:%d:%s\n", h.Path, h.Line, strings.TrimRight(h.Text, "\r"))
	}
	return 0
}
