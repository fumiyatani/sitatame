package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fumiyatani/sitatame/internal/review"
	"github.com/fumiyatani/sitatame/internal/search"
)

// searchOpts is the parsed form of flags for `sitatame search`.
type searchOpts struct {
	Pattern string
	Project string // --project: filter to a specific project slug
	Branch  string // --branch: filter to a specific branch slug
	State   string // --state: open|resolved|stale|all (default "all")
	JSON    bool   // --json: emit JSON array instead of human text
	Root    string // --root: override SITATAME_HOME (skips git-discovery)
}

// parseSearchArgs splits args for `sitatame search` into searchOpts.
// Returns an error message and exit code != 0 on bad input.
func parseSearchArgs(args []string) (searchOpts, string, int) {
	var opts searchOpts
	opts.State = "all"
	remaining := args[:0]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			opts.JSON = true
		case a == "--project" || strings.HasPrefix(a, "--project="):
			v, ok := flagValue(args, &i, a, "--project")
			if !ok {
				return opts, "flag --project requires a value", 2
			}
			opts.Project = v
		case a == "--branch" || strings.HasPrefix(a, "--branch="):
			v, ok := flagValue(args, &i, a, "--branch")
			if !ok {
				return opts, "flag --branch requires a value", 2
			}
			opts.Branch = v
		case a == "--state" || strings.HasPrefix(a, "--state="):
			v, ok := flagValue(args, &i, a, "--state")
			if !ok {
				return opts, "flag --state requires a value", 2
			}
			switch v {
			case "open", "resolved", "stale", "all":
			default:
				return opts, fmt.Sprintf("--state must be one of open|resolved|stale|all, got %q", v), 2
			}
			opts.State = v
		case a == "--root" || strings.HasPrefix(a, "--root="):
			v, ok := flagValue(args, &i, a, "--root")
			if !ok {
				return opts, "flag --root requires a value", 2
			}
			opts.Root = v
		case len(a) > 0 && a[0] == '-':
			return opts, fmt.Sprintf("unknown flag: %s", a), 2
		default:
			remaining = append(remaining, a)
		}
	}
	if len(remaining) < 1 {
		return opts, "", 0 // caller will emit usage
	}
	opts.Pattern = remaining[0]
	return opts, "", 0
}

// flagValue extracts the value for a flag of the form "--name value" or
// "--name=value". It advances i when consuming the next token.
func flagValue(args []string, i *int, cur, name string) (string, bool) {
	prefix := name + "="
	if strings.HasPrefix(cur, prefix) {
		return cur[len(prefix):], true
	}
	// "--name value" form
	*i++
	if *i >= len(args) {
		return "", false
	}
	return args[*i], true
}

// RunSearch implements `sitatame search [flags] <pattern>`.
//
// Pattern is treated as a Go regexp (re2 syntax). By default the command walks
// all project directories under SITATAME_HOME and prints matches in
// "project/branch  path:line  [state]  text" format. With --json it emits a
// JSON array of SearchResult objects suited for machine consumption (e.g. from
// an IntelliJ action).
//
// When ripgrep is available and neither --json nor --state filtering is
// requested, the command shells out to `rg -n` for speed and feature parity
// with the user's grep workflow. The Go fallback is used otherwise.
func RunSearch(env Env, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(env.Stderr, "usage: sitatame search [--project <slug>] [--branch <slug>] [--state open|resolved|stale|all] [--json] [--root <path>] <pattern>")
		return 2
	}

	opts, msg, code := parseSearchArgs(args)
	if code != 0 {
		fmt.Fprintf(env.Stderr, "sitatame: %s\n", msg)
		return code
	}
	if opts.Pattern == "" {
		fmt.Fprintln(env.Stderr, "usage: sitatame search [--project <slug>] [--branch <slug>] [--state open|resolved|stale|all] [--json] [--root <path>] <pattern>")
		return 2
	}

	// Resolve the search root (SITATAME_HOME).
	outputRoot := opts.Root
	if outputRoot == "" {
		outputRoot = os.Getenv(review.EnvOutputRoot)
	}
	if outputRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			outputRoot = filepath.Join(home, ".sitatame")
		}
	}
	if outputRoot == "" {
		fmt.Fprintln(env.Stderr, "sitatame: cannot resolve SITATAME_HOME")
		return 1
	}

	if _, err := os.Stat(outputRoot); err != nil {
		if os.IsNotExist(err) {
			return 0 // nothing to search yet
		}
		fmt.Fprintf(env.Stderr, "sitatame: %v\n", err)
		return 1
	}

	// Structured search (--json or --state filter) requires parsing review.md
	// files and cannot delegate to rg.
	needStructured := opts.JSON || opts.State != "all"

	if needStructured {
		return runSearchStructured(env, opts, outputRoot)
	}

	// Fast path: plain grep over the file tree.
	// When --project is specified, narrow to that single project directory.
	// Otherwise walk all project directories; if none exist yet, return 0
	// (nothing to search — not an error) rather than 1 (no matches).
	if opts.Project != "" {
		searchRoot := filepath.Join(outputRoot, opts.Project)
		if opts.Branch != "" {
			searchRoot = filepath.Join(searchRoot, opts.Branch)
		}
		if _, err := os.Stat(searchRoot); err != nil {
			if os.IsNotExist(err) {
				return 0
			}
			fmt.Fprintf(env.Stderr, "sitatame: %v\n", err)
			return 1
		}
		lookPath := env.LookPath
		if lookPath == nil {
			lookPath = exec.LookPath
		}
		if rgPath, err := lookPath("rg"); err == nil && rgPath != "" {
			return runSearchRg(env, rgPath, opts.Pattern, searchRoot)
		}
		return runSearchGoPlain(env, opts.Pattern, searchRoot)
	}

	// No --project: discover project directories and walk each one.
	// An empty output root (no projects yet) is not an error.
	projectEntries, err := os.ReadDir(outputRoot)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: read root: %v\n", err)
		return 1
	}
	var projectDirs []string
	for _, pe := range projectEntries {
		if !pe.IsDir() || strings.HasPrefix(pe.Name(), ".legacy-") {
			continue
		}
		projectDirs = append(projectDirs, filepath.Join(outputRoot, pe.Name()))
	}
	if len(projectDirs) == 0 {
		return 0 // no projects saved yet
	}

	lookPath := env.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	rgPath, rgErr := lookPath("rg")
	useRg := rgErr == nil && rgPath != ""

	// Walk all project directories and aggregate results.
	anyHit := false
	for _, pd := range projectDirs {
		var code int
		if useRg {
			code = runSearchRg(env, rgPath, opts.Pattern, pd)
		} else {
			code = runSearchGoPlain(env, opts.Pattern, pd)
		}
		if code == 0 {
			anyHit = true
		} else if code > 1 {
			return code // propagate errors immediately
		}
	}
	if anyHit {
		return 0
	}
	return 1
}

