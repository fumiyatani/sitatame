# Per-repository config: `.sitatame/config.yaml`

`sitatame` can be configured per-repository via a YAML file at
`<repo>/.sitatame/config.yaml`. The file is optional — when it is absent,
`sitatame` falls back to its built-in defaults.

This page documents the Phase 1 schema (issue #24). Sections marked
*reserved* are accepted (and warned about on stderr) but otherwise ignored;
they exist so the file format can be extended in later releases without
breaking older binaries.

## Location and discovery

- Path: `<repo-root>/.sitatame/config.yaml`
- Discovery: `sitatame` looks at the working directory of the current repo
  (the root reported by `git rev-parse --show-toplevel`). Worktrees of the
  same repository each get their own config file because each worktree has
  its own root.
- The directory `<repo>/.sitatame/` was historically the on-disk location
  for review drafts and is now flagged as legacy on startup. The legacy
  warning is automatically suppressed when the directory contains nothing
  except `config.yaml`, so adding the new config file does not regress the
  startup output.

## Current schema

```yaml
base:
  default: "origin/develop"     # optional; tried first when no CLI base is given
  candidates:                    # optional; replaces the built-in fallback chain
    - "origin/develop"
    - "origin/main"
    - "main"

# Reserved for future use — parsed and ignored with a warning today.
# display: ...
# keybinds: ...
```

### `base.default` (string, optional)

The ref `sitatame` should try first when the user does not pass an explicit
base on the command line. Equivalent to placing one entry at the front of
the candidate chain.

### `base.candidates` (list of strings, optional)

Replaces the built-in `BaseCandidates` chain
(`origin/HEAD`, `@{upstream}`, `origin/main`, `origin/master`, `main`,
`master`) for the auto-detect path. The entries are tried in order; the
first one that resolves to a commit *and* differs from `HEAD` wins.

**When `base.candidates` is specified, the built-in fallback chain is NOT
appended.** A repo that pins `candidates: [origin/develop]` will fail with
"base not found" rather than silently falling back to `origin/main` /
`main`. This is deliberate: every review is anchored against the resolved
base, so a silently mismatched base would produce a misleading review.

When `base.candidates` is omitted but `base.default` is set, `sitatame`
keeps the built-in fallback after the configured default so common
workflows (`origin/main`, `main`, …) continue to work without further
configuration.

Writing `candidates: []` explicitly is **not** the same as omitting the
key. The empty list is treated as "I want auto-detect restricted to
`base.default` only — do not fall back to the built-in chain." This is the
idiomatic way to pin auto-detect to a single ref:

```yaml
base:
  default: "origin/release"
  candidates: []   # auto-detect uses only origin/release; no built-in fallback
```

When both `base.default` and `base.candidates` are omitted (or
`base.candidates: []` is written with no `base.default`), the built-in
chain is still used as a safety net so the TUI can launch.

The effective auto-detect order is:

| `base.default` | `base.candidates` | Effective chain                                |
| -------------- | ----------------- | ---------------------------------------------- |
| unset          | unset             | built-in `BaseCandidates`                      |
| set            | unset             | `[default, ...built-in BaseCandidates]`        |
| unset          | `[]`              | built-in `BaseCandidates` (safety net)         |
| set            | `[]`              | `[default]` only (built-in NOT appended)       |
| unset          | non-empty         | `candidates` (built-in NOT appended)           |
| set            | non-empty         | `[default, ...candidates]` (built-in NOT appended) |

Duplicates between `default` and the chain that follows are collapsed so
the auto-detect failure message stays readable.

## Priority order with the CLI

For the default (range-diff) mode, base resolution follows this priority:

1. An explicit positional argument to `sitatame` (e.g. `sitatame
   origin/develop`).
2. `base.default` from `.sitatame/config.yaml`.
3. `base.candidates` from `.sitatame/config.yaml` *if set*, otherwise the
   built-in `BaseCandidates` fallback chain.

Step 3 is exclusive: when `base.candidates` is set, the built-in fallback
chain is not consulted. See the table in `base.candidates` above for the
full matrix.

`--staged` and `--working` ignore both the CLI base argument and the config
file because their diff is always against `HEAD` by definition.

## Environment variables

`SITATAME_HOME` (output root for reviews and drafts) is unrelated to this
file and is **not** overridable from `.sitatame/config.yaml`. Environment
variables always win over config; config wins over built-in defaults.

## Error handling

`sitatame` is intentionally permissive about config errors so a broken file
never blocks the TUI from launching:

- **File missing**: silently treated as an empty config; built-in defaults
  apply.
- **File present but unparseable** (malformed YAML): one stderr warning,
  config is dropped, and `sitatame` proceeds with built-in defaults.
- **Unknown top-level key** (e.g. a typo, or an old binary seeing a new
  section): one stderr warning, that key is ignored, the rest of the file
  is honored.
- **Reserved section** (`display`, `keybinds`): one stderr warning that the
  section is reserved, contents ignored.
- **Field with the wrong type** (e.g. `base.candidates: "main"` instead of
  a list): one stderr warning, the offending field is dropped, other
  fields in the same section are still applied.
- **YAML scalar typed as non-string** (e.g. `base.default: true`,
  `base.default: 123`, `base.default: null`): one stderr warning naming
  the offending YAML type, that single value is dropped, parsing
  continues. Quote the value (`base.default: "true"`) to force string
  interpretation. For `base.candidates`, the rule is per-entry: bad
  entries are skipped individually so a single typo does not discard the
  whole chain.

Warnings are prefixed with `sitatame: config:` so they are easy to grep for
when wiring `sitatame` into agent pipelines.

## Reserved for future releases

The following keys are accepted today only to reserve their names; they
have no behavioral effect and emit a warning when present.

- `display` — placeholder for theme / layout / width tuning.
- `keybinds` — placeholder for keymap customization. (Issue #29 is
  currently deferred; the schema for this section will be designed
  alongside that work.)

If you are configuring `sitatame` for a team today, stick to the `base`
section. Anything else may change shape before it ships.
