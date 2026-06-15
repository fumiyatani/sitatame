package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// gitRunner abstracts the execution of git subcommands. Implementations must
// return the combined stdout on success, or a wrapped error on failure.
// workdir is the working directory for the git command; pass an empty string to
// use the process cwd. args must NOT include the "git" binary name; it is
// prepended by the runner.
type gitRunner interface {
	run(workdir string, args ...string) (string, error)
}

// execRunner is the production gitRunner that shells out to the real git binary.
type execRunner struct{}

func (r *execRunner) run(workdir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if workdir != "" {
		cmd.Dir = workdir
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