// SearchResult is the JSON-serialisable record for a single comment match.
// It mirrors the shape described in issue #127.
type SearchResult struct {
	Project   string        `json:"project"`
	Branch    string        `json:"branch"`
	Anchor    searchAnchor  `json:"anchor"`
	State     string        `json:"state"`
	Body      string        `json:"body"`
	Match     string        `json:"match"`
	CommentID string        `json:"comment_id"`
}

type searchAnchor struct {
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// runSearchStructured walks the output root, parses review.md files, and
// applies --state and regexp filters at the comment level. Used when --json
// or --state is active.
func runSearchStructured(env Env, opts searchOpts, outputRoot string) int {
	re, err := regexp.Compile(opts.Pattern)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: invalid pattern: %v\n", err)
		return 2
	}

	var results []SearchResult

	projectEntries, err := os.ReadDir(outputRoot)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sitatame: read root: %v\n", err)
		return 1
	}

	for _, pe := range projectEntries {
		if !pe.IsDir() {
			continue
		}
		projectSlug := pe.Name()
		if opts.Project != "" && projectSlug != opts.Project {
			continue
		}
		projectDir := filepath.Join(outputRoot, projectSlug)

		branchEntries, err := os.ReadDir(projectDir)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sitatame: read project %s: %v\n", projectSlug, err)
			continue
		}

		for _, be := range branchEntries {
			if !be.IsDir() {
				continue
			}
			branchSlug := be.Name()
			if opts.Branch != "" && branchSlug != opts.Branch {
				continue
			}
			// Skip .legacy-* directories (migrated data).
			if strings.HasPrefix(branchSlug, ".legacy-") {
				continue
			}

			reviewFile := filepath.Join(projectDir, branchSlug, "review.md")
			b, err := os.ReadFile(reviewFile)
			if err != nil {
				if !os.IsNotExist(err) {
					fmt.Fprintf(env.Stderr, "sitatame: read %s: %v\n", reviewFile, err)
				}
				continue
			}

			r, err := review.Decode(b)
			if err != nil {
				fmt.Fprintf(env.Stderr, "sitatame: decode %s: %v (skipping)\n", reviewFile, err)
				continue
			}

			for _, c := range r.Comments {
				stateStr := string(c.State)
				if opts.State != "all" && stateStr != opts.State {
					continue
				}
				if !re.MatchString(c.Body) {
					continue
				}
				results = append(results, SearchResult{
					Project: projectSlug,
					Branch:  branchSlug,
					Anchor: searchAnchor{
						Kind: string(c.Kind),
						Path: c.Path,
						Line: c.Line,
					},
					State:     stateStr,
					Body:      c.Body,
					Match:     firstMatch(re, c.Body),
					CommentID: c.AnchorID,
				})
			}
		}
	}

	if opts.JSON {
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		if results == nil {
			results = []SearchResult{}
		}
		if err := enc.Encode(results); err != nil {
			fmt.Fprintf(env.Stderr, "sitatame: json: %v\n", err)
			return 1
		}
		return 0
	}

	// Human output for --state filter (no --json).
	if len(results) == 0 {
		return 1
	}
	w := bufio.NewWriter(env.Stdout)
	defer w.Flush()
	for _, r := range results {
		loc := r.Anchor.Path
		if r.Anchor.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, r.Anchor.Line)
		}
		summary := strings.TrimRight(r.Body, "\n")
		if idx := strings.IndexByte(summary, '\n'); idx >= 0 {
			summary = summary[:idx]
		}
		fmt.Fprintf(w, "%s/%s\t%s\t[%s]\t%s\n", r.Project, r.Branch, loc, r.State, summary)
	}
	return 0
}

// firstMatch returns the first line of body that matches re, or body itself if
// body is single-line. Used to populate SearchResult.Match.
func firstMatch(re *regexp.Regexp, body string) string {
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if re.MatchString(line) {
			return line
		}
	}
	return body
}

func runSearchRg(env Env, rgPath, pattern, root string) int {
	// --glob '!.legacy-*' excludes legacy migration directories created by
	// MigrateLegacyLayout so migrated data does not surface in search results.
	cmd := exec.Command(rgPath, "-n", "--no-heading", "--color=never", "--glob", "!.legacy-*", "--", pattern, root)
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

func runSearchGoPlain(env Env, pattern, root string) int {
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
