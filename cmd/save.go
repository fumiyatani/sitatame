package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/fumiyatani/sitatame/internal/review"
	"github.com/fumiyatani/sitatame/internal/tui"
)

// finalizeReview takes the TUI result and persists it according to the user's
// quit reason:
//
//   - tui.QuitPromote: save a draft, then promote it into reviews/, and print
//     the resolved absolute path as a `SITATAME_REVIEW=<path>` machine-readable
//     line on stdout. Exit code 0 on success.
//   - tui.QuitDraft: save the in-memory review under drafts/ and exit 1, which
//     is the "user explicitly bailed without promoting" signal callers expect.
//   - any other QuitReason (e.g. QuitNone from a short-circuited test runner):
//     leave the filesystem untouched and exit 0.
//
// Errors at any step are written to env.Stderr and surface as exit code 1.
// The function intentionally does not panic: panic-on-shutdown recovery is
// handled upstream in runTUIWithShutdown.
func finalizeReview(env Env, store *review.Store, result TUIResult) int {
	switch result.Reason {
	case tui.QuitPromote:
		draftPath, err := store.SaveDraft(&result.Review)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sitatame: save draft: %v\n", err)
			return 1
		}
		finalPath, err := store.Promote(draftPath)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sitatame: promote: %v\n", err)
			return 1
		}
		abs, _ := filepath.Abs(finalPath)
		fmt.Fprintf(env.Stdout, "SITATAME_REVIEW=%s\n", abs)
		return 0
	case tui.QuitDraft:
		if _, err := store.SaveDraft(&result.Review); err != nil {
			fmt.Fprintf(env.Stderr, "sitatame: save draft: %v\n", err)
			return 1
		}
		return 1
	}
	// QuitNone or any other value: runner returned without a save signal — exit
	// cleanly without touching the filesystem. In production this only happens
	// if the TUI is short-circuited (e.g. test stubs).
	return 0
}
