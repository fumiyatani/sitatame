# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-06-14

### Breaking Changes

- **Storage layout rewrite (1-branch-1-file).**
  The previous `drafts/<branch-slug>/` + `reviews/<branch-slug>/` dual-directory
  layout is replaced by a single `<branch-slug>/review.md` file per branch.
  Existing `drafts/` and `reviews/` directories are automatically migrated on the
  next startup (`MigrateLegacyLayout`), and a timestamped `.legacy-<ts>/` backup is
  created alongside the migrated content.

- **Keybinding changes.**
  `q` now discards the current session (previously it saved to draft on quit).
  `Esc` at the top level does nothing (previously it could trigger quit flows).
  Draft promotion via the `s` key is removed in favour of the unified `review.md`
  write-on-quit model (`QuitSave` / `QuitDiscard`).

- **IntelliJ Plugin: `PromoteReviewAction` removed.**
  The "Promote draft to review" action is no longer registered in `plugin.xml`.
  `ReviewStore` is replaced by the combined `SaveReview` path that writes directly
  to `<branch-slug>/review.md`, matching the CLI layout.

- **IntelliJ Plugin: `Paths` API renamed.**
  `Paths.reviewsDir()` / `Paths.draftsDir()` are replaced by
  `Paths.branchDir()` / `Paths.reviewFile()` to reflect the 1-file layout.

- **AI skill (`sitatame-review-apply`) search path changed.**
  The skill now looks for `~/.claude/skills/sitatame-review-apply/` (user-global).
  Previously the skill lived at `<repo>/.claude/skills/sitatame-review-apply/`.

### Added

- **`--new` flag.** Refuse to start if `review.md` already exists for the current
  branch. Useful when you want to guarantee a fresh session without overwriting
  existing notes.

- **`--force-new` flag.** Backs up the existing `review.md` to `review.md.bak`
  and starts a fresh session. Mutually exclusive with `--new`.

- **Rescue mechanism.** Before overwriting `review.md`, the IntelliJ plugin writes
  a sidecar `review.md.rescue.<timestamp>.json` that captures the comment count at
  the time of save. This prevents silent data loss when a concurrent write from a
  different client races the plugin's save.

- **Automatic migration.** `MigrateLegacyLayout` runs at startup and transparently
  moves existing `drafts/` + `reviews/` trees into the new 1-branch-1-file layout,
  logging a summary to stderr.

- **Pruning contract documented.** `SKILL.md` and inline code comments define when
  stale `.legacy-<ts>/` backups and `review.md.rescue.*.json` sidecars should be
  pruned (manual for now; `sitatame prune` is a future candidate).

- **`.legacy-*` excluded from `sitatame search`.** The search scanner skips legacy
  backup directories so they do not pollute search results after migration.

### Changed

- **`cmd/save.go` rewritten** around `QuitSave` / `QuitDiscard` events, removing
  the three-state draft/promote/discard flow.

- **`--staged` and `--working` are mutually exclusive** with an explicit base
  argument (documented in `--help`).

- **Web `Paths` API aligned** with the 1-file layout (`BranchDir` / `ReviewFile`).
  `ReviewLoader` reads from `<branch-slug>/review.md` instead of iterating
  `reviews/<branch-slug>/`.

- **IntelliJ ToolWindow** no longer shows "Promote" button; the status bar reflects
  the unified save path.

### Fixed

- **Issue #76: YAML `Encode` round-trip bug.**
  A bug where `review.md` written by the Go encoder could not be decoded back
  without data loss (field reordering / missing scalar quoting) was fixed.
  The codec now passes a bit-exact round-trip test against all fixtures in
  `examples/`.

- **Issue #118: detached HEAD slug mismatch between IntelliJ and TUI.**
  The IntelliJ plugin previously passed the raw 40-char SHA directly to
  `Slug.branchSlug`, while the TUI normalised detached HEAD to
  `"detached/<sha[:12]>"` before calling `BranchSlug`. The result was that the
  same commit produced two different on-disk directories, splitting review data
  across tools. The plugin now mirrors the TUI normalisation exactly.

#### Manual migration note for pre-#118 detached HEAD reviews

If you used the IntelliJ plugin while in detached HEAD state before this fix,
you may have orphan review files at:

```
~/.sitatame/<project-slug>/branch__<sha1-8 of full 40-char SHA>/review.md
```

These directories are no longer written to. To preserve the data, manually
rename each such directory to the new form:

```
~/.sitatame/<project-slug>/detached_<sha12>__<sha1-8 of "detached/<sha12>">/
```

Where `<sha12>` is the first 12 characters of the original 40-char SHA.
Alternatively, delete the orphan directories if the review data is no longer
needed.

### Removed

- `drafts/` and `reviews/` top-level subdirectories under `~/.sitatame/<project>/`
  (superseded by `<branch-slug>/review.md`).
- `PromoteReviewAction` and its keyboard shortcut from the IntelliJ plugin.
- Draft auto-load on startup (the single `review.md` auto-loads instead).

---

## [0.1.0] - 2026-06-10

Initial public release.

- TUI diff review with bubbletea / lipgloss
- Per-branch review storage at `~/.sitatame/<project-slug>/reviews/<branch-slug>/`
- Draft auto-save and auto-load
- `sitatame search` subcommand
- IntelliJ IDEA plugin (Community 2024.3+) for inline comment display
- Compose for Web read-only review viewer (Ktor + WASM)
- `sitatame-review-apply` Claude AI skill
- Mouse wheel support and resolve toggle in TUI

[0.2.0]: https://github.com/fumiyatani/sitatame/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fumiyatani/sitatame/releases/tag/v0.1.0
