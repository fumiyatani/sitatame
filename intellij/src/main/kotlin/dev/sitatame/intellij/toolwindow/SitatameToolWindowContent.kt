package dev.sitatame.intellij.toolwindow

import com.intellij.icons.AllIcons
import com.intellij.openapi.Disposable
import com.intellij.openapi.actionSystem.ActionManager
import com.intellij.openapi.actionSystem.ActionPlaces
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.DataProvider
import com.intellij.openapi.actionSystem.DefaultActionGroup
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.fileEditor.OpenFileDescriptor
import com.intellij.openapi.ide.CopyPasteManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.ComboBox
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.ui.OnePixelSplitter
import com.intellij.ui.components.JBLabel
import dev.sitatame.intellij.actions.CopyAIPromptAction
import dev.sitatame.intellij.actions.ToolWindowToggleResolvedAction
import dev.sitatame.intellij.git.BaseRefDiscovery
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.settings.SitatameSettings
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.REVIEW_CHANGED_TOPIC
import dev.sitatame.intellij.storage.ReviewChangedListener
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore
import dev.sitatame.intellij.toolwindow.panes.ChangedFilesPane
import dev.sitatame.intellij.toolwindow.panes.CommentDetailPane
import dev.sitatame.intellij.toolwindow.panes.CommentListPane
import dev.sitatame.intellij.toolwindow.panes.FileSelection
import java.awt.BorderLayout
import java.awt.Dimension
import java.awt.FlowLayout
import java.awt.event.ItemEvent
import java.awt.event.KeyAdapter
import java.awt.event.KeyEvent
import javax.swing.DefaultComboBoxModel
import javax.swing.JButton
import javax.swing.JComponent
import javax.swing.JPanel

/** Filter state for the comment list. */
enum class FilterState { ALL, OPENED, RESOLVED }

/**
 * Three-pane tool window content:
 *
 *   Left:   Changed files list (git diff --name-only base..HEAD) with per-file comment counts.
 *   Middle: Comments for the selected file (or all), with state filter toolbar.
 *   Right:  Comment detail — body, state, Resolve/Reopen/Delete actions.
 *
 * Toolbar above all three panes hosts: Refresh, base ref selector, "Set as default", Copy AI prompt.
 *
 * Pane widths are user-adjustable via OnePixelSplitter. Width ratios are persisted in
 * PropertiesComponent (via the splitter's own persistence key).
 *
 * @param disposable Lifecycle scope for the MessageBus subscription.
 */
