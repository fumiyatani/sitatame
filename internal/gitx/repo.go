package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Repo is a thin wrapper that runs git commands within a working tree.
// Pass an empty Workdir to use the process cwd.
type Repo struct {
	Workdir string
}

// Discover resolves the repo root by running `git rev-parse --show-toplevel`
// from the given start dir. Returns the repo with Workdir set to the root.
func Discover(start string) (*Repo, error) {
	r := &Repo{Workdir: start}
	root, err := r.run("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not in a git repository: %w", err)
	}
	return &Repo{Workdir: strings.TrimSpace(root)}, nil
}

// HeadSHA returns the commit SHA at HEAD.
func (r *Repo) HeadSHA() (string, error) {
	out, err := r.run("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// CurrentBranch returns the symbolic name of HEAD (e.g. "feature/x").
// Returns an empty string and a nil error when HEAD is detached.
func (r *Repo) CurrentBranch() (string, error) {
	out, err := r.run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		// detached HEAD: symbolic-ref exits 1 with empty stdout
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// RevParse resolves a ref to a full SHA. Returns an error if the ref doesn't
// exist or git fails.
func (r *Repo) RevParse(ref string) (string, error) {
	// `--end-of-options` (git 2.24+) makes ref parsing immune to flag-shaped
	// refs like "--upload-pack=...".
	out, err := r.run("rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("rev-parse %q: %w", ref, err)
	}
	sha := strings.TrimSpace(out)
	if sha == "" {
		return "", fmt.Errorf("rev-parse %q: empty output", ref)
	}
	return sha, nil
}

// RefExists returns true if the ref resolves to a commit.
func (r *Repo) RefExists(ref string) bool {
	_, err := r.RevParse(ref)
	return err == nil
}

func (r *Repo) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if r.Workdir != "" {
		cmd.Dir = r.Workdir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, msg)
	}
	return stdout.String(), nil
}
