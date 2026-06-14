package review

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MigrateLegacyLayout detects legacy drafts/reviews trees under the output root
// and moves them into <output-root>/<project-slug>/.legacy-<YYYYMMDDTHHMMSS>/
// while copying the latest review per branch-slug into the new BranchDir layout.
// Returns the number of branch-slugs migrated and the legacy directory path
// (empty if no migration was needed).
//
// Non-destructive: legacy data is preserved under .legacy-<ts>/ until the user
// removes it manually. This is intentional so the user can recover anything
// that was missed by the auto-migration.
//
// Crash safety: each rename is logged but not transactional. If the migration
// is interrupted mid-flight, the user can re-run; partial state is detected by
// the absence of drafts/reviews directories at the legacy paths.
//
// Cross-device mv: os.Rename across filesystems will fail. This is assumed to
// be an unusual configuration; users who hit this should migrate manually.
func (s *Store) MigrateLegacyLayout() (migrated int, legacyDir string, err error) {
	draftsRoot := s.Paths.LegacyDraftsRoot()
	reviewsRoot := s.Paths.LegacyReviewsRoot()

	hasDrafts := dirExists(draftsRoot)
	hasReviews := dirExists(reviewsRoot)
	if !hasDrafts && !hasReviews {
		// No legacy layout found; new user or already migrated.
		return 0, "", nil
	}

	// Create .legacy-<ts>/ under the project root.
	now := s.Now().UTC()
	ts := now.Format("20060102T150405")
	projectRoot := s.Paths.Root()
	legacyDir = filepath.Join(projectRoot, ".legacy-"+ts)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		return 0, "", fmt.Errorf("migration: mkdir legacy dir: %w", err)
	}

	// Move drafts/ into .legacy-<ts>/drafts/ if present.
	if hasDrafts {
		dest := filepath.Join(legacyDir, "drafts")
		if rerr := os.Rename(draftsRoot, dest); rerr != nil {
			return 0, "", fmt.Errorf("migration: mv drafts to legacy: %w", rerr)
		}
	}

	// Move reviews/ into .legacy-<ts>/reviews/ if present.
	if hasReviews {
		dest := filepath.Join(legacyDir, "reviews")
		if rerr := os.Rename(reviewsRoot, dest); rerr != nil {
			return 0, "", fmt.Errorf("migration: mv reviews to legacy: %w", rerr)
		}
	}

	// Copy the latest review per branch-slug from legacy reviews into the new layout.
	legacyReviewsDir := filepath.Join(legacyDir, "reviews")
	if dirExists(legacyReviewsDir) {
		entries, err := os.ReadDir(legacyReviewsDir)
		if err != nil {
			// Non-fatal: the mv succeeded but we cannot enumerate branch slugs.
			return 0, legacyDir, fmt.Errorf("migration: read legacy reviews dir: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			branchSlug := e.Name()
			branchLegacyDir := filepath.Join(legacyReviewsDir, branchSlug)
			latest, lerr := latestReviewFile(branchLegacyDir)
			if lerr != nil || latest == "" {
				// Empty dir or unreadable; skip this branch.
				continue
			}

			// Destination: <project-root>/<branch-slug>/review.md
			newBranchDir := filepath.Join(projectRoot, branchSlug)
			newReviewFile := filepath.Join(newBranchDir, "review.md")

			// Skip if the new review.md already exists to avoid silent overwrites.
			if _, serr := os.Stat(newReviewFile); serr == nil {
				fmt.Fprintf(os.Stderr,
					"sitatame: migration: new review already exists for %s; skipping copy from legacy\n",
					branchSlug)
				continue
			}

			if err := os.MkdirAll(newBranchDir, 0o700); err != nil {
				fmt.Fprintf(os.Stderr,
					"sitatame: migration: mkdir %s: %v (skipping)\n", newBranchDir, err)
				continue
			}
			if err := copyFile(latest, newReviewFile); err != nil {
				fmt.Fprintf(os.Stderr,
					"sitatame: migration: copy %s -> %s: %v (skipping)\n", latest, newReviewFile, err)
				continue
			}
			migrated++
		}
	}

	return migrated, legacyDir, nil
}

// dirExists returns true if path exists and is a directory.
func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// latestReviewFile returns the path of the most recently modified .md file
// directly under dir, or "" if the directory is empty or contains no .md files.
func latestReviewFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var latest string
	var latestMod int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); mod > latestMod {
			latestMod = mod
			latest = filepath.Join(dir, e.Name())
		}
	}
	return latest, nil
}

// copyFile copies the content of src to dst, creating dst if it does not
// exist. dst is written with 0o600 permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return out.Sync()
}
