package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RescueError is returned by SaveReview when Encode fails. It carries the path
// of the rescue JSON file that was written so callers can surface it to the
// user with actionable information.
type RescueError struct {
	// RescuePath is the absolute path of the written rescue file.
	RescuePath string
	// EncodeErr is the original Encode failure.
	EncodeErr error
}

func (e *RescueError) Error() string {
	return fmt.Sprintf("encode failed (%v); rescue written to %s", e.EncodeErr, e.RescuePath)
}

func (e *RescueError) Unwrap() error { return e.EncodeErr }

// rescuePayload is the JSON structure written to the rescue file.
type rescuePayload struct {
	Schema              string  `json:"schema"`
	SavedAt             string  `json:"saved_at"`
	Reason              string  `json:"reason"`
	OriginalEncodeError string  `json:"original_encode_error"`
	Review              *Review `json:"review"`
}

// Store handles atomic review writes under the 1-branch-1-file layout
// introduced in issue #76.
type Store struct {
	Paths Paths
	// Now lets tests inject a deterministic clock.
	Now func() time.Time
	// encodeFunc is the encode implementation. Nil means use the package-level
	// Encode function. Tests may replace this to inject failures.
	encodeFunc func(Review) ([]byte, error)
}

func NewStore(p Paths) *Store {
	return &Store{Paths: p, Now: time.Now}
}

// isEmpty reports whether a Review has no content worth persisting.
// An empty Review is one with no comments and a blank top-level review comment.
func isEmpty(r *Review) bool {
	return len(r.Comments) == 0 && strings.TrimSpace(r.ReviewComment) == ""
}

// SaveReview atomically persists r to <BranchDir>/review.md using a
// tmp-write + rename sequence. An existing review.md is backed up to
// review.md.bak before the new version lands.
//
// If r is empty (no comments, blank review_comment), SaveReview is a no-op
// and returns ("", nil). This prevents creating an empty file when the user
// quits without writing anything.
//
// On Encode failure the Rescue mechanism fires: the in-memory Review is
// written as JSON to review.md.rescue.<timestamp>.json and a *RescueError
// is returned so the caller can surface the rescue path to the user.
func (s *Store) SaveReview(r *Review) (string, error) {
	if isEmpty(r) {
		return "", nil
	}

	branchDir := s.Paths.BranchDir()
	// 0o700 keeps reviews owner-private: review notes can contain unreleased
	// implementation notes the user does not want world- or group-readable.
	if err := os.MkdirAll(branchDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir branch dir: %w", err)
	}

	encode := s.encodeFunc
	if encode == nil {
		encode = Encode
	}
	body, err := encode(*r)
	if err != nil {
		rescuePath, rescueErr := s.writeRescue(r, err)
		if rescueErr != nil {
			return "", fmt.Errorf("encode failed (%w); rescue write also failed: %v", err, rescueErr)
		}
		return "", &RescueError{RescuePath: rescuePath, EncodeErr: err}
	}

	// Step 1: write to a tmp file in branchDir (same filesystem as review.md).
	tmp, err := os.CreateTemp(branchDir, ".review.*.tmp")
	if err != nil {
		return "", fmt.Errorf("create tmp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("write tmp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("sync tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("close tmp: %w", err)
	}

	// Step 2: back up the existing review.md as review.md.bak (overwriting any
	// previous .bak — we keep only 1 generation).
	final := s.Paths.ReviewFile()
	bak := s.Paths.BakFile()
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, bak); err != nil {
			cleanup()
			return "", fmt.Errorf("rename review.md -> review.md.bak: %w", err)
		}
	}

	// Step 3: atomic rename tmp -> review.md.
	if err := os.Rename(tmpPath, final); err != nil {
		cleanup()
		return "", fmt.Errorf("rename tmp -> review.md: %w", err)
	}

	// Step 4: fsync the directory so the rename is durable.
	if dir, err := os.Open(branchDir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}

	return final, nil
}

// writeRescue writes an in-memory Review as a JSON rescue file under BranchDir
// when Encode fails. The filename encodes the timestamp so multiple failures in
// the same session produce distinct files. Returns the written path on success.
func (s *Store) writeRescue(r *Review, encodeErr error) (string, error) {
	branchDir := s.Paths.BranchDir()
	// Ensure the dir exists even if SaveReview failed before MkdirAll.
	if err := os.MkdirAll(branchDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir branch dir for rescue: %w", err)
	}

	now := s.Now().UTC()
	ts := now.Format("20060102T150405")
	// Append nanoseconds to prevent filename collision when two Encode failures
	// occur within the same second (e.g. rapid retry or parallel goroutines).
	// The nanos component is zero-padded to 9 digits to keep lexicographic order
	// meaningful. The glob pattern `review.md.rescue.*.json` is still satisfied.
	nanos := fmt.Sprintf("%09d", now.Nanosecond())
	filename := "review.md.rescue." + ts + "-" + nanos + ".json"
	rescuePath := filepath.Join(branchDir, filename)

	payload := rescuePayload{
		Schema:              "rescue/1",
		SavedAt:             now.Format(time.RFC3339),
		Reason:              "yaml encode failed",
		OriginalEncodeError: encodeErr.Error(),
		Review:              r,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("json marshal rescue: %w", err)
	}
	if err := os.WriteFile(rescuePath, b, 0o600); err != nil {
		return "", fmt.Errorf("write rescue file: %w", err)
	}
	return rescuePath, nil
}

// DetectReview returns the path of the review file for the current branch slug
// if it exists, or "" if no review file is present.
func (s *Store) DetectReview() (string, error) {
	p := s.Paths.ReviewFile()
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return p, nil
}

// RecoverFromCrash is called at startup to repair an incomplete write that
// left review.md missing but review.md.bak present (the crash window between
// steps 2 and 3 of SaveReview). It also cleans up any leftover .tmp files.
//
// Safe to call when no crash occurred: if review.md exists or review.md.bak
// doesn't exist, the function is a no-op.
func (s *Store) RecoverFromCrash() error {
	branchDir := s.Paths.BranchDir()

	// Clean up orphaned .tmp files from a previous interrupted write.
	tmpGlob := filepath.Join(branchDir, ".review.*.tmp")
	if tmps, err := filepath.Glob(tmpGlob); err == nil {
		for _, t := range tmps {
			_ = os.Remove(t)
		}
	}

	final := s.Paths.ReviewFile()
	bak := s.Paths.BakFile()

	_, errFinal := os.Stat(final)
	_, errBak := os.Stat(bak)

	if os.IsNotExist(errFinal) && errBak == nil {
		// Crash window: review.md gone but .bak exists — restore it.
		if err := os.Rename(bak, final); err != nil {
			return fmt.Errorf("crash recovery: rename review.md.bak -> review.md: %w", err)
		}
	}
	return nil
}

// slugifyReviewComment turns the first line of `s` into a filesystem-safe slug
// up to 32 chars. Falls back to "review" when the input is empty or all-unsafe.
// Retained for backward compatibility with GenerateID callers.
func slugifyReviewComment(s string) string {
	first := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if first == "" {
		return "review"
	}
	var b strings.Builder
	for _, r := range first {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 32 {
		out = out[:32]
	}
	out = strings.Trim(out, "_")
	// Reject leading '.' so the resulting id can never start with a dot
	// (avoids dotfile-shaped paths and "..", "..." style ids that downstream
	// agents might mishandle).
	out = strings.TrimLeft(out, ".")
	if out == "" {
		return "review"
	}
	return out
}
