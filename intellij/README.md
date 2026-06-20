# sitatame IntelliJ plugin (Phase 1)

A first-class IDE surface for sitatame code reviews. Authors and consumers
can add comments, browse a tool window of all open / resolved / stale
comments, save the review directly to `review.md`, and copy an AI-ready
prompt to the clipboard — all without leaving the editor.

**Status**: experimental. Not on JetBrains Marketplace yet; install from
disk.

## Requirements

- JDK 21 (IntelliJ 2024.3 bundles JBR 21)
- IntelliJ IDEA Community / Ultimate 2024.3 or newer
- Android Studio 2024.3.x (Jellyfish) — best-effort; not in CI matrix yet

## Build

```bash
cd intellij
./gradlew :buildPlugin
```

The plugin zip is produced under `intellij/build/distributions/`:

```
intellij/build/distributions/sitatame-intellij-0.1.0.zip
```

## Install from disk

1. Build the zip as above.
2. In your IDE: **Settings → Plugins → ⚙ → Install Plugin from Disk…**
3. Point at the generated zip and restart.

## Run in a sandbox IDE

```bash
cd intellij
./gradlew :runIde
```

This launches a clean IntelliJ Community instance with the plugin loaded.
The sandbox config and logs live under `intellij/build/idea-sandbox/`.

## Test

```bash
cd intellij
./gradlew :test
```

The `CodecTest` cases round-trip the `web/fixtures/*.yaml` files — same
fixtures the Web UI PoC (PR #65) uses — so a change to the Go-side YAML
emitter immediately fails one or both routes.

## Features (Phase 1)

| Action                        | Where                            | Shortcut             |
| ----------------------------- | -------------------------------- | -------------------- |
| Add comment (line / range)    | Editor right-click               | Cmd+Shift+C / Ctrl+Shift+C |
| Add file-level comment        | Editor right-click               | —                    |
| Add review-level comment      | Right-click / Find Action        | —                    |
| Toggle resolved/open          | Editor right-click               | Cmd+Shift+R / Ctrl+Shift+R |
| Go to next / prev comment     | Editor right-click               | Cmd+Shift+. / , (Ctrl on Win/Linux) |
| List comments + jump to line  | Tool window "sitatame review"    | —                    |
| Copy AI prompt                | Tool window toolbar              | —                    |
| Save review                   | Tool window toolbar              | —                    |
| Configure `SITATAME_HOME`     | Settings → Tools → sitatame review | —                  |

The line / range / file / review comment scopes match the Go TUI's `c`
(line/range), file-header `c` (file), and `Shift+R` (review-level) bindings.
"Go to next / prev comment" mirrors the TUI's keyboard-driven stepping through
commented lines so you can review without leaving the editor.

**Storage layout per comment kind** (matches Go TUI, Go CLI, and Web UI):

| Kind   | Stored in review.md                | Go equivalent                       |
| ------ | ---------------------------------- | ----------------------------------- |
| LINE   | `comments[]` entry, kind: line     | `comments[]`                        |
| RANGE  | `comments[]` entry, kind: range    | `comments[]`                        |
| FILE   | `comments[]` entry, kind: file     | `comments[]`                        |
| REVIEW | top-level `review_comment` scalar  | `Review.ReviewComment` (not appended to `comments[]`) |

The REVIEW kind overwrites the previous value (single top-level scalar, not a
list). This matches Go TUI's Shift+R in-place edit semantics
(`confirmModal` in modal.go sets `m.Review.ReviewComment = body`).

## Storage

The plugin reads and writes:

```
$SITATAME_HOME/<project-slug>/<branch-slug>/review.md
```

…which is exactly what the Go CLI and the Web UI PoC use (1-branch-1-file
layout introduced in issue #76). The slug algorithms are byte-for-byte ports
of `internal/review/slug.go` so multiple tools see the same file for the
same repo + branch.

An automatic backup (`review.md.bak`) is written before each save. If the
encoder fails, a rescue JSON (`review.md.rescue.<timestamp>.json`) is
written instead so no in-memory state is lost.

The Settings panel exposes a `SITATAME_HOME` override that takes precedence
over the environment variable; leave it blank to honour the shell.

## Known limitations

- Phase 1 does no concurrency control. If the CLI and the IDE both write to
  the same `review.md` simultaneously, last-write-wins. Phase 2 will add an
  inode-based mtime check and conflict prompt.
- The "copy AI prompt" body's `関連 diff (要約)` section is a placeholder;
  Phase 2 wires it up to `git diff --stat` via Git4Idea.
- Plugin Verifier is configured but only runs against the primary target
  (IntelliJ 2024.3). Android Studio support is unverified — try it and
  file an issue.

## Phase 2 backlog

- Inline gutter markers in the editor (`EditorMarkupModel`) instead of
  having to open the tool window to see comments.
- Compose Multiplatform UI for the comment list so the Web UI and IntelliJ
  plugin can share render code.
- JetBrains Marketplace submission with a screenshot set and changelog.
- Concurrent edit conflict prompt + retry.
- LSP-style integration so non-JetBrains IDEs can call the same store via
  a localhost transport.
