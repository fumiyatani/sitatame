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

## M. CopyAIPromptAction — Threading and Lifecycle (Issue #105)

These items verify the threading fix (P2-1 / P2-2 / P3-1) in the IntelliJ
plugin action. Requires running the plugin inside a sandboxed IDE instance
(`cd intellij && ./gradlew runIde`).

### M-1. snapshotComments runs off the EDT

- [ ] Set a breakpoint (or add a log statement) inside `CopyAIPromptAction.run`
  before `store.snapshotComments(...)`.
- [ ] Trigger the action → breakpoint / log should show the current thread is
  **not** the Event Dispatch Thread (thread name does not contain "AWT-EventQueue").

### M-2. buildPrompt runs off the EDT

- [ ] `buildPrompt(targets)` is called from the same background thread as
  `snapshotComments` — confirm the thread name at that call site is not
  "AWT-EventQueue".

### M-3. Clipboard write and dialog show on the EDT

- [ ] After `buildPrompt` completes, `CopyPasteManager.setContents` and
  `PromptPreviewDialog.show()` should run on the Event Dispatch Thread.
- [ ] Verify by adding a breakpoint inside the `invokeLater` lambda — thread name
  should contain "AWT-EventQueue".

### M-4. Disposed project does not open dialog

- [ ] With the action running: force-close the project while the background task
  is still in flight (e.g. simulate slow I/O with a sleep patch).
- [ ] The `PromptPreviewDialog` must **not** appear; `project.disposed` expiration
  prevents the `invokeLater` runnable from executing.

### M-5. Clipboard unchanged on failure

- [ ] Copy some sentinel text to the clipboard before triggering the action.
- [ ] Simulate a failure in `snapshotComments` (e.g. point the plugin at a repo
  with a corrupted review.md).
- [ ] After the error notification appears, paste from clipboard — the sentinel
  text should still be there (no partial prompt was written).

## N. Fat Jar Distribution (Issue #88)

### N-1. Build the fat jar

```sh
make web-jar
```

- [ ] `make web-jar` completes without error.
- [ ] `web/build/libs/sitatame-web-*-fat.jar` exists and is roughly 15–25 MB.

### N-2. Launch fat jar against a different repository

1. Copy (or note the path of) the fat jar, e.g.
   `JAR=/path/to/sitatame-web-0.2.0-fat.jar`.
2. Change to a directory that is **not** the sitatame repo root (e.g. `cd /tmp`).
3. Run:
   ```sh
   java -jar "$JAR" --repo /path/to/other-repo
   ```
4. - [ ] `SITATAME_WEB_URL=http://127.0.0.1:<port>` is printed on stdout.
5. - [ ] Open the URL — diff view shows the *other* repo's changes, not sitatame's.
6. - [ ] `GET /api/v1/workspace` returns a `projectSlug` matching the other repo.
7. - [ ] Ctrl-C stops the server cleanly (no exception stack trace).

### N-3. `--base` flag works with fat jar

```sh
java -jar "$JAR" --repo /path/to/repo --base origin/develop
```

- [ ] Diff is relative to `origin/develop`, not `origin/main`.

### N-4. `--help` prints usage and exits

```sh
java -jar "$JAR" --help
```

- [ ] Usage text is printed and the process exits 0 (no server is started, no
  port is bound).

### N-5. Invalid `--repo` rejected at startup

```sh
java -jar "$JAR" --repo /no/such/path
```

- [ ] Server prints a clear error to stderr and exits without binding a port.

### N-6. SPI descriptors are present in the fat jar

```sh
JAR=web/build/libs/sitatame-web-*-fat.jar
unzip -p "$JAR" META-INF/services/org.slf4j.spi.SLF4JServiceProvider
```

- [ ] The `org.slf4j.spi.SLF4JServiceProvider` descriptor exists in the fat jar
  and contains exactly one provider line (`org.slf4j.simple.SimpleServiceProvider`).
- [ ] No `SLF4J: No SLF4J providers were found` warning is printed when launching
  the fat jar (`java -jar "$JAR" --help`).

## O. IntelliJ Editor Inlay — Block Comment Display (#117)

Prerequisites: build and install the plugin (`./gradlew buildPlugin`), open a project that has at least one `review.md` with LINE or RANGE comments.

### O-1. Basic inlay appearance

1. Open a file that has a sitatame comment anchored to a specific line.
2. - [ ] A card appears directly below the anchor line in the editor (not in the gutter).
3. - [ ] The card shows: a coloured dot (blue = open, green = resolved), the first line of the comment body, and a "Resolve" or "Reopen" button depending on state.
4. - [ ] Multiple comments on the same line appear as stacked rows in a single card.

### O-2. Resolve / Reopen button

1. Click the **Resolve** button on an open comment.
2. - [ ] The dot changes from blue to green and the button label changes to "Reopen".
3. - [ ] The underlying `review.md` is updated (check with `cat ~/.sitatame/<project>/<branch>/review.md`).
4. Click **Reopen** on a resolved comment.
5. - [ ] State toggles back to open (blue dot, "Resolve" label).

