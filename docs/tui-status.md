# TUI status — maintenance mode (2026-06)

This document records the maintenance policy for the terminal UI shipped
under `internal/tui/`. Active feature development for sitatame has moved
to the Kotlin Web UI (scoped under [`web/`](../web/); Phase 0 PoC
merged, Phase 1 read-only viewer tracked in
[#66](https://github.com/fumiyatani/sitatame/issues/66)) and the
IntelliJ Plugin (tracked in
[#68](https://github.com/fumiyatani/sitatame/issues/68); no code merged
yet — the `intellij/` directory will land with that issue). The TUI is
not deprecated and is still the only review surface that ships in the
default `sitatame` binary, but new features land in the Web UI or the
IntelliJ Plugin first.

## Current feature surface

The TUI implements the full review loop today. The list below is the
contract a maintenance fix must preserve. Every binding is sourced from
[`internal/tui/keys.go`](../internal/tui/keys.go) and the dispatcher in
[`internal/tui/model.go`](../internal/tui/model.go); the help overlay in
[`internal/tui/help.go`](../internal/tui/help.go) mirrors the same list.

### Navigation

- `j` / `k` and `↓` / `↑` row-wise cursor movement (`KeyDown` /
  `KeyUp` plus their `KeyDownArrow` / `KeyUpArrow` aliases)
- `n` / `p` and `→` / `←` next / previous file (`KeyNextFile` /
  `KeyPrevFile` plus their `KeyRightArrow` / `KeyLeftArrow` aliases)
- `Tab` toggles unified ↔ split (preview) layout (`KeyToggleLayout`)
- `f` opens the file-picker modal; `j` / `k` / `↑` / `↓` move the
  highlight, `Enter` jumps to the chosen file header, `Esc` cancels
  (`KeyFilePicker`; the modal owns its own update loop in
  `updateFilePicker`)
- mouse wheel scrolls the diff body at `mouseWheelStep = 3` rows per
  tick. The help overlay swallows wheel events so the diff behind it
  cannot scroll invisibly (the
  [PR #40](https://github.com/fumiyatani/sitatame/pull/40) guard)
- left-click on a diff line moves the cursor there; clicks on the status
  bar / hint line are silently dropped

### Selection / range

- `r` toggles range-selection mode (`KeySelectKey`); `j` / `k` / `↑` /
  `↓` extend the selection
- `Esc` clears the selection or closes the active modal (priority:
  modal → help overlay → selection)

### Comments

- `c` opens the comment modal. The kind (line / range / file) is
  inferred from selection state and cursor row by
  [`openCommentModal`](../internal/tui/modal.go); file-scope picks
  `SideBase` for deleted files and `SideHead` everywhere else
- `Shift+R` opens the review-level comment modal directly and pre-loads
  the existing top-level `review_comment` for in-place editing
- inside the modal: `Ctrl+S` confirms and appends the comment; `Esc`
  cancels without saving
- `x` toggles the resolved state of the comment under the cursor
  (`KeyResolveToggle`). State machine: `open` ↔ `resolved`; `stale`
  comments are read-only. Sticky undo: a follow-up `x` on the same row
  re-targets the previously-toggled anchor via `lastToggledAnchor` so
  the action and the hint label never diverge (see
  [`resolveTarget`](../internal/tui/model.go))
- comment gutter markers: `*` for open, `~` for stale

### Layout

- single-column unified diff layout (default — `LayoutUnified`)
- split preview layout via `Tab` (`LayoutSplit`): left pane is the
  unified editable diff, right pane is a read-only side-by-side preview
  with paired `-` / `+` rows ([`buildSplitRows`](../internal/tui/split.go);
  added in [PR #35](https://github.com/fumiyatani/sitatame/pull/35))
- split is preview-only: `r`, `c`, `R`, `x`, `f` are guarded with the
  `split is preview-only — press Tab to return` status message
- the bottom hint line is mode-aware: it narrows the visible hints when
  in selection, on a comment row, or while split preview is active

### Persistence

- `s` saves and promotes the review to
  `~/.sitatame/<project-slug>/reviews/<branch-slug>/<id>.md` and prints
  `SITATAME_REVIEW=<abs path>` on stdout (`QuitPromote`)
- `q` saves a draft to `~/.sitatame/<project-slug>/drafts/...` and exits
  with status 1 (`QuitDraft`)
- `Ctrl+C` is an alias of `q` (`KeyQuitCtrl`)
- YAML front matter + Markdown body; unknown front-matter keys round-trip
  via `yaml.Node` so external agents can extend the schema

### Help

- `?` toggles the help overlay (`KeyHelp`)
- the help overlay swallows mouse wheel events so they do not also
  scroll the underlying diff; `f` is suppressed while help is open so
  the picker cannot stack on top of the overlay

### Subcommands shipped in the same binary

- `sitatame [base]` — launch the TUI against `<base>..HEAD`
- `sitatame --staged` — review `git diff --cached` (index vs HEAD)
- `sitatame --working` — review the worktree against HEAD (staged +
  unstaged)
- `sitatame search <pattern>` — grep saved reviews under
  `~/.sitatame/<project-slug>/reviews/`. Uses `ripgrep` when present and
  falls back to the Go regexp implementation in `internal/search/`

## TUI ↔ Web UI ↔ IntelliJ Plugin feature parity

The table below tracks each TUI capability against the two parallel
frontends so a future decision to freeze one of them has a concrete
inventory to work from. Status legend:

- **Shipped** — already merged into `main` on that surface
- **Phase 1 (in flight)** — scoped in the current Phase 1 issue, not
  yet merged
- **Phase 2** — explicitly out of Phase 1 scope per the tracking issue
- **Planned** — beyond the current tracking issues; no merged code and
  no Phase 1 commitment yet
- **Native (platform)** — provided by the browser / IDE itself; no
  sitatame code required on that surface
- **N/A** — not planned for that surface (different interaction model)

Web UI ([#66](https://github.com/fumiyatani/sitatame/issues/66)) status
today: only the Phase 0 YAML round-trip PoC under `web/` is merged;
the Phase 1 read-only viewer is tracked in
[#72](https://github.com/fumiyatani/sitatame/pull/72) and is still
in flight. IntelliJ Plugin ([#68](https://github.com/fumiyatani/sitatame/issues/68))
status today: no code merged, the `intellij/` directory does not yet
exist on `main`.

| Feature                                  | TUI           | Web UI (Compose Multiplatform Web, [#66](https://github.com/fumiyatani/sitatame/issues/66)) | IntelliJ Plugin ([#68](https://github.com/fumiyatani/sitatame/issues/68)) |
| ---------------------------------------- | ------------- | ------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| Cursor navigation (`j`/`k`/↑/↓)          | Shipped       | Phase 2 (`LazyColumn` scroll only in Phase 1)                                               | N/A (IDE caret)                                                           |
| Next / previous file (`n`/`p`/←/→)       | Shipped       | Phase 2                                                                                     | N/A (IDE project view)                                                    |
| File picker (`f`)                        | Shipped       | Phase 2                                                                                     | N/A (IDE Goto File)                                                       |
| Mouse wheel scroll                       | Shipped       | Native (browser)                                                                            | Native (IDE)                                                              |
| Mouse click to position cursor           | Shipped       | Native (browser)                                                                            | Native (IDE)                                                              |
| Range selection (`r` → `j`/`k`)          | Shipped       | Phase 2                                                                                     | Phase 1 (in flight; editor selection drives Add Comment)                  |
| Line comment (`c` on a line)             | Shipped       | Phase 2 (Phase 1 is read-only viewer)                                                       | Phase 1 (in flight; `Cmd+Shift+C` on caret)                               |
| Range comment (`c` after `r`)            | Shipped       | Phase 2                                                                                     | Phase 1 (in flight; `Cmd+Shift+C` on selection)                           |
| File-scope comment                       | Shipped       | Phase 2                                                                                     | Phase 2                                                                   |
| Review-level comment (`Shift+R`)         | Shipped       | Phase 2                                                                                     | Phase 2                                                                   |
| Modal confirm (`Ctrl+S`)                 | Shipped       | Phase 2                                                                                     | Phase 1 (in flight; `DialogWrapper` OK button)                            |
| Resolved toggle (`x`)                    | Shipped       | Phase 2                                                                                     | Phase 1 (in flight; `Cmd+Shift+R` / Tool Window action)                   |
| Stale comments read-only                 | Shipped       | Phase 1 (in flight; rendered with state badge)                                              | Phase 1 (in flight; state colour in Tool Window)                          |
| Comment gutter markers (`*` / `~`)       | Shipped       | Phase 2 (heatmap planned for Phase 1)                                                       | Phase 2 (gutter bar)                                                      |
| Split / side-by-side layout (`Tab`)      | Shipped       | Phase 2                                                                                     | N/A (IDE diff viewer covers this)                                         |
| Help overlay (`?`)                       | Shipped       | Phase 2 (keyboard shortcuts panel)                                                          | Native (IDE Keymap)                                                       |
| Save & promote (`s`)                     | Shipped       | Phase 2 (Phase 1 is read-only)                                                              | Phase 1 (in flight; `Promote Review` action)                              |
| Save as draft (`q`)                      | Shipped       | Phase 2                                                                                     | Phase 1 (in flight; automatic on `Add Comment`)                           |
| `SITATAME_REVIEW=` stdout handoff        | Shipped       | Phase 2                                                                                     | Phase 1 (in flight; IDE Notification)                                     |
| Read latest review                       | Shipped       | Phase 1 (in flight; `GET /api/v1/reviews/latest`)                                           | Phase 1 (in flight; Tool Window list)                                     |
| Auto-detect base (`origin/HEAD`, …)      | Shipped       | Phase 2                                                                                     | Phase 2 (Git4Idea)                                                        |
| `--staged` / `--working` flags           | Shipped       | Phase 2                                                                                     | Phase 2                                                                   |
| `sitatame search <pattern>`              | Shipped (CLI) | Phase 2                                                                                     | Phase 2                                                                   |
| AI prompt export                         | N/A           | Phase 2                                                                                     | Phase 1 (in flight; `Copy AI Prompt` action)                              |
| YAML round-trip with unknown-key preserve| Shipped       | Shipped (snakeyaml-engine 2.9 PoC, [#62](https://github.com/fumiyatani/sitatame/issues/62)) | Phase 1 (in flight; shares the Web UI `Codec.kt`)                         |

Rows marked **Phase 1 (in flight)** depend on the open issues / PRs
linked above and are not yet merged on `main`. Rows marked **Phase 2**
are explicit non-goals of the current Phase 1 issues. If a Phase 1 row
merges or a Phase 2 row ships on Web UI / IntelliJ Plugin, update the
corresponding row and the freeze-decision section below before
merging.

## What "maintenance mode" means

### Accepted fixes

- crash / panic / data-loss bugs in any of the surfaces above
- regressions against a previously-shipped behaviour (the scenario suite
  under `internal/tui/scenarios_test.go` enumerates the ones we care
  about by name)
- security fixes
- dependency bumps (bubbletea / bubbles / lipgloss / runewidth / vt10x /
  teatest) when they are required by the build
- documentation / typo fixes
- minor ergonomic touch-ups that do not change keybindings or the
  rendered layout — e.g. tightening a hint string, adjusting a colour
  pair within an existing theme slot

### Not accepted

- net-new features. File a Web-UI issue ([#66](https://github.com/fumiyatani/sitatame/issues/66)
  tracker) or an IntelliJ Plugin issue ([#68](https://github.com/fumiyatani/sitatame/issues/68))
  instead and we will scope it there
- breaking changes to keybindings or the saved-review on-disk format
- new external dependencies (new Go modules, new CLI tools required at
  runtime)
- refactors whose only justification is "this would be cleaner". The
  refactor cost is real; the maintenance throughput is small

If you are unsure which bucket a change falls into, open an issue
referencing this document before writing code.

## Testing policy

The default `go test ./...` (and `make test`) only run the unit and
golden-snapshot suites. The teatest scenario harness and the pty smoke
under `internal/replay/` are gated behind the `tui_e2e` build tag:

- `make test` — required suite, fast, no virtual terminal
- `make test-tui-e2e` — full suite including scenarios + pty
- `make update-golden` — refresh classic `internal/tui/testdata/`
  goldens (untagged)
- `make update-golden-tui-e2e` — refresh scenario goldens under
  `internal/tui/testdata/scenarios/` (tagged)

The build-tag gate lives on the following files; reverse the list when
unfreezing the TUI (see below):

- `internal/tui/scenario_runner_test.go`
- `internal/tui/scenarios_test.go`
- `internal/tui/scenarios_filepicker_test.go`
- `internal/replay/replay.go`
- `internal/replay/replay_test.go`

CI mirrors this split: the `Test` job is required, the
`TUI E2E (optional)` job runs with `continue-on-error: true` so a flaky
teatest run cannot block merges.

## Decision points — freezing the TUI

When the Web UI or the IntelliJ Plugin reaches parity with the rows in
the comparison table above, the maintainer may freeze the TUI. "Freeze"
means: keep the binary entry point, but stop accepting even bug fixes
for the rendered surface and drop the `tui_e2e` job from CI.

Before flipping the switch:

1. Update the comparison table so the candidate replacement shows
   **Shipped** on every row the TUI marks **Shipped**.
2. Confirm the replacement surface emits the same
   `SITATAME_REVIEW=<abs>` handoff line (or document the new contract
   loudly in the top-level README).
3. Snapshot the current `internal/tui/testdata/scenarios/` set so the
   freeze can be reverted without re-running the full scenario suite by
   hand.

## Reverting maintenance mode (unfreeze)

If the Web UI or the IntelliJ Plugin is wound down and the TUI needs to
become primary again:

1. Revert the `tui_e2e` build-tag commit. The
   `internal/tui/scenarios_*_test.go`, `internal/tui/scenario_runner_test.go`
   and `internal/replay/` files should drop their `//go:build tui_e2e`
   lines.
2. Move `TestScenario_FilePickerJumpFlow` back into
   `internal/tui/filepicker_test.go` (or leave it as its own file — the
   only reason it was extracted was to keep the default suite compiling
   with the tag off).
3. Fold the `tui-e2e` CI job back into `test`, or promote it to
   required by removing `continue-on-error: true`.
4. Drop this document and the README banners pointing at it. Search for
   `tui-status.md` across the repo to catch every link.
5. If a frontend was retired in the process, also remove its directory
   (`web/` or `intellij/`) and the matching CI workflow
   (`.github/workflows/web-ci.yml`, `.github/workflows/intellij-ci.yml`)
   in the same change so the comparison table above does not lie.
