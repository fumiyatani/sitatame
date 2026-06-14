# Web UI Phase 1 Step 2: Write Path Specification

> **Scope**: comment add, resolve toggle, review-level comment save, and ETag-based
> conflict detection for the Compose for Web UI.  Read-only diff viewing (Phase 1
> step 1) is a prerequisite and is not described here.

## 1. Overview

Phase 1 step 2 adds write capabilities to the sitatame Web UI so reviewers can
create and manage comments from the browser instead of, or in addition to, the TUI
or IntelliJ Plugin.  All three surfaces share the same `review.md` file on disk;
concurrent edits are detected via ETag (see §4).

## 2. API Endpoints

All write endpoints live under `/api/v1/` and require:
- `Content-Type: application/json`
- `If-Match: <etag>` header — the ETag the client obtained from the most recent
  `GET /api/v1/workspace` or from the `ETag` header of the last successful write.

### POST /api/v1/comments

Add a new comment.

**Request body** (`CreateCommentRequest`):

| Field       | Type     | Required | Description |
|-------------|----------|----------|-------------|
| `kind`      | string   | yes      | `"line"` \| `"range"` \| `"file"` \| `"review"` |
| `path`      | string?  | kind≠review | File path relative to repo root |
| `side`      | string   | kind=line/range | `"head"` (default) \| `"base"` |
| `blob`      | string?  | no       | Git blob SHA from `FileDto.blobHead/blobBase`; enables stale detection |
| `line`      | int?     | kind=line | Line number on the selected side |
| `lineStart` | int?     | kind=range | First line of the range |
| `lineEnd`   | int?     | kind=range | Last line of the range (≥ lineStart) |
| `body`      | string   | yes      | Comment text; must not be blank |

**Responses**:

| Status | Meaning |
|--------|---------|
| 200    | Comment created. Body: `{"anchor_id": "<uuid>"}`. `ETag` header carries the new review.md hash. |
| 400    | `If-Match` header absent. |
| 412    | ETag mismatch. Body: `EtagMismatchError`. `ETag` header carries the current server hash. |
| 422    | Validation error. Body: `{"errors": ["..."]}`. |

### PATCH /api/v1/comments/{anchorId}/state

Toggle or set a comment's state.

**Request body** (`UpdateCommentStateRequest`):

| Field   | Type   | Values |
|---------|--------|--------|
| `state` | string | `"open"` \| `"resolved"` \| `"stale"` |

**Responses**: same status codes as POST.  On 200 the body is `{"anchor_id": "<id>"}`.

### PUT /api/v1/review-comment

Replace the review-level comment (the top-level narrative).

**Request body** (`UpdateReviewCommentRequest`):

| Field  | Type   | Description |
|--------|--------|-------------|
| `text` | string | New review comment. Empty string clears the field. |

**Responses**: 200 `{"ok": true}` on success; 404 when `review.md` does not exist yet.

### GET /api/v1/workspace

Read-only.  Returns `WorkspaceResponse` with `FileDto` entries that now include
`blobBase` and `blobHead` fields (abbreviated git blob SHAs from the diff `index`
header).  The `ETag` response header carries the current hash of `review.md`.

## 3. ETag Protocol

The ETag identifies the exact byte contents of `review.md` on disk.

- **Format**: `sha256-<hex>` where hex is the SHA-256 of the file bytes.
- **Empty sentinel**: `"empty"` when `review.md` does not exist yet.
- **Transport**: HTTP `ETag` response header (GET workspace, 200 write responses,
  412 write responses).  The client sends the last-seen value in `If-Match`.
- **Quote handling**: the server accepts ETags with or without surrounding
  double-quotes in `If-Match`.
- **Concurrency**: a per-path `kotlinx.coroutines.sync.Mutex` serialises all writes
  to the same `review.md`.  The ETag check, mutation, and file write happen inside
  the critical section so no two concurrent clients can both pass the check with the
  same ETag.

## 4. UX Specification

### 4.1 Line Comment

1. Tap any diff line → `CommentModal` opens.
2. Enter text, click **Submit** → `POST /api/v1/comments` with `kind=line`.
3. On 200: comment appears inline; `ETag` updated.
4. On 412: `ConflictModal` (see §4.6).

### 4.2 Range Comment

Compose for Web (CMP 1.7.x) does not expose `isShiftPressed` through
`detectTapGestures`, so the canonical interaction is:

**Long-press** any line → range mode starts; a sticky banner appears at the top of
the file view showing the start line.  Click a second line in the **same hunk** to
confirm the range → `CommentModal` opens.

- **Cancel**: click the "Cancel" button in the banner; range state and highlighting
  are discarded.
- **Cross-hunk attempt**: clicking an end line in a different hunk is rejected; the
  banner shows "Select end line in the same hunk".
- **"Add range comment" button** (top bar): also enters range mode from the top bar,
  useful for keyboard-only or pointer-challenged flows.

> **Future**: when CMP adds `isShiftPressed` support, `Shift+click` will start and
> confirm range mode as an alternative to long-press.  The long-press path will be
> retained for touch devices.

### 4.3 File Comment

Click the **"Add file comment"** button in the file header row.  Opens
`CommentModal` with `kind=file` pre-filled.

