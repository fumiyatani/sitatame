package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RescueError is returned by SaveDraft when Encode fails. It carries the path
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

// Store handles atomic draft writes and promotion to reviews/.
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

// GenerateID builds an id of the form `yyyyMMddTHHmmss-<slug>`. When the same
// (timestamp, slug) pair already has a file in either drafts/ or reviews/, a
// `-1`, `-2`, ... suffix is appended (up to -99). Returns an error if all
// suffixes are taken — extremely unlikely outside pathological tests.
func (s *Store) GenerateID(reviewComment string) (string, error) {
	ts := s.Now().UTC().Format("20060102T150405")
	slug := slugifyReviewComment(reviewComment)
	base := ts + "-" + slug
	if !s.idTaken(base) {
		return base, nil
	}
	for i := 1; i <= 99; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if !s.idTaken(cand) {
			return cand, nil
		}
	}
	return "", fmt.Errorf("could not allocate id under %q after 99 tries", base)
}

func (s *Store) idTaken(id string) bool {
	if _, err := os.Stat(s.Paths.DraftFile(id)); err == nil {
		return true
	}
	if _, err := os.Stat(s.Paths.ReviewFile(id)); err == nil {
		return true
	}
	return false
}

// SaveDraft writes the review to drafts/<slug>/<id>.md atomically (write to a
// tmp file in the same directory, then rename). Returns the final path.
// If r.ID is empty, a new id is generated and assigned to r in-place.
func (s *Store) SaveDraft(r *Review) (string, error) {
	if r.ID == "" {
		id, err := s.GenerateID(r.ReviewComment)
		if err != nil {
			return "", err
		}
		r.ID = id
	}
	// Stamp first-save time so external tools can sort/filter by created_at.
	// Existing drafts (e.g. auto-loaded) keep their original value.
	if r.CreatedAt.IsZero() {
		r.CreatedAt = s.Now().UTC()
	}
	// 0o700 keeps drafts owner-private: reviews can contain unreleased
	// implementation notes the user does not want world- or group-readable
	// on shared machines. os.CreateTemp below produces 0o600 files, so the
	// combined effect is "owner-only" for both the dir and the file.
	if err := os.MkdirAll(s.Paths.DraftsDir(), 0o700); err != nil {
		return "", fmt.Errorf("mkdir drafts: %w", err)
	}
	final := s.Paths.DraftFile(r.ID)
	encode := s.encodeFunc
	if encode == nil {
		encode = Encode
	}
	body, err := encode(*r)
	if err != nil {
		rescuePath, rescueErr := s.writeRescue(r, err)
		if rescueErr != nil {
			// If rescue write also fails, surface both errors.
			return "", fmt.Errorf("encode failed (%w); rescue write also failed: %v", err, rescueErr)
		}
		return "", &RescueError{RescuePath: rescuePath, EncodeErr: err}
	}
	tmp, err := os.CreateTemp(s.Paths.DraftsDir(), "."+r.ID+".*.tmp")
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
	if err := os.Rename(tmpPath, final); err != nil {
		cleanup()
		return "", fmt.Errorf("rename tmp -> draft: %w", err)
	}
	return final, nil
}

// writeRescue writes an in-memory Review as a JSON rescue file under DraftsDir
// when Encode fails. The filename encodes the timestamp so multiple failures in
// the same session produce distinct files. Returns the written path on success.
func (s *Store) writeRescue(r *Review, encodeErr error) (string, error) {
	now := s.Now().UTC()
	ts := now.Format("20060102T150405")
	filename := "review.md.rescue." + ts + ".json"
	rescuePath := filepath.Join(s.Paths.DraftsDir(), filename)

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

// Promote moves a draft file to reviews/<slug>/<id>.md atomically.
// The draft directory is left in place even if empty afterwards.
func (s *Store) Promote(draftPath string) (string, error) {
	id := strings.TrimSuffix(filepath.Base(draftPath), ".md")
	// See SaveDraft: 0o700 keeps promoted reviews owner-private to match the
	// 0o600 perm os.CreateTemp gave the draft file before it was renamed in.
	if err := os.MkdirAll(s.Paths.ReviewsDir(), 0o700); err != nil {
		return "", fmt.Errorf("mkdir reviews: %w", err)
	}
	final := s.Paths.ReviewFile(id)
	if err := os.Rename(draftPath, final); err != nil {
		return "", fmt.Errorf("rename draft -> review: %w", err)
	}
	return final, nil
}

// DetectDraft returns the most recently modified draft path for the current
// branch slug, or "" if no draft exists.
func (s *Store) DetectDraft() (string, error) {
	dir := s.Paths.DraftsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	type cand struct {
		path string
		mod  time.Time
	}
	var cands []cand
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		cands = append(cands, cand{
			path: filepath.Join(dir, e.Name()),
			mod:  info.ModTime(),
		})
	}
	if len(cands) == 0 {
		return "", nil
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod.After(cands[j].mod) })
	return cands[0].path, nil
}

// LatestReview returns the most recently modified review file path for the
// current branch slug, or "" if none exists.
func (s *Store) LatestReview() (string, error) {
	dir := s.Paths.ReviewsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var newest string
	var newestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestMod) {
			newestMod = info.ModTime()
			newest = filepath.Join(dir, e.Name())
		}
	}
	return newest, nil
}

// slugifyReviewComment turns the first line of `s` into a filesystem-safe slug
// up to 32 chars. Falls back to "review" when the input is empty or all-unsafe.
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
