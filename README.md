# sitatame

[日本語版 README](README.ja.md)

Terminal UI for reviewing your own git diff before opening a pull request.
`sitatame` runs `git diff <base>..HEAD` inside a bubbletea TUI, lets you
attach 4 grains of comments (review-level, file, line, range), and saves the
result as a Markdown + YAML front-matter file under `.sitatame/reviews/` that
downstream agents can ingest.

## Build / install

`sitatame` requires Go 1.26 or later and `git` on `$PATH`.

```sh
git clone https://github.com/tanifumiya/sitatame
cd sitatame
make build      # produces ./sitatame
make install    # go install ./... — places sitatame on $GOBIN
```

Cross-build for the four supported targets:

```sh
make build-all  # writes dist/sitatame-{darwin,linux}-{amd64,arm64}
```

> Phase 2 will publish prebuilt artifacts via GitHub Releases and enable
> `go install github.com/tanifumiya/sitatame@<version>` for one-shot setup.

## Usage

Run inside a git working tree:

```sh
sitatame                # auto-detect base (origin/HEAD, @{upstream}, main, …)
sitatame origin/main    # explicit base
sitatame search TODO    # grep saved reviews under .sitatame/reviews/
```

Keys:

```
j / k       cursor down / up
n / p       next / previous file
V           start range selection (extend with j/k, Esc to clear)
c           comment at the cursor (kind auto-decided from selection / row)
R           review-level comment (edits review_comment in front matter)
s           save & promote — writes .sitatame/reviews/<slug>/<id>.md
            and prints SITATAME_REVIEW=<abs path> on stdout
q           save as draft and exit 1 (drafts/<slug>/<id>.md)
?           toggle help
Esc         close modal / clear selection
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
   SITATAME_REVIEW=/abs/path/to/.sitatame/reviews/<slug>/<id>.md
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
[`examples/agent-handoff.sh`](examples/agent-handoff.sh).

`q` on the other hand exits 1 and leaves a draft under
`.sitatame/drafts/<slug>/<id>.md` — pick it up next session, or feed it to an
agent that knows to look at drafts before starting work.

### Reviewing uncommitted changes

`sitatame` diffs `<base>..HEAD`, so staged / unstaged changes don't appear in
the TUI yet. The workaround is a temporary commit:

```sh
git add -A
git commit -m "wip:review"
sitatame HEAD~1
git reset --soft HEAD~1   # undo the commit, keep changes in index / working tree
```

Native `--staged` / `--working` flags are deferred to Phase 2.

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

## Phase 2 (not in this MVP)

- GitHub Release / goreleaser-driven distribution; `go install <module>@<version>`
- Homebrew tap / aqua / mise integration
- delta pipe integration for syntax-highlighted diffs
- side-by-side and tree-pane layouts