### 4.4 Overall Comment (Review-Level)

Click **"Add overall comment"** in the top bar.  Opens `CommentModal` with
`kind=review` pre-filled.  The body is submitted to `POST /api/v1/comments` (not
to `PUT /api/v1/review-comment`, which only updates the review narrative summary).

### 4.5 Resolve Toggle

Each comment card in the sidebar shows a toggle button.  Clicking it:

1. Applies an **optimistic update** immediately (UI changes without a round-trip).
2. Sends `PATCH /api/v1/comments/{anchorId}/state`.
3. On 200: confirms the optimistic state; ETag updated.
4. On failure: rolls back the optimistic state and shows a toast error.
5. On 412: rolls back and shows `ConflictModal`.

While the PATCH is in flight the button is disabled to prevent double-submit.

### 4.6 Conflict Modal (412 Handling)

When any mutating request returns 412 the `ConflictModal` is shown with two
choices:

| Button | Action |
|--------|--------|
| **Reload latest and retry pending edit** | `GET /workspace` → update ETag → re-send the pending mutation with the new ETag |
| **Discard pending edit** | Clear the pending mutation; close the modal |

Force-overwrite ("retry as-is") is intentionally not offered in Phase 1 step 2.
It will be added in a future PR with an explicit confirmation.

## 5. Blob SHA and Deletion-Line Anchors

Each `FileDto` carries `blobBase` and `blobHead` fields (abbreviated git blob SHAs
from the `index <base>..<head>` line in the unified diff).  The Frontend populates
`CreateCommentRequest.blob` as follows:

| Anchor side | Blob field used |
|-------------|-----------------|
| `"base"` (deletion line `-`) | `FileDto.blobBase` |
| `"head"` (addition or context line) | `FileDto.blobHead` |

**Deletion lines** (`-` prefix) are anchored to `side: "base"` following the same
rule as the Go TUI (`internal/tui/modal.go: lineSideForRow`).

**Blob integrity check**: if the client supplies `blob` and the server has workspace
file data available at `POST /comments` time, the server verifies the SHA is a
prefix-match of the current diff's blob for the same path/side.  A mismatch returns
422 with a "stale anchor" message, indicating the diff changed since the Frontend
loaded the workspace.

## 6. Concurrent Editing

`review.md` is a shared file.  All three sitatame surfaces (TUI, Web UI, IntelliJ
Plugin) read and write it directly:

- **Web UI**: honours ETag.  Every write checks `If-Match`; conflict → 412 →
  `ConflictModal`.
- **TUI / IntelliJ Plugin**: use last-write-wins semantics (no ETag).  A TUI save
  after the Web UI loaded will not be detected until the next Web UI write, which
  will receive a 412.

There is no WebSocket or SSE push for concurrent-edit notification in Phase 1
step 2.  Manual browser refresh reflects the latest state from any other client.

## 7. Comment UX Improvements (Issue #18)

### Thread grouping

Comments sharing the same anchor (`kind + path + side + line/lineStart/lineEnd`)
are grouped into a single _thread_. Thread state is derived from its comments:
`Open` if any comment is open, `Stale` if none are open but at least one is
stale, `Resolved` otherwise.

### State filter

A segmented control at the top of the sidebar (`All / Open / Done / Stale`)
filters the thread list for the selected file. Badge counts reflect open threads
independent of the active filter so the counts do not jump when switching filters.
On sidebar widths below 320 dp the control falls back to a dropdown menu.

### Collapsible threads

Each thread can be toggled open/closed by clicking its header row.  The chevron
`▶/▼` and the thread header (path:line) and state badge are always visible.
Default state:

| Thread state | Default |
|---|---|
| `Open` | Expanded |
| `Stale` | Expanded |
| `Resolved` | Collapsed |

### Reply flow

An expanded thread shows a "Reply to this thread" button below the last comment.
Clicking it opens an inline text field.  On submit, `CommentDto.toCommentTarget()`
derives the same anchor as the existing comment, and the reply is posted via the
same `POST /api/v1/comments` path.  A stale anchor may return 422 — the user is
notified via the toast/conflict flow already in place.

### Visual distinction

| Thread state | Background | Badge colour |
|---|---|---|
| `Open` | default canvas (`0xFF0D1117`) | orange-red (`openBadge`) |
| `Resolved` | dim grey (`0xFF1C2128`) | green (`resolvedBadge`) |
| `Stale` | dark amber (`0xFF2D2208`) | yellow (`staleBadge`) |

Resolved-thread body text is rendered in `Color.Gray` to visually de-emphasise.

## 8. Scope Exclusions (Phase 1 Step 2)

| Feature | Planned for |
|---------|-------------|
| DELETE comment endpoint | Future PR (needs confirmation UI + `.bak` n-generation recovery) |
| Force overwrite (Retry as-is) on conflict | Future PR (explicit `force` flag + dedicated confirm) |
| WebSocket / SSE push for concurrent edits | Future PR |
| Shift+click range mode in browser | Future PR (pending CMP isShiftPressed support) |
| Per-repo configurable base ref | Future PR (#67 Phase 1 step 3) |
