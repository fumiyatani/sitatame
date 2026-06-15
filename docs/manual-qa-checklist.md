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

## K. Thread Grouping and State Filter (Issue #18)

- [ ] Select a file with multiple comments on the same anchor (same path + line) —
  they appear in a single collapsible thread, not as separate rows.
- [ ] State filter control (`All / Open / Done / Stale`) appears at the top of the
  sidebar under "Files (N)".
- [ ] Selecting "Open" hides resolved/stale threads for the selected file.
- [ ] Selecting "Done" shows only resolved threads.
- [ ] Selecting "Stale" shows only stale threads.
- [ ] Selecting "All" restores the full thread list.
- [ ] Open/Stale threads start expanded; Resolved threads start collapsed.
- [ ] Clicking a thread header toggles collapsed/expanded state.
- [ ] Collapsed thread shows: `▶ filename:line [state] (N comments)` in one row.
- [ ] Expanded thread shows each comment with its Resolve/Reopen button.
- [ ] "Reply to this thread" button appears at the bottom of an expanded thread.
- [ ] Clicking "Reply to this thread" opens `CommentModal` with the header "Reply to: &lt;anchor&gt;"
  (e.g. "Reply to: Line 42 · foo.kt").
- [ ] The modal text area starts empty and focused; typing and clicking Submit adds a new
  comment on the same anchor (visible in the thread without a page reload).
- [ ] Clicking Cancel in the reply modal closes it without posting.
- [ ] After submitting a reply, the modal closes and the thread shows the new comment.
- [ ] Narrowing the browser window below 320 dp causes the filter to switch to a
  dropdown fallback (may require a narrow window or DevTools device emulation).
- [ ] Open thread background is default dark; Resolved thread has dim grey background;
  Stale thread has dark amber background.
- [ ] The open-thread count badge on each file row counts open threads regardless of
  the active filter setting.

## M. IntelliJ Plugin — Comment List UX (Issue #95)

Install the plugin from `intellij/build/distributions/sitatame-intellij-0.2.0.zip`
via **Settings → Plugins → Install Plugin from Disk**, then open a project that has
a sitatame `review.md` in `~/.sitatame/<project>/<branch>/`.

### M-1. State Filter (All / Opened / Resolved)

- [ ] Open the "SitatameReview" tool window.
- [ ] Filter bar below the toolbar shows three radio buttons: **All**, **Opened**,
  **Resolved**. Default is **All**.
- [ ] With mixed open/resolved comments loaded, select **Opened** → only open
  comments are shown.
- [ ] Select **Resolved** → only resolved comments shown.
- [ ] Select **All** → all comments shown again.

### M-2. State Icon Shape and Colour

- [ ] Each open comment row shows a **green filled circle (●)** icon.
- [ ] Each resolved comment row shows a **purple check mark (✓)** icon.
- [ ] Each stale comment shows a **yellow warning triangle** (platform icon).
- [ ] Open/Resolved are distinguishable in both light and dark IDE themes.
- [ ] Open/Resolved are distinguishable by shape alone (colour-blind check).

### M-3. Popup Label: Mark Resolved / Reopen

- [ ] Right-click an **open** comment → context menu shows **"Mark Resolved"**
  (not "Toggle Resolved").
- [ ] Right-click a **resolved** comment → context menu shows **"Reopen"**.
- [ ] Clicking "Mark Resolved" on an open comment sets its state to resolved
  and the list refreshes.
- [ ] Clicking "Reopen" on a resolved comment sets its state to open and the
  list refreshes.

### M-4. Delete with Confirmation

- [ ] Each comment row shows a trash-can icon on the right edge.
- [ ] Clicking the trash icon selects the row and opens a confirmation dialog.
- [ ] Clicking **Cancel** in the dialog → comment is not deleted.
- [ ] Clicking **Delete** → comment is removed from the list and from
  `review.md` on disk.
- [ ] Right-click any comment → context menu also contains **"Delete"** item
  which triggers the same confirmation flow.

### M-5. Auto Refresh via MessageBus

- [ ] Add a comment from the editor (Cmd+Shift+C or right-click → "sitatame:
  Add Comment") while the tool window is open → the new comment appears in the
  list **without** clicking the Refresh button.
- [ ] Toggle resolved from the editor (Cmd+Shift+R) while the tool window is
  open → the state icon in the list updates automatically.
- [ ] If the project's repo/branch does not match the changed review, the tool
  window does **not** refresh (multi-project isolation — verify by having two
  projects open simultaneously if possible).

### M-6. Enter / Space Key Bindings (Issue #94)

- [ ] Select any comment row with the keyboard (arrow keys).
- [ ] Press **Enter** → the editor jumps to the file and line anchored by the
  comment (same behaviour as double-clicking the row). The resolved/open state
  does **not** change.
- [ ] Press **Space** → the resolved/open state of the selected comment toggles
  (open → resolved or resolved → open) and the list refreshes. The editor does
  **not** navigate.
- [ ] In **Settings → Keymap** search for "sitatame: Toggle Resolved (Tool
  Window)" — the action appears with no default shortcut.
- [ ] Assign a custom shortcut (e.g. **Space**) via Keymap settings → confirm
  that the action fires when the tool window list is focused and a comment is
  selected.
- [ ] With **no** comment selected, verify the Keymap action is disabled
  (greyed out in the menu).
- [ ] Note: the list uses SINGLE_SELECTION mode; multi-select is not supported.
  When a Keymap action fires, only the single selected comment is affected.

## J. Regression — Read-Only View Still Works

- [ ] After all write operations, the diff view still renders correctly.
- [ ] Sidebar file list and comment list are accurate.
- [ ] No JavaScript console errors from Wasm.

## L. Other Repository (`--repo`) and Base Ref (`--base`) (#88)

### L-1. `--repo` targeting a different repository

1. Clone a second git repository to a temp directory (it must have `origin/main`
   reachable or use `--base` with a local ref).
2. Start the server pointing at it:
   ```sh
   cd web && ./gradlew :run --args="--repo /path/to/other-repo"
   ```
3. - [ ] `SITATAME_WEB_URL=http://127.0.0.1:<port>` is printed (server started).
4. - [ ] Open the URL — the sidebar shows files from the *other* repo's diff, not
     from the sitatame repo.
5. - [ ] The `projectSlug` in the response (`GET /api/v1/workspace`) reflects the
     other repo's directory name, not "sitatame".

### L-2. `--base` overrides the default `origin/main`

1. In a repo that has a branch named `origin/develop` (or create a local ref):
   ```sh
   cd web && ./gradlew :run --args="--repo /path/to/repo --base origin/develop"
   ```
2. - [ ] The diff shown is relative to `origin/develop`, not `origin/main`.

### L-3. `SITATAME_REPO` / `SITATAME_BASE` environment variables

```sh
SITATAME_REPO=/path/to/other-repo SITATAME_BASE=origin/develop ./gradlew :web:run
```

- [ ] Same result as `--repo` / `--base` flags respectively.
- [ ] CLI flag takes precedence over env when both are set (test by setting
  `SITATAME_REPO=<wrong-path>` and passing `--repo <correct-path>` — the correct
  path must be used).

### L-4. Invalid `--repo` path rejected at startup

```sh
cd web && ./gradlew :run --args="--repo /no/such/path"
```

- [ ] Server prints a clear error to stderr and exits without binding a port.
- [ ] Error message contains "does not exist" or "not a directory".

### L-5. Path without `.git` rejected at startup

```sh
cd web && ./gradlew :run --args="--repo /tmp"
```

- [ ] Server prints an error mentioning `.git` and exits without binding a port.
