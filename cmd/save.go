package cmd

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/fumiyatani/sitatame/internal/clipboard"
	"github.com/fumiyatani/sitatame/internal/review"
	"github.com/fumiyatani/sitatame/internal/tui"
)

// finalizeReview takes the TUI result and persists it according to the user's
// quit reason:
//
//   - tui.QuitSave: call SaveReview. If the review is non-empty, print
//     `SITATAME_REVIEW=<abs>` on stdout and exit 0. If it is empty
//     (SaveReview no-ops) exit 0 without printing.
//   - tui.QuitDiscard: do nothing — the in-memory review is discarded.
//     Exit 0 (the user made a deliberate choice to abandon the session).
//   - any other QuitReason (QuitNone from a short-circuited test runner):
//     leave the filesystem untouched and exit 0.
//
// Errors are written to env.Stderr and surface as exit code 1.
//
// noClipboard suppresses the automatic clipboard copy (equivalent to
// --no-clipboard / SITATAME_NO_CLIPBOARD=1).
func finalizeReview(env Env, store *review.Store, result TUIResult, noClipboard bool) int {
	switch result.Reason {
	case tui.QuitSave:
		finalPath, err := store.SaveReview(&result.Review)
		if err != nil {
			printSaveReviewError(env, err)
			return 1
		}
		if finalPath == "" {
			// Empty review: no file written, exit cleanly.
			return 0
		}
		abs, _ := filepath.Abs(finalPath)
		fmt.Fprintf(env.Stdout, "SITATAME_REVIEW=%s\n", abs)

		if !noClipboard {
			copyFn := env.Clipboard
			if copyFn == nil {
				copyFn = clipboard.Copy
			}
			if err := copyFn(abs); err != nil {
				fmt.Fprintf(env.Stderr, "sitatame: clipboard copy failed: %v\n", err)
			} else {
				fmt.Fprintf(env.Stderr, "sitatame: path copied to clipboard\n")
			}
		}
		return 0
	case tui.QuitDiscard:
		// User explicitly chose to discard — do not write anything.
		return 0
	}
	// QuitNone or any other value: runner returned without a save signal.
	return 0
}

// printSaveReviewError writes a human-readable error to env.Stderr. When the
// error is a RescueError (i.e. Encode failed but rescue file was written), a
// prominent message with the rescue file path is emitted so the user can
// recover their work manually.
func printSaveReviewError(env Env, err error) {
	var re *review.RescueError
	if errors.As(err, &re) {
		fmt.Fprintf(env.Stderr,
			"sitatame: SAVE FAILED. Rescue file written to: %s. Open this file and recover manually.\n",
			re.RescuePath)
		return
	}
	fmt.Fprintf(env.Stderr, "sitatame: save review: %v\n", err)
}
