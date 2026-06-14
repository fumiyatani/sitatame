# sitatame

[日本語版 README](README.ja.md)

> **Project status (2026-06)**: the terminal UI is in maintenance mode.
> Active development has moved to the Kotlin Web UI under [`web/`](web/)
> and the IntelliJ Plugin under [`intellij/`](intellij/). TUI bug fixes
> are still welcome; new feature work targets the Web UI or the IntelliJ
> Plugin. See
> [docs/tui-status.md](docs/tui-status.md) for the maintenance policy,
> the full TUI feature inventory, and the parity table that tracks
> each capability across all three surfaces.

Terminal UI for reviewing your own git diff before opening a pull request.
`sitatame` runs `git diff <base>..HEAD` inside a bubbletea TUI, lets you
attach 4 grains of comments (review-level, file, line, range), and saves the
result as a Markdown + YAML front-matter file at
`~/.sitatame/<project-slug>/<branch-slug>/review.md` that downstream agents can ingest.

## Tech stack

- **Language / runtime**: Go 1.26 (`go 1.26.2` in `go.mod`)
- **Required external tool**: `git` on `$PATH`
- **Optional external tool**: `ripgrep` (`rg`) — used by `sitatame search` when
  available; a regexp-based Go fallback in `internal/search/` ships either way
