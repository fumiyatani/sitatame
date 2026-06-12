# sitatame

[日本語版 README](README.ja.md)

Terminal UI for reviewing your own git diff before opening a pull request.
`sitatame` runs `git diff <base>..HEAD` inside a bubbletea TUI, lets you
attach 4 grains of comments (review-level, file, line, range), and saves the
result as a Markdown + YAML front-matter file under
`~/.sitatame/<project-slug>/reviews/` that downstream agents can ingest.

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

## Build / install

`sitatame` requires Go 1.26 or later and `git` on `$PATH`.

```sh
git clone https://github.com/fumiyatani/sitatame
cd sitatame
make build      # produces ./sitatame
make install    # go install ./... — places sitatame on $GOBIN
```

Cross-build for the four supported targets:

```sh
make build-all  # writes dist/sitatame-{darwin,linux}-{amd64,arm64}
```

> Phase 2 will publish prebuilt artifacts via GitHub Releases and enable
> `go install github.com/fumiyatani/sitatame@<version>` for one-shot setup.

## Usage

Run inside a git working tree:

```sh
sitatame                # auto-detect base (origin/HEAD, @{upstream}, main, …)
sitatame origin/main    # explicit base
sitatame --staged       # review staged changes (index vs HEAD)
sitatame --working      # review all uncommitted changes (worktree vs HEAD)
sitatame search TODO    # grep saved reviews under ~/.sitatame/<project-slug>/reviews/
```

Keys:

```
j / k       cursor down / up
n / p       next / previous file
↑ ↓ ← →     arrow-key aliases for k / j / p / n
wheel       scroll the diff (hold Option/Fn to text-select)
r           start range selection (extend with j/k, Esc to clear)
c           comment at the cursor (kind auto-decided from selection / row)
x           toggle resolved on the comment under the cursor (open ↔ resolved; stale skipped)
Shift+R     review-level comment (edits review_comment in front matter)
s           save & promote — writes ~/.sitatame/<project-slug>/reviews/<branch-slug>/<id>.md
            and prints SITATAME_REVIEW=<abs path> on stdout
q           save as draft and exit 1 (~/.sitatame/<project-slug>/drafts/<branch-slug>/<id>.md)
?           toggle help
Esc         close modal / clear selection

Inside the comment modal:
Ctrl+S      confirm and append the comment
Esc         cancel without saving
```

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
2. The reviewer reviews and presses `s` (save & promote).
3. `sitatame` exits 0 and writes one machine-readable line to **stdout**:

   ```
   SITATAME_REVIEW=/abs/path/to/.sitatame/<project-slug>/reviews/<branch-slug>/<id>.md
   ```

4. Capture that path. The file is YAML front matter + Markdown body. The
   front matter includes `schema`, `branch`, `base.{ref,sha}`,
   `head.{ref,sha}`, and a `comments` list whose entries carry
   `kind: review|file|line|range`, `path`, `side`, optional `line` /
   `line_start` / `line_end`, blob hashes for staleness detection, and a
   `body`. Anything not modeled by `sitatame` is preserved verbatim, so an
   agent can extend the schema without losing data on the next save.
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

`q` on the other hand exits 1 and leaves a draft under
`~/.sitatame/<project-slug>/drafts/<branch-slug>/<id>.md` — pick it up next
session, or feed it to an agent that knows to look at drafts before starting
work.

### Storage location

Reviews and drafts live outside the repository tree so you don't have to add
ignore rules per project. The output root resolves in this order:

1. `$SITATAME_HOME` if set and non-empty
2. `~/.sitatame` (default)
3. `$TMPDIR/sitatame` as a last-resort fallback (with a stderr warning)

Under the output root each repository checkout gets its own
`<project-slug>/` directory, derived from the basename plus a short hash of
the absolute repo path. Distinct checkouts of the same repository (e.g.
worktrees) therefore stay separated.

If a legacy `<repo>/.sitatame/` directory still exists from before this
change, `sitatame` prints a one-line stderr notice on startup but does not
auto-migrate or read from it. Delete it once you've copied anything you want
to keep.

To migrate drafts from the legacy in-repo location, copy the printed target
path out of stderr — `sitatame` prints the resolved drafts root so you don't
have to compute the `<project-slug>` by hand:

```sh
# Run once, inside the repo with the legacy directory. The second stderr line
# from `sitatame` looks like:
#   sitatame: To migrate drafts: mkdir -p '/Users/you/.sitatame/<project-slug>/drafts' && mv '/path/to/repo/.sitatame'/drafts/* '/Users/you/.sitatame/<project-slug>/drafts'/
# Paths are POSIX single-quoted so spaces and shell metacharacters in your
# checkout or `$SITATAME_HOME` survive copy-paste; the `/drafts/*` glob is
# left outside the closing quote so the shell still expands it. Run that
# printed line (the `mkdir -p` is there so the first upgrade does not fail
# when the new drafts root doesn't exist yet), then remove the empty legacy
# directory. The form below is the same shape but uses `~` for brevity — if
# your repo path or `$SITATAME_HOME` contains spaces, prefer copy-pasting the
# stderr line verbatim instead.
mkdir -p ~/.sitatame/<project-slug>/drafts && mv .sitatame/drafts/* ~/.sitatame/<project-slug>/drafts/ 2>/dev/null || true
rm -rf .sitatame
```

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

- GitHub Release / goreleaser-driven distribution; `go install <module>@<version>`
- Homebrew tap / aqua / mise integration
- delta pipe integration for syntax-highlighted diffs
- side-by-side and tree-pane layouts