### O-3. Collapse defaults

1. Open a file with a resolved comment and an open comment.
2. - [ ] The resolved comment row is visually compact (collapsed) — only its first line is shown.
3. - [ ] The open comment is expanded.

### O-4. No inlay for non-line anchors

1. Add a file-level comment (`kind: file`) or review-level comment (`kind: review`) via CLI.
2. - [ ] No inlay appears in the editor for such comments (they have no line anchor).

### O-5. Editor with no comments

1. Open any file that has no sitatame comments.
2. - [ ] No inlays appear; no exceptions logged in idea.log.

### O-6. Line out of range

1. Add a comment with `line: 9999` to a short file, then open that file.
2. - [ ] No crash; idea.log shows a `sitatame inlay: line 9999 out of range` debug message.

### O-7. Multiple files

1. Open two tabs with comments on different files.
2. - [ ] Each tab shows only the inlays for its own file, not the other's.

## P. IntelliJ 3-pane Tool Window + Base Ref Selector (#136)

Prerequisites: build the plugin (`cd intellij && ./gradlew buildPlugin`), install from disk,
open a project that has a `~/.sitatame/<project>/<branch>/review.md` with at least a few
comments, and confirm `origin/main` is reachable.

### P-1. Three panes visible

1. Open the **SitatameReview** tool window.
2. - [ ] Three panes are visible: Changed files (left), Comments (middle), Comment detail (right).
3. - [ ] A toolbar appears above with Refresh icon, "Base:" label + ComboBox, "Set as default" button, and Copy AI prompt action.

### P-2. Changed files pane (left)

1. - [ ] Files changed since `origin/main` (or current base ref) are listed.
2. - [ ] An "All files" entry appears at the top of the list.
3. - [ ] Files that have comments show a count suffix `[open/total]` in bold.
4. - [ ] Files without comments appear in a dimmed colour.
5. Double-click any file entry.
6. - [ ] The corresponding file opens in the editor.

### P-3. File selection drives comment list (middle)

1. Click a file that has comments in the left pane.
2. - [ ] The middle pane shows only comments for that file.
3. Click "All files".
4. - [ ] The middle pane shows all comments for the branch.

### P-4. State filter in middle pane

1. With "All files" selected, click **Opened** radio.
2. - [ ] Only open comments are shown.
3. Click **Resolved**.
4. - [ ] Only resolved comments are shown.
5. Click **All**.
6. - [ ] All comments shown again.

### P-5. Comment detail pane (right)

1. Click a comment row in the middle pane.
2. - [ ] The right pane shows the anchor location, state, and body text.
3. - [ ] "Resolve" button is enabled for an open comment; "Reopen" for resolved.
4. - [ ] "Delete" button is enabled.
5. - [ ] "Reply" button is present but greyed out with tooltip "Coming soon".

### P-6. Resolve / Reopen from detail pane

1. Click **Resolve** in the right pane for an open comment.
2. - [ ] Button label changes to "Reopen"; middle pane row icon updates.
3. Click **Reopen**.
4. - [ ] State reverts to open.

### P-7. Delete from detail pane

1. Click **Delete** in the right pane.
2. - [ ] Confirmation dialog appears.
3. Confirm deletion.
4. - [ ] Comment disappears from middle pane; right pane clears.

### P-8. Base ref selector — session override

1. Open the "Base:" ComboBox in the toolbar.
2. - [ ] Candidates include: "(default — Settings)" entry if Settings has a baseRef set, `origin/main`, `origin/master`, `origin/develop`, and local branches.
3. Select a different base ref.
4. - [ ] Left pane refreshes and shows files changed since the new base ref.
5. Close and reopen the tool window.
6. - [ ] Base ref resets to the Settings default (session override is not persisted).

### P-9. "Set as default" persists base ref

1. Select a non-default ref in the ComboBox.
2. Click **Set as default**.
3. Open **Settings → Tools → sitatame review**.
4. - [ ] The "Base ref" field shows the selected ref.
5. Close and reopen the tool window.
6. - [ ] The ComboBox shows the saved ref (and the "(default — Settings)" annotated entry).

### P-10. Pane width persistence

1. Drag the splitter between the left and middle panes to a custom width.
2. Close the tool window.
3. Reopen the tool window.
4. - [ ] Pane widths are restored to the dragged position (PropertiesComponent persistence).

### P-11. MessageBus auto-refresh

1. Use the editor shortcut (Cmd+Shift+C / Ctrl+Shift+C) to add a new comment.
2. - [ ] The middle pane updates automatically without pressing Refresh.
3. - [ ] The left pane count for the affected file updates.

### P-12. Key bindings on middle pane

1. Select a comment row in the middle pane, press **Enter**.
2. - [ ] Editor jumps to the anchored file:line.
3. Press **Space**.
4. - [ ] Comment state toggles (open → resolved or vice versa).