- **TUI runtime**:
  [`charmbracelet/bubbletea`](https://github.com/charmbracelet/bubbletea) v1
  for the model / update / view loop,
  [`bubbles/textarea`](https://github.com/charmbracelet/bubbles) for the
  comment editor, plus `lipgloss` (indirect via bubbletea) for styling
- **Terminal width math**: [`mattn/go-runewidth`](https://github.com/mattn/go-runewidth)
  with `EastAsianWidth=false` so column math stays deterministic across locales
- **Persistence**: [`gopkg.in/yaml.v3`](https://gopkg.in/yaml.v3) with
  `yaml.Node` for unknown-key preservation across decode → encode round trips
- **Identifiers**: [`google/uuid`](https://github.com/google/uuid) for anchor
  IDs in saved reviews
- **Standard library leveraged**: `os/exec` (git plumbing & ripgrep handoff),
  `regexp`, `bufio`, `path/filepath`, `syscall` (TTY ioctl behind a build tag
  in `internal/termcheck/`), `time`, `flag`
- **Tooling**:
  - `go test ./...` for unit + integration tests
  - golden snapshots under `internal/tui/testdata/` (ANSI stripped before compare)
  - `BenchmarkUpdate_LargeDiff` for TUI hot path
  - `make build` / `make build-all` (darwin & linux × amd64 & arm64) /
    `make install` / `make bench` / `make update-golden` / `make vet`
- **Web UI**: Kotlin Multiplatform 2.1.10, Compose Multiplatform 1.7.3
  for the Wasm client, Ktor 3.0.3 for the JVM backend, Gradle wrapper
  8.11.1, JDK 21 toolchain
- **IntelliJ Plugin**: Kotlin/JVM 2.1.10, JetBrains IntelliJ Platform
  Gradle Plugin 2.1.0, target IntelliJ IDEA Community 2024.3
  (`sinceBuild=243`, `untilBuild=251.*`), bundled `Git4Idea`, JDK 21
  toolchain

## Build / install

`sitatame` requires Go 1.26 or later and `git` on `$PATH`. `git` is the only
runtime dependency — Go is only needed if you build from source or use
`go install`.

### Prebuilt binaries

Download an unsigned binary for your platform from the
[releases page](https://github.com/fumiyatani/sitatame/releases) — four
targets are published per tag: `sitatame-{darwin,linux}-{amd64,arm64}` plus
a `checksums.txt` with SHA-256 hashes.

```sh
# macOS arm64 example — adjust the asset name for your OS/arch.
curl -L https://github.com/fumiyatani/sitatame/releases/latest/download/sitatame-darwin-arm64 \
  -o /usr/local/bin/sitatame
chmod +x /usr/local/bin/sitatame
```

The macOS binaries are not yet code-signed or notarized, so Gatekeeper will
quarantine them on first launch. Clear the attribute manually if you trust
the download:

```sh
xattr -d com.apple.quarantine /usr/local/bin/sitatame
```

Signing / notarization and a Homebrew tap are planned for Phase 2.

### `go install`

```sh
go install github.com/fumiyatani/sitatame@latest
# or pin to a tagged release:
go install github.com/fumiyatani/sitatame@v0.1.0
```

### From source

```sh
git clone https://github.com/fumiyatani/sitatame
cd sitatame
make build      # produces ./sitatame
make install    # go install ./... — places sitatame on $GOBIN
```

Cross-build for the four supported targets (same matrix the release
workflow ships):

```sh
make build-all  # writes dist/sitatame-{darwin,linux}-{amd64,arm64}
```

## Web UI

The Web UI in [`web/`](web/) is a Kotlin Multiplatform implementation of the
sitatame review interface.  The JVM target runs a Ktor backend that reads the
current repository, runs `git diff origin/main..HEAD`, loads the latest review
Markdown from the shared sitatame storage directory, and exposes the result as
JSON.  The Wasm target renders that data with Compose for Web.

**Capabilities** (Phase 1 step 2 + Issue #18 UX):

- **Read**: unified diff view with file/hunk navigation and comment display
- **Write**:
  - Add line, range, file, or overall comments
  - Resolve / reopen comments with optimistic UI
  - Edit the review-level narrative comment
  - Conflict detection: ETag-based 412 handling with Reload + retry or Discard
- **Comment UX** (Issue #18):
  - GitHub-style threads: comments sharing the same anchor are grouped into one
    collapsible thread
  - State filter (`All / Open / Done / Stale`) narrows the thread list in the sidebar
  - Open / Stale threads expand by default; Resolved threads start collapsed
  - Reply to an existing thread with "Reply to this thread" — inherits the same anchor
  - Visual state distinction: open (default bg), resolved (dim grey), stale (amber)

Range comments use **long-press** to start range mode, then click the end line
(both must be in the same hunk).  An "Add range comment" button in the file
header provides an alternative entry point.

Requirements:

- JDK 21
- `git` on `$PATH`
- Network access on the first Gradle run if the JDK toolchain or Gradle
  dependencies are not already cached

Run the backend from the repository root:

```sh
cd web
./gradlew :run
```

The server binds to a random local port and prints the URL on stdout:

```text
SITATAME_WEB_URL=http://127.0.0.1:<port>
```

Open the printed URL in your browser.  To also run the Wasm frontend dev server
(hot-reload during UI development):

```sh
cd web
./gradlew :wasmJsBrowserDevelopmentRun
```

For build and smoke-test coverage:

```sh
cd web
./gradlew :jvmTest
./gradlew :wasmJsBrowserDistribution
```

Regenerate the shared YAML fixtures from the Go implementation before changing
the Kotlin codec or storage compatibility tests:

```sh
make web-fixtures
```

Current limitations:

- The diff base is hard-coded to `origin/main`.
- The production Wasm distribution is not yet wired into the Ktor static
  resources automatically; use `:wasmJsBrowserDevelopmentRun` for local UI
  development or run `:wasmJsBrowserDistribution` then `:run`.
- Shift+click for range mode is not yet supported (Compose for Web CMP 1.7.x
  limitation); use long-press instead.
- DELETE comment and force-overwrite on conflict are not yet implemented.
- No WebSocket/SSE push: changes from TUI or IntelliJ Plugin are visible only
  after a manual browser refresh or the next write conflict (412).

See [`web/README.md`](web/README.md) for the full module layout, API routes,
environment variables, and known limitations.  The full write-path specification
is at [`docs/web-ui-phase1-step2-spec.md`](docs/web-ui-phase1-step2-spec.md).

## IntelliJ Plugin

The IntelliJ plugin in [`intellij/`](intellij/) is an experimental IDE surface
for writing and consuming sitatame reviews. It can add line/range comments from
the editor, toggle comment state, list comments in the `SitatameReview` tool
window, promote a draft to `reviews/`, copy an AI-ready prompt, and configure a
plugin-level `SITATAME_HOME` override.

Requirements:

- JDK 21
- IntelliJ IDEA Community / Ultimate 2024.3 or newer
- Android Studio 2024.3.x is intended to work, but is not verified in the
  current CI matrix
- Network access on the first Gradle run if the IntelliJ SDK or dependencies
  are not already cached

Build the plugin zip:

```sh
cd intellij
./gradlew :buildPlugin
```

The zip is written to:

```text
intellij/build/distributions/sitatame-intellij-0.1.0.zip
```

Install it in the IDE with **Settings -> Plugins -> gear icon -> Install
Plugin from Disk...**, then select the generated zip and restart.

Launch a sandbox IDE with the plugin already loaded:

```sh
cd intellij
./gradlew :runIde
```

Run plugin tests:

```sh
cd intellij
./gradlew :test
```

Main entry points after installation:

- Editor context menu: `sitatame: Add Comment`
- Shortcut: `Cmd+Shift+C` on macOS, `Ctrl+Shift+C` elsewhere
- Editor context menu: `sitatame: Toggle Resolved`
- Shortcut: `Cmd+Shift+R` on macOS, `Ctrl+Shift+R` elsewhere
- Tool window: `SitatameReview`
- Settings: **Settings -> Tools -> sitatame review**

The plugin uses the same storage shape as the CLI and Web UI:

```text
$SITATAME_HOME/<project-slug>/<branch-slug>/review.md
```

See [`intellij/README.md`](intellij/README.md) for the detailed feature list,
storage notes, and plugin-specific limitations.

## Usage

Run inside a git working tree:

```sh
sitatame                # auto-detect base (origin/HEAD, @{upstream}, main, …)
sitatame origin/main    # explicit base
sitatame --staged       # review staged changes (index vs HEAD)
sitatame --working      # review all uncommitted changes (worktree vs HEAD)
sitatame --new          # refuse if review.md already exists for this branch
sitatame --force-new    # back up review.md to review.md.bak and start fresh
sitatame search TODO    # grep saved reviews under ~/.sitatame/<project-slug>/
```

Keys:

```
j / k       cursor down / up
n / p       next / previous file
↑ ↓ ← →     arrow-key aliases for k / j / p / n
f           file picker modal (jump to any file by name)
wheel       scroll the diff (hold Option/Fn to text-select)
r           start range selection (extend with j/k, Esc to clear)
c           comment at the cursor (kind auto-decided from selection / row)
x           toggle resolved on the comment under the cursor (open ↔ resolved; stale skipped)
Shift+R     review-level comment (edits review_comment in front matter)
s           save & exit — writes ~/.sitatame/<project-slug>/<branch-slug>/review.md
            atomically and prints SITATAME_REVIEW=<abs path> on stdout; exits 0
q           discard & exit — leaves review.md untouched; exits 1
?           toggle help
Esc         close modal / clear selection (no-op at top level)

Inside the comment modal:
Ctrl+S      confirm and append the comment
Esc         cancel without saving
```

The bottom hint line is mode-aware: it advertises the keys that are useful
in the current context (selection, existing-comment row, split preview) and
falls back to a compact form when the viewport is narrow.

Typical flows:

```
# line comment
sitatame → j/k to a content line → c → type body → Ctrl+S → s

# range comment
sitatame → j/k to the first line → r → j/k to extend → c → type body → Ctrl+S → s

# review-level comment (front matter)
sitatame → Shift+R → type body → Ctrl+S → s
```

Comment markers in the gutter:

- `*` open
- `~` stale (anchored content drifted; the comment is read-only)

## Agent integration

`sitatame` is designed so that another process — typically a coding agent —
can consume the human reviewer's notes without screen scraping:

1. Spawn `sitatame` against the branch you want reviewed.
2. The reviewer reviews and presses `s` (save & exit).
3. `sitatame` exits 0 and writes one machine-readable line to **stdout**:

   ```
   SITATAME_REVIEW=/abs/path/to/.sitatame/<project-slug>/<branch-slug>/review.md
   ```

4. Capture that path. The file is YAML front matter + Markdown body. The
   front matter includes `schema`, `branch`, `base.{ref,sha}`,
   `head.{ref,sha}`, and a `comments` list whose entries carry
   `kind: review|file|line|range`, `path`, `side`, optional `line` /
   `line_start` / `line_end`, blob hashes for staleness detection, and a
   `body`. Anything not modeled by `sitatame` is preserved verbatim, so an
   agent can extend the schema without losing data on the next save. The
   full field-by-field reference, side decision rules, state transitions,
   and forward-compat strategy live in
   [docs/review-schema.md](docs/review-schema.md).
5. `sitatame search <pattern>` is the read path for past reviews.

A minimal handoff in shell looks like:

```sh
REVIEW_PATH=$(sitatame HEAD~1 | awk -F= '/^SITATAME_REVIEW=/{print $2}')
test -n "$REVIEW_PATH" || { echo "no review captured" >&2; exit 1; }
cat "$REVIEW_PATH" | your-agent --consume-review
```

A runnable version of the same flow lives at
[`examples/agent-handoff.sh`](examples/agent-handoff.sh). The script feeds
the promoted Markdown into `$SITATAME_AGENT` via `sh -c`, so set that
variable only to commands you fully trust — never to values pulled from
untrusted sources.

`q` on the other hand exits 1 and leaves `review.md` untouched — the
previous `review.md` (if any) is still readable next session.

Re-launching `sitatame` on the same branch auto-loads the existing `review.md`
so prior comments reappear in the TUI; pressing `s` again overwrites the same
file atomically.

### Storage location

Reviews live outside the repository tree so you don't have to add ignore rules
per project. The output root resolves in this order:

1. `$SITATAME_HOME` if set and non-empty
2. `~/.sitatame` (default)
3. `$TMPDIR/sitatame` as a last-resort fallback (with a stderr warning)

Under the output root each repository checkout gets its own
`<project-slug>/` directory, derived from the basename plus a short hash of
the absolute repo path. Distinct checkouts of the same repository (e.g.
worktrees) therefore stay separated. Within that directory, each branch gets
`<branch-slug>/review.md` — one file per branch.

#### Migration from the pre-#76 layout

If you have a `~/.sitatame/<project-slug>/drafts/` or `reviews/` directory
from an earlier version, `sitatame` auto-migrates on the first run: the old
trees are moved into `.legacy-<timestamp>/` (data is preserved, not deleted)
and the latest review per branch is copied into the new `<branch-slug>/review.md`
location. `sitatame` prints a migration summary on stderr.

If a legacy `<repo>/.sitatame/` directory still exists from before the
output-root change (pre-#38), `sitatame` prints a one-line stderr notice on
startup but does not auto-migrate or read from it. Delete it once you've
copied anything you want to keep.

### Reviewing uncommitted changes

```sh
sitatame --staged    # review the index against HEAD (`git diff --cached`)
sitatame --working   # review the working tree against HEAD (staged + unstaged)
```

Both modes skip base auto-detection and record the review with
`base.ref: HEAD` plus `head.ref: INDEX` (for `--staged`) or
`head.ref: WORKTREE` (for `--working`). When there are no changes to review,
`sitatame` prints a friendly message and exits 0 without launching the TUI.

`--staged` and `--working` are mutually exclusive and cannot be combined with
an explicit base argument. Untracked files are not included; run
`git add -N <path>` first if you want them in the diff.

### Per-repo config

You can override the base auto-detect chain per repository by dropping a
[`<repo>/.sitatame/config.yaml`](docs/config.md) file with a `base` section.
The minimal form is:

```yaml
base:
  default: "origin/develop"   # tried first when no CLI base is given
```

See [docs/config.md](docs/config.md) for the full schema, error-handling
rules, and reserved-for-future-use sections.

## Development

```sh
make test            # go test ./...
make bench           # Update / View bench across a few thousand rows
make update-golden   # rewrite internal/tui/testdata/*.golden after intentional view changes
make vet
make fmt
```

The TUI tests use snapshot files in `internal/tui/testdata/`; ANSI escapes
are stripped before comparison so tests stay deterministic across hosts and
locales.

### Adding a scenario

`internal/tui/scenarios_test.go` runs declarative `tui.Scenario`s through a
`teatest`-backed harness. Each scenario lists `Steps` (one input event per
step) and an `Expectation` per step. The runner asserts view-level
substrings as the program runs (`ViewContains`, `ViewNotContains`,
`ViewGolden`) and asserts model-state fields (`Cursor`, `Top`, `Comments`,
`Layout`, `QuitReason`) against the final model after `tea.Quit`. View
assertions evaluate against the *currently visible terminal screen* — the
runner replays the cumulative bubbletea output through a `hinshun/vt10x`
virtual terminal so that delta-repaint frames reconstruct correctly. That
way `ViewContains` only matches what's on screen now and `ViewNotContains`
keeps working even after the forbidden text appeared in an earlier step.
To add a new regression case, append a
`Test_Scenario_<Name>` function that calls `runScenario(t, tui.Scenario{...})`;
see the seed scenarios in `scenarios_test.go` for examples and `scenario.go`
for the full DSL.

### PTY smoke tests

`internal/replay/` boots the compiled `sitatame` binary inside a
pseudo-terminal, mirrors its output through a [`vt10x`][vt10x] virtual
terminal, and asserts on the reconstructed screen. The tests cover the boot
path, `q` / `s` exit paths, the help modal toggle, SIGWINCH-style resize, and
xterm SGR-mode mouse wheel events.

```sh
go test ./internal/replay -count=1
```

Each scenario calls `replay.SkipIfNoPTY(t)` first and skips cleanly on hosts
where the kernel will not allocate a pty (sandboxed CI shells, containers
without `/dev/ptmx`). On a regular developer machine or a standard Linux CI
runner the tests execute the real binary end to end.

[vt10x]: https://github.com/hinshun/vt10x

## Phase 2 (not in this MVP)

- Code-signing / notarization of the darwin binaries (currently shipped unsigned)
- Homebrew tap / aqua / mise integration
- delta pipe integration for syntax-highlighted diffs
- side-by-side and tree-pane layouts

## See also

- [`docs/tui-status.md`](docs/tui-status.md) — TUI maintenance policy,
  full keybinding / feature inventory, and the parity table comparing
  the TUI against the Web UI (Compose Multiplatform Web) and the
  IntelliJ Plugin. Read this before filing a TUI feature request or
  scoping a fix.
- [`docs/config.md`](docs/config.md) — per-repo
  `<repo>/.sitatame/config.yaml` schema.
- [`web/README.md`](web/README.md) — Web UI scope, build, and the
  bit-exact YAML round-trip Kill criteria.
