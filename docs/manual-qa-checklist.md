# Manual QA Checklist — sitatame Web UI Write Path (Phase 1 Step 2)

Run these checks after `cd web && ./gradlew :run` and opening the printed URL in a
browser.  A real git repository with `origin/main` reachable is required so the
diff is non-empty.

## Setup

1. `cd web && ./gradlew :run` — note the port printed on stdout.
2. Open `http://127.0.0.1:<port>` in a browser.
3. Verify: the diff view loads, files appear in the sidebar.

## A. Line Comment

- [ ] Tap a `+` line in any hunk → `CommentModal` opens.
- [ ] Enter comment text, click **Submit**.
- [ ] The comment appears inline under the tapped line without a page reload.
- [ ] The comment also appears in the sidebar comment list.
- [ ] Repeat for a `-` (deletion) line — comment should appear anchored to the base side.

## B. Range Comment (long-press)

- [ ] Long-press any diff line → range mode banner appears at top of file view with
  "Select end line…".
- [ ] Click a second line **in the same hunk** → `CommentModal` opens pre-filled
  with a range anchor.
- [ ] Submit comment → range comment appears inline spanning the selected lines.
- [ ] Long-press to start range mode, then click **Cancel** in the banner → banner
  disappears, no comment opened, line highlights cleared.
- [ ] Long-press, then click a line in a **different hunk** → banner shows a
  rejection message; range mode stays active.

## C. File Comment

- [ ] Click **"Add file comment"** in the file header → `CommentModal` opens.
- [ ] Submit → file-level comment appears under the file header.

## D. Overall Comment

- [ ] Click **"Add overall comment"** in the top bar → `CommentModal` opens.
- [ ] Submit → comment appears in the sidebar review section.

## E. Resolve Toggle (Optimistic UI)

- [ ] Find a comment with state "Open" in the sidebar.
- [ ] Click the toggle/resolve button → state changes to "Resolved" immediately
  (optimistic).
- [ ] Wait ~1 s — the state remains "Resolved" (server confirmed).
- [ ] Toggle again → state returns to "Open".

## F. 412 Conflict Flow

Reproduce a conflict by writing from two concurrent clients:

1. Open the Web UI in two browser tabs (both load the same workspace/ETag).
2. In **Tab 1**: add a comment (succeeds; Tab 1's ETag advances).
3. In **Tab 2** (still holding the old ETag): add a comment → should receive 412 →
   `ConflictModal` appears.
4. Click **"Reload latest and retry pending edit"** → modal closes, comment is
   retried with the new ETag, appears in the list.
5. Repeat steps 2-3, then click **"Discard pending edit"** → modal closes, no
   comment added.

Alternative (TUI + Web):

1. Open the Web UI and note the current ETag (visible in DevTools → Network → GET
   /workspace → Response Headers → ETag).
2. In the TUI, add a comment and save (`w`).
3. In the Web UI, try adding a comment without reloading → 412 → conflict modal.

## G. ETag Verification (DevTools)

- [ ] Open DevTools → Network tab.
- [ ] Load the page — inspect `GET /api/v1/workspace` response: `ETag` header should
  be `"empty"` (if no review.md) or `sha256-...`.
- [ ] After adding a comment: inspect `POST /api/v1/comments` response — `ETag`
  header should be a new `sha256-...` value different from the previous.

## H. Blob SHA in Comment Request

- [ ] Open DevTools → Network → `POST /api/v1/comments`.
- [ ] For a line comment on a `+` line: `blob` field in request body should be the
  abbreviated `blobHead` SHA (visible in `GET /workspace` → `files[i].blobHead`).
- [ ] For a line comment on a `-` line: `blob` field should be `blobBase`.
- [ ] For a `kind=review` or `kind=file` comment: `blob` is absent or null (not required).

## I. Stale Blob Detection

Simulate a stale blob by modifying the diff between page load and comment submit:

1. Load the Web UI.
2. In another terminal: `git commit --amend --no-edit` or make a new commit and
   rebase to change the HEAD diff.
3. In the browser (without reloading): attempt to add a line comment → server may
   return 422 with "stale anchor" message if the blob SHA changed.
   *(This is a best-effort check; if the diff base ref is unchanged the blob may
   still match.)*

## J. Regression — Read-Only View Still Works

- [ ] After all write operations, the diff view still renders correctly.
- [ ] Sidebar file list and comment list are accurate.
- [ ] No JavaScript console errors from Wasm.