class SitatameToolWindowContent(
    private val project: Project,
    disposable: Disposable,
) {

    private val log = Logger.getInstance(SitatameToolWindowContent::class.java)

    /** Session-level base ref override. Null means "use Settings / auto-detect". */
    private var sessionBaseRef: String? = null

    /**
     * Guards [baseRefCombo] against re-entrant events while the model/selection
     * is rebuilt programmatically. Without this, assigning the model and
     * selected item in [rebuildBaseRefCombo] re-fires the item listener, which
     * calls [refresh] again, which rebuilds the combo again — an unbounded
     * recursion on the EDT that freezes the IDE.
     */
    private var suppressBaseRefEvents = false

    /**
     * Monotonic refresh id, bumped on the EDT at the start of each [refresh].
     * Each async job captures its id and discards its result if a newer refresh
     * has started, so a slow job can't overwrite a newer base selection / diff.
     * EDT-confined — only read and written on the EDT.
     */
    private var refreshGeneration = 0L

    private val changedFilesPane = ChangedFilesPane(
        project = project,
        onFileSelected = { sel -> onFileSelected(sel) },
    )

    private val commentListPane = CommentListPane(
        project = project,
        onCommentSelected = { comment -> commentDetailPane.showComment(comment) },
        onJumpRequested = { comment -> jumpToComment(comment) },
        onRefreshAll = { refresh() },
    )

    private val commentDetailPane = CommentDetailPane(
        project = project,
        onRefreshAll = { refresh() },
    )

    /**
     * Base ref selector. The base ref is the *left* side of
     * `git diff <base>..HEAD`: the branch the current branch (HEAD) is compared
     * against — like the "base" in GitHub's "base ← compare" PR view. The model
     * (grouped remote/local branches) is rebuilt on each refresh.
     */
    private val baseRefCombo = ComboBox<BaseRefDiscovery.Entry>().apply {
        preferredSize = Dimension(240, preferredSize.height)
        isEditable = false
        renderer = BaseRefComboRenderer()
    }

    /** Read-only label showing the current branch (the compare side of the diff). */
    private val currentBranchLabel = JBLabel()

    /** Last selected branch, used to revert an accidental header-row selection. */
    private var lastSelectedBranch: BaseRefDiscovery.Entry.Branch? = null

    val component: JComponent = build()

    init {
        // Subscribe to review mutations from other parts of the plugin.
        ApplicationManager.getApplication().messageBus
            .connect(disposable)
            .subscribe(
                REVIEW_CHANGED_TOPIC,
                ReviewChangedListener { changedRoot, changedBranch ->
                    ApplicationManager.getApplication().invokeLater {
                        val repo = RepoContext.forProject(project) ?: return@invokeLater
                        if (repo.repoRoot == changedRoot && repo.branch == changedBranch) {
                            refresh()
                        }
                    }
                },
            )
    }

    private fun build(): JComponent {
        val outer = object : JPanel(BorderLayout()), DataProvider {
            override fun getData(dataId: String): Any? {
                if (CopyAIPromptAction.SELECTED_COMMENTS_KEY.`is`(dataId)) {
                    return commentListPane.list.selectedValuesList.toList()
                }
                if (ToolWindowToggleResolvedAction.TOGGLE_SELECTED_KEY.`is`(dataId)) {
                    return if (commentListPane.list.selectedValue != null) {
                        Runnable { commentListPane.toggleSelected() }
                    } else null
                }
                return null
            }
        }

        outer.add(buildTopBar(), BorderLayout.NORTH)
        outer.add(buildThreePaneSplitter(), BorderLayout.CENTER)
        outer.add(buildFooter(), BorderLayout.SOUTH)

        // Key bindings on the comment list (Enter = jump, Space = toggle).
        commentListPane.list.addKeyListener(object : KeyAdapter() {
            override fun keyPressed(e: KeyEvent) {
                keyCodeToAction(e.keyCode)?.let { action ->
                    e.consume()
                    when (action) {
                        KeyAction.JUMP -> commentListPane.list.selectedValue?.let { jumpToComment(it) }
                        KeyAction.TOGGLE -> commentListPane.toggleSelected()
                    }
                }
            }
        })

        refresh()
        return outer
    }

    private fun buildThreePaneSplitter(): JComponent {
        // Left | Right composite
        val rightSplitter = OnePixelSplitter(false, 0.5f, 0.2f, 0.8f).apply {
            firstComponent = commentListPane.component
            secondComponent = commentDetailPane.component
        }
        rightSplitter.splitterProportionKey = "sitatame.3pane.right.split"

        val leftSplitter = OnePixelSplitter(false, 0.28f, 0.1f, 0.7f).apply {
            firstComponent = changedFilesPane.component
            secondComponent = rightSplitter
        }
        leftSplitter.splitterProportionKey = "sitatame.3pane.left.split"

        return leftSplitter
    }

    private fun buildTopBar(): JComponent {
        val group = DefaultActionGroup().apply {
            add(RefreshAction { refresh() })
            addSeparator()
            ActionManager.getInstance().getAction("sitatame.CopyAIPrompt")?.let { add(it) }
        }
        val toolbar = ActionManager.getInstance()
            .createActionToolbar(ActionPlaces.TOOLWINDOW_CONTENT, group, true)
        toolbar.targetComponent = commentListPane.list

        val baseRefPanel = buildBaseRefSelector()

        val topPanel = JPanel(BorderLayout())
        topPanel.add(toolbar.component, BorderLayout.WEST)
        topPanel.add(baseRefPanel, BorderLayout.CENTER)
        return topPanel
    }

    private fun buildBaseRefSelector(): JComponent {
        val panel = JPanel(FlowLayout(FlowLayout.LEFT, 4, 0))
        panel.add(JBLabel("Base:"))
        panel.add(baseRefCombo)
        // "← <current branch>" makes the diff direction explicit: the changed
        // files are what the current branch adds on top of the selected base.
        panel.add(currentBranchLabel)

        val setDefaultBtn = JButton("Set as default")
        setDefaultBtn.addActionListener { saveBaseRefAsDefault() }
        panel.add(setDefaultBtn)

        baseRefCombo.addItemListener { e ->
            if (suppressBaseRefEvents) return@addItemListener
            if (e.stateChange != ItemEvent.SELECTED) return@addItemListener
            when (val item = baseRefCombo.selectedItem) {
                is BaseRefDiscovery.Entry.Branch -> {
                    lastSelectedBranch = item
                    sessionBaseRef = item.ref
                    refresh()
                }
                // Headers are not real choices; restore the previous branch.
                is BaseRefDiscovery.Entry.Header -> revertHeaderSelection()
                else -> { /* nothing selected */ }
            }
        }

        return panel
    }

    /** Restore the previous branch selection after a non-selectable header was clicked. */
    private fun revertHeaderSelection() {
        val revertTo = lastSelectedBranch ?: return
        suppressBaseRefEvents = true
        try {
            baseRefCombo.selectedItem = revertTo
        } finally {
            suppressBaseRefEvents = false
        }
    }

    private fun buildFooter(): JComponent {
        return JBLabel(" sitatame: Enter = jump  ·  Space = toggle resolved")
    }

    private fun saveBaseRefAsDefault() {
        val ref = (baseRefCombo.selectedItem as? BaseRefDiscovery.Entry.Branch)?.ref ?: return
        val settings = ApplicationManager.getApplication().getService(SitatameSettings::class.java)
        settings?.state?.baseRef = ref
        refresh()
    }

    // -----------------------------------------------------------------------
    // Selection propagation
    // -----------------------------------------------------------------------

    private fun onFileSelected(sel: FileSelection) {
        commentListPane.setFileSelection(sel)
    }

    // -----------------------------------------------------------------------
    // Refresh
    // -----------------------------------------------------------------------

    /**
     * Reload comments from the store and refresh all three panes, and rebuild
     * the base ref selector.
     *
     * Every git *subprocess* — branch discovery and the changed-files diff —
     * plus the store read runs off the EDT; only the Swing model updates happen
     * back on the EDT. Spawning `git branch` on the EDT previously froze the IDE
     * on every base ref selection. (Resolving the repo root / current branch via
     * [RepoContext.forProject] stays on the EDT: it reads git4idea's in-memory
     * state and at most a few KB of `.git/config`, no subprocess.)
     */
    fun refresh() {
        // Bump first, before any early return, so an in-flight async update from
        // an older refresh is invalidated even when this refresh finds no repo.
        val generation = ++refreshGeneration

        val repo = RepoContext.forProject(project) ?: run {
            commentListPane.setComments(emptyList())
            commentDetailPane.showComment(null)
            changedFilesPane.clear()
            currentBranchLabel.text = ""
            return
        }

        val effectiveBaseRef = sessionBaseRef ?: repo.baseRef
        currentBranchLabel.text = "←  ${repo.branch} (current)"

        ApplicationManager.getApplication().executeOnPooledThread {
            val entries = BaseRefDiscovery.listEntries(repo.repoRoot, repo.branch)
            val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)
            val comments = store.snapshotComments(repo.repoRoot, repo.branch)
            ApplicationManager.getApplication().invokeLater {
                // Drop this result if a newer refresh has started — otherwise a
                // slow job could revert the combo / diff to a stale base.
                if (generation != refreshGeneration) return@invokeLater
                // The combo's selected branch is authoritative for the diff so
                // the changed-files list always matches the displayed base.
                val diffBase = rebuildBaseRefCombo(entries, effectiveBaseRef)
                commentListPane.setComments(comments)
                // ChangedFilesPane runs its own async diff; guard it with the
                // same generation so a stale diff can't overwrite the file list.
                changedFilesPane.refresh(diffBase, comments) { generation == refreshGeneration }
            }
        }
    }

    /**
     * Swap in the freshly discovered branch [entries] and select the entry
     * matching [effectiveRef]. Must run on the EDT.
     *
     * Returns the ref that ends up selected (or [effectiveRef] when there are no
     * branches to select), so the caller can diff against exactly what is shown.
     *
     * The item listener is suppressed while the model and selection are
     * mutated: both fire SELECTED events synchronously, and without the guard
     * they would re-enter refresh() and recurse until the IDE hangs.
     */
    private fun rebuildBaseRefCombo(entries: List<BaseRefDiscovery.Entry>, effectiveRef: String): String {
        val selection = BaseRefDiscovery.resolveSelection(entries, effectiveRef)
        val model = DefaultComboBoxModel(selection.entries.toTypedArray())

        suppressBaseRefEvents = true
        try {
            baseRefCombo.model = model
            // Setting the model auto-selects index 0 (a header); override it.
            baseRefCombo.selectedItem = selection.selected
            lastSelectedBranch = selection.selected
        } finally {
            suppressBaseRefEvents = false
        }
        return selection.selected?.ref ?: effectiveRef
    }

    // -----------------------------------------------------------------------
    // Editor jump
    // -----------------------------------------------------------------------

    private fun jumpToComment(comment: Comment) {
        val repo = RepoContext.forProject(project) ?: return
        val absPath = if (comment.anchor.path.startsWith("/")) {
            comment.anchor.path
        } else {
            "${repo.repoRoot}/${comment.anchor.path}"
        }
        val vFile = LocalFileSystem.getInstance().findFileByPath(absPath) ?: return
        val targetLine = when (comment.anchor.kind) {
            AnchorKind.RANGE -> (comment.anchor.lineStart - 1).coerceAtLeast(0)
            AnchorKind.LINE -> (comment.anchor.line - 1).coerceAtLeast(0)
            else -> 0
        }
        OpenFileDescriptor(project, vFile, targetLine, 0).navigate(true)
    }

    // -----------------------------------------------------------------------
    // Private action
    // -----------------------------------------------------------------------

    private class RefreshAction(private val onRun: () -> Unit) :
        AnAction("Refresh", "Reload comments from drafts/", AllIcons.Actions.Refresh) {
        override fun actionPerformed(e: AnActionEvent) { onRun() }
    }

    // -----------------------------------------------------------------------
    // Companion — pure functions (testable without IntelliJ Platform)
    // -----------------------------------------------------------------------

    companion object {

        /**
         * Logical actions that a key-press can trigger in the tool-window list.
         *
         * Sealed so exhaustive `when` is enforced; new bindings can be added here
         * without touching [build]'s KeyAdapter.
         */
        internal enum class KeyAction { JUMP, TOGGLE }

        /**
         * Maps a [java.awt.event.KeyEvent] key code to a [KeyAction], or returns
         * `null` if the key has no binding.
         *
         * Pure function — testable without IntelliJ Platform.
         *
         *  - [KeyEvent.VK_ENTER] → [KeyAction.JUMP] (navigate to anchored file:line)
         *  - [KeyEvent.VK_SPACE] → [KeyAction.TOGGLE] (toggle resolved/open state)
         */
        internal fun keyCodeToAction(keyCode: Int): KeyAction? = when (keyCode) {
            KeyEvent.VK_ENTER -> KeyAction.JUMP
            KeyEvent.VK_SPACE -> KeyAction.TOGGLE
            else -> null
        }

        /**
         * Match a stored [Comment] against [target].
         *
         * Priority:
         * 1. If both have a non-empty anchorId, use exact identity.
         * 2. Otherwise fall back to path + anchor coordinates.
         */
        internal fun commentMatches(c: Comment, target: Comment): Boolean {
            if (c.anchor.anchorId.isNotEmpty() && target.anchor.anchorId.isNotEmpty()) {
                return c.anchor.anchorId == target.anchor.anchorId
            }
            if (c.anchor.path != target.anchor.path) return false
            return when {
                c.anchor.kind == target.anchor.kind && c.anchor.kind == AnchorKind.LINE ->
                    c.anchor.line == target.anchor.line
                c.anchor.kind == target.anchor.kind && c.anchor.kind == AnchorKind.RANGE ->
                    c.anchor.lineStart == target.anchor.lineStart && c.anchor.lineEnd == target.anchor.lineEnd
                else -> false
            }
        }

        /**
         * Filter [comments] by [filter]. Pure function — testable without Platform.
         */
        internal fun filterComments(comments: List<Comment>, filter: FilterState): List<Comment> =
            when (filter) {
                FilterState.ALL -> comments
                FilterState.OPENED -> comments.filter { it.state == ReviewState.OPEN }
                FilterState.RESOLVED -> comments.filter { it.state == ReviewState.RESOLVED }
            }
    }
}
