# TUI status — maintenance mode (2026-06)

This document records the maintenance policy for the terminal UI shipped
under `internal/tui/`. Active feature development for sitatame has moved
to the Kotlin Web UI (see [`web/`](../web/) once it lands). The TUI is
not deprecated and is still the only review surface that ships in the
default `sitatame` binary, but new features will land in the Web UI
first.

## Current feature surface

The TUI implements the full review loop today. The list below is the
contract a maintenance fix must preserve.

### Navigation

- `j` / `k` and `↓` / `↑` row-wise cursor movement
- `n` / `p` and `→` / `←` next / previous file
- mouse wheel scroll on the diff body, with the
  [PR #40](https://github.com/fumiyatani/sitatame/pull/40) help-overlay
  guard (wheel events are ignored while help is open)
- mouse click on a diff line moves the cursor there
- `f` opens the file-picker modal; arrow keys + Enter jump to the chosen
  file header

### Selection / range

- `r` toggles range-selection mode; `j` / `k` extend the selection
- `Esc` clears the selection (or closes the active modal, in priority
  order)

### Comments

- `c` opens the comment modal. The kind (review / file / line / range)
  is inferred from selection state and cursor row
- `Shift+R` opens the review-level comment modal directly
- `x` toggles the resolved state of the comment under the cursor
  (`open` ↔ `resolved`; `stale` comments are read-only)
- comment gutter markers: `*` for open, `~` for stale

### Layout

- single-column diff layout (default)
- `Tab` toggles split layout: left pane is the editable diff, right pane
  is a read-only preview (added in PR #35)
- the bottom hint line is mode-aware: it narrows the visible hints when
  in selection, on a comment row, or while split preview is active

### Persistence

- `s` saves and promotes the review to
  `~/.sitatame/<project-slug>/reviews/<branch-slug>/<id>.md` and prints
  `SITATAME_REVIEW=<abs path>` on stdout
- `q` saves a draft to `~/.sitatame/<project-slug>/drafts/...` and exits
  with status 1
- YAML front matter + Markdown body; unknown front-matter keys round-trip
  via `yaml.Node` so external agents can extend the schema

### Help

- `?` toggles the help overlay
- the help overlay swallows mouse wheel events so they do not also
  scroll the underlying diff

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

- net-new features. File a Web-UI issue instead and we will scope it
  there
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

CI mirrors this split: the `Test` job is required, the
`TUI E2E (optional)` job runs with `continue-on-error: true` so a flaky
teatest run cannot block merges.

## Migration table — features moved to the Web UI

This table is a placeholder that should be filled in as Web-UI features
ship. For each row, the "TUI status" column should move from `active`
to `legacy` once the Web UI covers it on parity.

| Feature                          | TUI status | Web UI status | Notes |
| -------------------------------- | ---------- | ------------- | ----- |
| diff navigation (j/k, n/p)       | active     | TBD           |       |
| file picker (`f`)                | active     | TBD           |       |
| line / range / file comments     | active     | TBD           |       |
| review-level comment (`Shift+R`) | active     | TBD           |       |
| resolved toggle (`x`)            | active     | TBD           |       |
| split layout (`Tab`)             | active     | TBD           |       |
| help overlay (`?`)               | active     | TBD           |       |
| save / draft (`s` / `q`)         | active     | TBD           |       |
| `sitatame search`                | active     | TBD           |       |

## Reverting maintenance mode

If the Web UI is wound down and the TUI needs to become primary again:

1. Revert the `tui_e2e` build-tag commit. The
   `internal/tui/scenarios_*_test.go`, `internal/tui/scenario_runner_test.go`
   and `internal/replay/` files should drop their `//go:build tui_e2e`
   lines
2. Move `TestScenario_FilePickerJumpFlow` back into
   `internal/tui/filepicker_test.go` (or leave it as its own file — the
   only reason it was extracted was to keep the default suite compiling
   with the tag off)
3. Fold the `tui-e2e` CI job back into `test`, or promote it to
   required by removing `continue-on-error: true`
4. Drop this document and the README banners pointing at it
