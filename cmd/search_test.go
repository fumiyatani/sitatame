package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/review"
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

// setupSearchHome points SITATAME_HOME at a fresh temp dir for the test and
// returns the per-project root that matches what RunSearch will resolve for
// the given repo dir. See save_test.withSitatameHome for the symlink-resolution
// rationale.
func setupSearchHome(t *testing.T, repoDir string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("SITATAME_HOME", home)
	resolved, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		resolved = repoDir
	}
	return filepath.Join(home, review.ProjectSlug(resolved))
}

// seedReviews writes synthetic review files into the project's reviews/
// branch-slug subdir so RunSearch (which walks the project reviews root) can
// find them. The branch name "feature" mirrors the legacy fixture; the slug
// shape is computed from BranchSlug to track the real layout.
func seedReviews(t *testing.T, projectRoot string, files map[string]string) {
	t.Helper()
	root := filepath.Join(projectRoot, "reviews", review.BranchSlug("feature"))
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// seedBranchReview writes a review.md for the given branch slug directly
// under <projectRoot>/<branchSlug>/review.md, matching the #76 layout.
func seedBranchReview(t *testing.T, projectRoot, branchSlug, body string) {
	t.Helper()
	dir := filepath.Join(projectRoot, branchSlug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// makeReviewMD renders a minimal review.md with front matter containing the
// given comment, suitable for Decode. Fields are at the 4-space level that
// matches the comment list-item indentation expected by the codec.
func makeReviewMD(branch, commentID, kind, anchorPath string, line int, state, body string) string {
	extra := ""
	if anchorPath != "" {
		extra += "    path: " + anchorPath + "\n"
	}
	if line > 0 {
		extra += "    line: " + itoa(line) + "\n"
	}
	return "---\n" +
		"schema: 1\n" +
		"branch: " + branch + "\n" +
		"comments:\n" +
		"  - anchor_id: " + commentID + "\n" +
		"    kind: " + kind + "\n" +
		extra +
		"    state: " + state + "\n" +
		"    body: |\n" +
		"      " + body + "\n" +
		"---\n"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
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
	projectRoot := setupSearchHome(t, dir)
	seedReviews(t, projectRoot, map[string]string{
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
	projectRoot := setupSearchHome(t, dir)
	seedReviews(t, projectRoot, map[string]string{
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
	_ = setupSearchHome(t, dir)
	env, _, _ := searchEnv()
	// No reviews dir exists yet. Searching shouldn't error.
	if got := RunSearch(env, []string{"anything"}); got != 0 {
		t.Errorf("exit = %d, want 0 when reviews dir is absent", got)
	}
}

func TestRunSearch_InvalidRegex(t *testing.T) {
	dir, _ := newRepo(t)
	chdir(t, dir)
	projectRoot := setupSearchHome(t, dir)
	seedReviews(t, projectRoot, map[string]string{"r1.md": "x\n"})
	env, _, stderr := searchEnv()
	if got := RunSearch(env, []string{"["}); got != 2 {
		t.Errorf("exit = %d, want 2 on invalid regex", got)
	}
	if !strings.Contains(stderr.String(), "invalid pattern") {
		t.Errorf("stderr missing invalid-pattern message: %q", stderr.String())
	}
}

// TestRunSearch_Root_OverridesSITATAME_HOME verifies that --root bypasses the
// env var and points the search at the given directory.
func TestRunSearch_Root_OverridesSITATAME_HOME(t *testing.T) {
	home := t.TempDir()
	// A separate dir that --root will point at.
	root := t.TempDir()
	t.Setenv("SITATAME_HOME", home) // must be ignored

	projectSlug := "myproject"
	branchSlug := "feature--auth"
	seedBranchReview(t, filepath.Join(root, projectSlug), branchSlug,
		makeReviewMD("feature/auth", "c1", "line", "main.go", 10, "open", "NEEDLE found here"),
	)

	env, stdout, _ := searchEnv()
	code := RunSearch(env, []string{"--root", root, "--json", "NEEDLE"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stdout=%q", code, stdout.String())
	}
	var results []SearchResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("parse json: %v; output: %q", err, stdout.String())
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1; %+v", len(results), results)
	}
	if results[0].Project != projectSlug {
		t.Errorf("project = %q, want %q", results[0].Project, projectSlug)
	}
}

// TestRunSearch_JSON_EmitsArray verifies the --json flag produces a valid JSON
// array with the expected fields populated.
func TestRunSearch_JSON_EmitsArray(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SITATAME_HOME", root)

	projectSlug := "proj-abc"
	branchSlug := "feature--login"
	body := makeReviewMD("feature/login", "anchor-1", "line", "auth.go", 42, "open", "TODO: add rate limiting")
	seedBranchReview(t, filepath.Join(root, projectSlug), branchSlug, body)

	env, stdout, _ := searchEnv()
	code := RunSearch(env, []string{"--json", "TODO"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%q", code, stdout.String())
	}
	var results []SearchResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("parse json: %v; output: %q", err, stdout.String())
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1; %+v", len(results), results)
	}
	r := results[0]
	if r.Project != projectSlug {
		t.Errorf("project = %q, want %q", r.Project, projectSlug)
	}
	if r.Branch != branchSlug {
		t.Errorf("branch = %q, want %q", r.Branch, branchSlug)
	}
	if r.State != "open" {
		t.Errorf("state = %q, want open", r.State)
	}
	if r.CommentID != "anchor-1" {
		t.Errorf("comment_id = %q, want anchor-1", r.CommentID)
	}
	if !strings.Contains(r.Match, "TODO") {
		t.Errorf("match %q missing TODO", r.Match)
	}
}

// TestRunSearch_JSON_EmptyResultIsArray ensures the JSON output for no matches
// is "[]" not "null".
func TestRunSearch_JSON_EmptyResultIsArray(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SITATAME_HOME", root)
	projectSlug := "proj-empty"
	branchSlug := "main--abc"
	body := makeReviewMD("main", "c1", "review", "", 0, "resolved", "nothing interesting")
	seedBranchReview(t, filepath.Join(root, projectSlug), branchSlug, body)

	env, stdout, _ := searchEnv()
	code := RunSearch(env, []string{"--json", "NOMATCH"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 for json with no hits", code)
	}
	out := strings.TrimSpace(stdout.String())
	if out != "[]" {
		t.Errorf("expected [], got %q", out)
	}
}

// TestRunSearch_StateFilter_Open verifies --state open excludes resolved comments.
func TestRunSearch_StateFilter_Open(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SITATAME_HOME", root)

	projectSlug := "myproject"
	branchSlug := "feature--search"

	// One open comment, one resolved — both match "KEYWORD".
	reviewMD := "---\n" +
		"schema: 1\nbranch: feature/search\n" +
		"comments:\n" +
		"  - anchor_id: c1\n    kind: review\n    state: open\n    body: |\n      KEYWORD open comment\n" +
		"  - anchor_id: c2\n    kind: review\n    state: resolved\n    body: |\n      KEYWORD resolved comment\n" +
		"---\n"
	seedBranchReview(t, filepath.Join(root, projectSlug), branchSlug, reviewMD)

	env, stdout, _ := searchEnv()
	code := RunSearch(env, []string{"--state", "open", "--json", "KEYWORD"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%q", code, stdout.String())
	}
	var results []SearchResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1 (only open); %+v", len(results), results)
	}
	if results[0].State != "open" {
		t.Errorf("state = %q, want open", results[0].State)
	}
}

// TestRunSearch_ProjectFilter verifies --project limits search to one project.
func TestRunSearch_ProjectFilter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SITATAME_HOME", root)

	slug1 := "project-one"
	slug2 := "project-two"
	branchSlug := "feature--x"
	md1 := makeReviewMD("feature/x", "c1", "review", "", 0, "open", "NEEDLE in project one")
	md2 := makeReviewMD("feature/x", "c2", "review", "", 0, "open", "NEEDLE in project two")
	seedBranchReview(t, filepath.Join(root, slug1), branchSlug, md1)
	seedBranchReview(t, filepath.Join(root, slug2), branchSlug, md2)

	env, stdout, _ := searchEnv()
	code := RunSearch(env, []string{"--project", slug1, "--json", "NEEDLE"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%q", code, stdout.String())
	}
	var results []SearchResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1 (only slug1); %+v", len(results), results)
	}
	if results[0].Project != slug1 {
		t.Errorf("project = %q, want %q", results[0].Project, slug1)
	}
}

// TestRunSearch_BranchFilter verifies --branch limits search to one branch.
func TestRunSearch_BranchFilter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SITATAME_HOME", root)

	projectSlug := "myproject"
	branch1 := "feature--auth"
	branch2 := "feature--search"
	md1 := makeReviewMD("feature/auth", "c1", "review", "", 0, "open", "NEEDLE in branch one")
	md2 := makeReviewMD("feature/search", "c2", "review", "", 0, "open", "NEEDLE in branch two")
	seedBranchReview(t, filepath.Join(root, projectSlug), branch1, md1)
	seedBranchReview(t, filepath.Join(root, projectSlug), branch2, md2)

	env, stdout, _ := searchEnv()
	code := RunSearch(env, []string{"--branch", branch1, "--json", "NEEDLE"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%q", code, stdout.String())
	}
	var results []SearchResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1 (only branch1); %+v", len(results), results)
	}
	if results[0].Branch != branch1 {
		t.Errorf("branch = %q, want %q", results[0].Branch, branch1)
	}
}

// TestRunSearch_StateFilter_InvalidValue verifies --state with an unknown value
// returns exit 2 with an error message.
func TestRunSearch_StateFilter_InvalidValue(t *testing.T) {
	env, _, stderr := searchEnv()
	code := RunSearch(env, []string{"--state", "unknown", "NEEDLE"})
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--state must be one of") {
		t.Errorf("stderr = %q, want --state must be one of message", stderr.String())
	}
}

// TestRunSearch_UnknownFlag verifies an unrecognised flag returns exit 2.
func TestRunSearch_UnknownFlag(t *testing.T) {
	env, _, stderr := searchEnv()
	code := RunSearch(env, []string{"--unknown", "NEEDLE"})
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown flag") {
		t.Errorf("stderr = %q, want unknown flag message", stderr.String())
	}
}

// TestRunSearch_HumanOutput_StateFilter verifies human-readable output when
// --state is set but --json is not.
func TestRunSearch_HumanOutput_StateFilter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SITATAME_HOME", root)

	projectSlug := "proj-human"
	branchSlug := "main--abc"
	reviewMD := makeReviewMD("main", "c1", "line", "main.go", 5, "open", "TODO: humanoutput test")
	seedBranchReview(t, filepath.Join(root, projectSlug), branchSlug, reviewMD)

	env, stdout, _ := searchEnv()
	code := RunSearch(env, []string{"--state", "open", "TODO"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%q", code, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, projectSlug+"/"+branchSlug) {
		t.Errorf("output missing project/branch: %q", out)
	}
	if !strings.Contains(out, "[open]") {
		t.Errorf("output missing [open]: %q", out)
	}
}
