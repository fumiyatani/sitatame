package gitx

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
)

// stubRunner is a test double for gitRunner that returns pre-baked outputs
// keyed on the first argument (the git subcommand + format flag).
type stubRunner struct {
	// responses maps a key derived from args to (output, error).
	// Key is derived by joining args with spaces.
	responses map[string]stubResponse
	// lastWorkdir records the workdir passed to the most recent run call.
	lastWorkdir string
}

type stubResponse struct {
	out string
	err error
}

func (s *stubRunner) run(workdir string, args ...string) (string, error) {
	s.lastWorkdir = workdir
	key := strings.Join(args, " ")
	if r, ok := s.responses[key]; ok {
		return r.out, r.err
	}
	return "", fmt.Errorf("stubRunner: unexpected call with args %v", args)
}

// stubRepoWithRunner constructs a Repo that uses the given stubRunner.
func stubRepoWithRunner(r gitRunner) *Repo {
	return &Repo{runner: r}
}

// TestDiff_WithStubRunner_ModifiedFile verifies Diff() end-to-end using a stub
// runner, proving git binary is not required for orchestration logic.
func TestDiff_WithStubRunner_ModifiedFile(t *testing.T) {
	t.Parallel()

	rawOut := ":100644 100644 aaa bbb M\x00foo.go\x00"
	numOut := "3\t1\tfoo.go\x00"
	patchOut := `diff --git a/foo.go b/foo.go
index aaa..bbb 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 a
-b
+B
+c
 d
`

	stub := &stubRunner{responses: map[string]stubResponse{
		"diff --raw -z --find-renames --find-copies --cached":       {out: rawOut},
		"diff --numstat -z --find-renames --find-copies --cached":   {out: numOut},
		"diff --patch --no-color --find-renames --find-copies --cached": {out: patchOut},
	}}

	repo := stubRepoWithRunner(stub)
	files, err := repo.Diff(DiffSpec{Source: SourceStaged})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files=%d, want 1", len(files))
	}
	f := files[0]
	if f.Status != diffmodel.StatusModified {
		t.Errorf("status = %q, want M", f.Status)
	}
	if f.PostPath != "foo.go" {
		t.Errorf("path = %q, want foo.go", f.PostPath)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(f.Hunks))
	}
	if len(f.Hunks[0].Lines) != 5 {
		t.Errorf("lines = %d, want 5", len(f.Hunks[0].Lines))
	}
}

// TestDiff_WithStubRunner_BinaryFile verifies that binary detection works
// through the full orchestration path without git.
func TestDiff_WithStubRunner_BinaryFile(t *testing.T) {
	t.Parallel()

	rawOut := ":000000 100644 000 abc A\x00img.png\x00"
	numOut := "-\t-\timg.png\x00"
	patchOut := `diff --git a/img.png b/img.png
index 000..abc 100644
Binary files a/img.png and b/img.png differ
`

	stub := &stubRunner{responses: map[string]stubResponse{
		"diff --raw -z --find-renames --find-copies --cached":           {out: rawOut},
		"diff --numstat -z --find-renames --find-copies --cached":       {out: numOut},
		"diff --patch --no-color --find-renames --find-copies --cached": {out: patchOut},
	}}

	repo := stubRepoWithRunner(stub)
	files, err := repo.Diff(DiffSpec{Source: SourceStaged})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files=%d, want 1", len(files))
	}
	if !files[0].Binary {
		t.Errorf("img.png not flagged binary: %+v", files[0])
	}
	if len(files[0].Hunks) != 0 {
		t.Errorf("binary should have no hunks: %+v", files[0])
	}
}

// TestDiff_WithStubRunner_RunnerError verifies that runner errors are
// propagated correctly.
func TestDiff_WithStubRunner_RunnerError(t *testing.T) {
	t.Parallel()

	stub := &stubRunner{responses: map[string]stubResponse{
		"diff --raw -z --find-renames --find-copies --cached": {err: fmt.Errorf("git not found")},
	}}

	repo := stubRepoWithRunner(stub)
	_, err := repo.Diff(DiffSpec{Source: SourceStaged})
	if err == nil {
		t.Fatal("expected error from runner, got nil")
	}
	if !strings.Contains(err.Error(), "git diff --raw") {
		t.Errorf("error should mention git diff --raw, got: %v", err)
	}
}

// TestRepo_WorkdirMutationReflectedInRunner verifies that mutating Repo.Workdir
// after construction (mimicking the Discover() path) is reflected in the workdir
// passed to the runner on the next call.
//
// Before the fix, newRepo() stored workdir inside execRunner at construction
// time, so a later assignment to r.Workdir had no effect on which directory
// git ran in. The fix moves workdir ownership to Repo.run(), which reads
// r.Workdir on every call.
func TestRepo_WorkdirMutationReflectedInRunner(t *testing.T) {
	t.Parallel()

	rawOut := ":100644 100644 aaa bbb M\x00foo.go\x00"
	numOut := "3\t1\tfoo.go\x00"
	patchOut := "diff --git a/foo.go b/foo.go\nindex aaa..bbb 100644\n--- a/foo.go\n+++ b/foo.go\n@@ -1,2 +1,2 @@\n-a\n+b\n"
	diffResponses := map[string]stubResponse{
		"diff --raw -z --find-renames --find-copies --cached":           {out: rawOut},
		"diff --numstat -z --find-renames --find-copies --cached":       {out: numOut},
		"diff --patch --no-color --find-renames --find-copies --cached": {out: patchOut},
	}

	stub := &stubRunner{responses: diffResponses}

	// Simulate Discover(): construct with initial workdir via Repo struct directly
	// (stub replaces the real runner, so execRunner is not involved).
	repo := &Repo{Workdir: "/initial", runner: stub}

	if _, err := repo.Diff(DiffSpec{Source: SourceStaged}); err != nil {
		t.Fatal(err)
	}
	if stub.lastWorkdir != "/initial" {
		t.Errorf("first call: lastWorkdir = %q, want /initial", stub.lastWorkdir)
	}

	// Mutate Workdir — must be reflected on the next runner call.
	repo.Workdir = "/other"
	stub.responses = diffResponses // reset for second call

	if _, err := repo.Diff(DiffSpec{Source: SourceStaged}); err != nil {
		t.Fatal(err)
	}
	if stub.lastWorkdir != "/other" {
		t.Errorf("after mutation: lastWorkdir = %q, want /other", stub.lastWorkdir)
	}
}
