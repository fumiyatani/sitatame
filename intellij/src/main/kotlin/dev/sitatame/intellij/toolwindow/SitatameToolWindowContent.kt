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
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.Messages
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.ui.JBColor
import com.intellij.ui.OnePixelSplitter
import com.intellij.ui.components.JBList
import com.intellij.ui.components.JBRadioButton
import com.intellij.ui.components.JBScrollPane
import dev.sitatame.intellij.actions.CopyAIPromptAction
import dev.sitatame.intellij.actions.ToolWindowToggleResolvedAction
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.REVIEW_CHANGED_TOPIC
import dev.sitatame.intellij.storage.ReviewChangedListener
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore
import java.awt.BasicStroke
import java.awt.BorderLayout
import java.awt.Component
import java.awt.FlowLayout
import java.awt.Graphics
import java.awt.Graphics2D
import java.awt.RenderingHints
import java.awt.event.KeyAdapter
import java.awt.event.KeyEvent
import java.awt.event.MouseAdapter
import java.awt.event.MouseEvent
import java.awt.geom.GeneralPath
import javax.swing.ButtonGroup
import javax.swing.DefaultListModel
import javax.swing.Icon
import javax.swing.JComponent
import javax.swing.JLabel
import javax.swing.JList
import javax.swing.JMenuItem
import javax.swing.JPanel
import javax.swing.JPopupMenu
import javax.swing.JTextArea
import javax.swing.ListCellRenderer
import javax.swing.ListSelectionModel
import javax.swing.SwingConstants

/** Filter state for the comment list. */
enum class FilterState { ALL, OPENED, RESOLVED }

/**
 * Two-pane tool window content:
 *
 *   - Left: list of all comments for the current branch, with an icon
 *     summarising state (open / resolved / stale). A filter bar at the top
 *     lets the user narrow the list to All / Opened / Resolved.
 *   - Right: details pane for the selected comment (anchor + body).
 *
 * A toolbar above hosts Refresh / Copy AI prompt. Selection changes refresh
 * the right pane; double-click jumps the editor to the anchored line via
 * [OpenFileDescriptor].
 *
 * @param disposable Lifecycle scope for the MessageBus subscription. Pass
 *   [com.intellij.openapi.wm.ToolWindow] (which implements [Disposable]) so
 *   the subscription is cleaned up when the tool window is disposed.
 */
class SitatameToolWindowContent(
    private val project: Project,
    disposable: Disposable,
) {

    private val log = Logger.getInstance(SitatameToolWindowContent::class.java)

    /** Current filter; mutations always trigger a list reload from cache. */
    private var currentFilter: FilterState = FilterState.ALL

    private val listModel = DefaultListModel<Comment>()
    private val list = JBList(listModel).apply {
        selectionMode = ListSelectionModel.SINGLE_SELECTION
        cellRenderer = CommentRenderer { index -> onTrashClicked(index) }
    }
    private val detailsArea = JTextArea().apply {
        isEditable = false
        lineWrap = true
        wrapStyleWord = true
    }

    val component: JComponent = build()

    init {
        // Subscribe to review mutations from other parts of the plugin (actions,
        // other tool windows). Scoped to [disposable] so we don't leak listeners.
        ApplicationManager.getApplication().messageBus
            .connect(disposable)
            .subscribe(
                REVIEW_CHANGED_TOPIC,
                ReviewChangedListener { changedRoot, changedBranch ->
                    // All project state access (GitRepositoryManager / RepoContext) and
                    // UI mutations must happen on the EDT. Wrap the entire callback body
                    // in invokeLater so we never touch project state off-EDT.
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
        // Exposing the current selection via DataProvider lets the registered
        // CopyAIPromptAction read it from any context (toolbar button,
        // right-click menu) without us reaching into the JBList from inside
        // the action class.
        val outer = object : JPanel(BorderLayout()), DataProvider {
            override fun getData(dataId: String): Any? {
                if (CopyAIPromptAction.SELECTED_COMMENTS_KEY.`is`(dataId)) {
                    return list.selectedValuesList.toList()
                }
                if (ToolWindowToggleResolvedAction.TOGGLE_SELECTED_KEY.`is`(dataId)) {
                    // Only expose the runnable when a comment is actually selected so
                    // ToolWindowToggleResolvedAction.update() can disable itself.
                    return if (list.selectedValue != null) Runnable { toggleSelected() } else null
                }
                return null
            }
        }

        val topPanel = JPanel(BorderLayout()).apply {
            add(buildToolbar().component, BorderLayout.NORTH)
            add(buildFilterBar(), BorderLayout.SOUTH)
        }
        outer.add(topPanel, BorderLayout.NORTH)

        val splitter = OnePixelSplitter(false, 0.45f).apply {
            firstComponent = JBScrollPane(list)
            secondComponent = JBScrollPane(detailsArea)
        }
        outer.add(splitter, BorderLayout.CENTER)
        outer.add(buildFooter(), BorderLayout.SOUTH)

        list.addListSelectionListener { renderDetails() }
        list.addMouseListener(object : MouseAdapter() {
            override fun mouseClicked(e: MouseEvent) {
                if (e.clickCount == 2) jumpToSelected()
            }

            override fun mousePressed(e: MouseEvent) {
                maybeShowPopup(e)
            }

            override fun mouseReleased(e: MouseEvent) {
                maybeShowPopup(e)
            }

            private fun maybeShowPopup(e: MouseEvent) {
                if (!e.isPopupTrigger) return
                // Select the row under the cursor so the menu action targets it.
                val index = list.locationToIndex(e.point)
                if (index < 0 || list.getCellBounds(index, index)?.contains(e.point) != true) return
                list.selectedIndex = index
                buildContextMenu().show(list, e.x, e.y)
            }
        })
        list.addKeyListener(object : KeyAdapter() {
            override fun keyPressed(e: KeyEvent) {
                when (e.keyCode) {
                    KeyEvent.VK_ENTER -> {
                        e.consume()
                        jumpToSelected()
                    }
                    KeyEvent.VK_SPACE -> {
                        e.consume()
                        toggleSelected()
                    }
                }
            }
        })

        refresh()
        return outer
    }

    /** Build the three-way filter bar: All / Opened / Resolved. */
    private fun buildFilterBar(): JComponent {
        val panel = JPanel(FlowLayout(FlowLayout.LEFT, 4, 2))
        val group = ButtonGroup()

        fun addRadio(labelText: String, state: FilterState, selected: Boolean) {
            val btn = JBRadioButton(labelText, selected)
            group.add(btn)
            btn.addActionListener {
                currentFilter = state
                applyFilter()
            }
            panel.add(btn)
        }

        addRadio("All", FilterState.ALL, selected = true)
        addRadio("Opened", FilterState.OPENED, selected = false)
        addRadio("Resolved", FilterState.RESOLVED, selected = false)

        return panel
    }

    private fun buildToolbar(): com.intellij.openapi.actionSystem.ActionToolbar {
        val group = DefaultActionGroup().apply {
            add(RefreshAction { refresh() })
            addSeparator()
            ActionManager.getInstance().getAction("sitatame.CopyAIPrompt")?.let { add(it) }
        }
        val toolbar = ActionManager.getInstance()
            .createActionToolbar(ActionPlaces.TOOLWINDOW_CONTENT, group, true)
        toolbar.targetComponent = list
        return toolbar
    }

    private fun buildFooter(): JComponent {
        val label = JLabel("", SwingConstants.LEFT)
        label.text = " sitatame: drafts under ~/.sitatame/<project>/drafts/<branch>/"
        return label
    }

    /** Build the right-click popup menu for the comment list. */
    private fun buildContextMenu(): JPopupMenu {
        val menu = JPopupMenu()
        val selected = list.selectedValue
        // Show state-aware label: "Mark Resolved" for open comments, "Reopen" for resolved ones.
        val toggleLabel = if (selected?.state == ReviewState.RESOLVED) "Reopen" else "Mark Resolved"
        val toggleItem = JMenuItem(toggleLabel)
        toggleItem.addActionListener { toggleSelected() }
        menu.add(toggleItem)

        val deleteItem = JMenuItem("Delete", AllIcons.General.Remove)
        deleteItem.addActionListener { deleteSelected() }
        menu.add(deleteItem)

        return menu
    }

    /**
     * Toggle open ↔ resolved for the currently selected comment.
     *
     * Matching uses anchorId when non-empty (exact identity); falls back to
     * path + line / path + range comparison for comments written before
     * anchorId was introduced.
     *
     * Runs file I/O on a pooled thread, then calls [refresh] on EDT.
     * Auto-refresh is also driven by [REVIEW_CHANGED_TOPIC] subscription
     * (set up in init), so the explicit [refresh] call here is for
     * immediate response in the same tool window.
     */
    private fun toggleSelected() {
        val selected = list.selectedValue ?: return
        val repo = RepoContext.forProject(project) ?: return
        ApplicationManager.getApplication().executeOnPooledThread {
            val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)
            try {
                store.toggleComment(repo.repoRoot, repo.branch) { c ->
                    commentMatches(c, selected)
                }
            } catch (ex: Exception) {
                log.warn("SitatameToolWindowContent: toggleSelected failed", ex)
            }
            // Invalidate cache so refresh picks up the mutated state from disk.
            store.invalidate()
            ApplicationManager.getApplication().invokeLater { refresh() }
        }
    }

    /**
     * Delete the currently selected comment after showing a confirmation dialog.
     * Runs file I/O on a pooled thread.
     */
    private fun deleteSelected() {
        val selected = list.selectedValue ?: return
        val repo = RepoContext.forProject(project) ?: return

        val confirm = Messages.showOkCancelDialog(
            project,
            "Delete this comment?\n\n${selected.body.take(120)}",
            "Delete Comment",
            "Delete",
            "Cancel",
            Messages.getWarningIcon(),
        )
        if (confirm != Messages.OK) return

        ApplicationManager.getApplication().executeOnPooledThread {
            val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)
            try {
                store.removeComment(repo.repoRoot, repo.branch) { c ->
                    commentMatches(c, selected)
                }
            } catch (ex: Exception) {
                log.warn("SitatameToolWindowContent: deleteSelected failed", ex)
            }
            store.invalidate()
            ApplicationManager.getApplication().invokeLater { refresh() }
        }
    }

    /**
     * Called by [CommentRenderer] when the trash icon area is clicked for a row.
     */
    private fun onTrashClicked(index: Int) {
        list.selectedIndex = index
        deleteSelected()
    }

    /**
     * Match a stored [Comment] against the [target] selected in the list.
     *
     * Delegates to [Companion.commentMatches] so the logic is testable
     * without instantiating the tool window (no IntelliJ Platform required).
     */
    private fun commentMatches(c: Comment, target: Comment): Boolean =
        Companion.commentMatches(c, target)

    companion object {
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

        // Icon constants shared between CommentRenderer and StateIcon.
        internal const val STATE_ICON_SIZE = 12
        // Trash icon width (icon + 4px padding on each side)
        internal const val TRASH_CLICK_WIDTH = STATE_ICON_SIZE + 8
    }

    /**
     * Reload the comment list from the store. Triggered on tool window open,
     * after Refresh, and after each mutating action (see [REVIEW_CHANGED_TOPIC]
     * subscription in init for the MessageBus-driven path).
     */
    fun refresh() {
        val repo = RepoContext.forProject(project) ?: run {
            ApplicationManager.getApplication().invokeLater {
                listModel.clear()
                detailsArea.text = "(no Git repository detected)"
            }
            return
        }
        ApplicationManager.getApplication().executeOnPooledThread {
            val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)
            val comments = store.snapshotComments(repo.repoRoot, repo.branch)
            ApplicationManager.getApplication().invokeLater {
                updateListModel(comments)
                renderDetails()
            }
        }
    }

    /** Apply the current filter to [allComments] and update the list model. */
    private fun updateListModel(allComments: List<Comment>) {
        val filtered = filterComments(allComments, currentFilter)
        listModel.clear()
        for (c in filtered) listModel.addElement(c)
    }

    /** Re-apply the current filter to the already-loaded snapshot. */
    private fun applyFilter() {
        val repo = RepoContext.forProject(project) ?: return
        ApplicationManager.getApplication().executeOnPooledThread {
            val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)
            val comments = store.snapshotComments(repo.repoRoot, repo.branch)
            ApplicationManager.getApplication().invokeLater {
                updateListModel(comments)
                renderDetails()
            }
        }
    }

    private fun renderDetails() {
        val selected = list.selectedValue
        if (selected == null) {
            detailsArea.text = ""
            return
        }
        val a = selected.anchor
        val anchorDesc = when (a.kind) {
            AnchorKind.RANGE -> "${a.path}:${a.lineStart}-${a.lineEnd} (range)"
            AnchorKind.LINE -> "${a.path}:${a.line} (line)"
            else -> "${a.path} (${a.kind})"
        }
        detailsArea.text = buildString {
            append("anchor: ").append(anchorDesc).append('\n')
            append("state: ").append(selected.state).append('\n')
            append("\n")
            append(selected.body)
        }
        detailsArea.caretPosition = 0
    }

    /**
     * Jump the editor to the anchored file:line via OpenFileDescriptor.
     * For Range anchors we navigate to the first line.
     */
    private fun jumpToSelected() {
        val selected = list.selectedValue ?: return
        val repo = RepoContext.forProject(project) ?: return
        val absPath = if (selected.anchor.path.startsWith("/")) {
            selected.anchor.path
        } else {
            "${repo.repoRoot}/${selected.anchor.path}"
        }
        val vFile = LocalFileSystem.getInstance().findFileByPath(absPath) ?: return
        val targetLine = when (selected.anchor.kind) {
            AnchorKind.RANGE -> (selected.anchor.lineStart - 1).coerceAtLeast(0)
            AnchorKind.LINE -> (selected.anchor.line - 1).coerceAtLeast(0)
            else -> 0
        }
        OpenFileDescriptor(project, vFile, targetLine, 0).navigate(true)
    }

    private class RefreshAction(private val onRun: () -> Unit) :
        AnAction("Refresh", "Reload comments from drafts/", AllIcons.Actions.Refresh) {
        override fun actionPerformed(e: AnActionEvent) { onRun() }
    }

    /**
     * Renders each comment row with a state icon (shape + colour distinct) on
     * the left and a trash-can delete icon on the right.
     *
     * Icon shapes:
     *  - Opened: filled green circle (●)
     *  - Resolved: purple check mark (✓)
     *  - Stale: yellow warning triangle (platform AllIcons.General.Warning)
     *
     * @param onTrashClick invoked with the row index when the trash icon area is clicked.
     */
    private class CommentRenderer(
        private val onTrashClick: (Int) -> Unit,
    ) : ListCellRenderer<Comment> {

        private val label = JLabel()
        private val trashLabel = JLabel(AllIcons.General.Remove)
        private val row = JPanel(BorderLayout(4, 0)).apply {
            add(label, BorderLayout.CENTER)
            add(trashLabel, BorderLayout.EAST)
        }

        // Detect trash-icon clicks via MouseAdapter on the owning JBList.
        // We register the adapter lazily the first time getListCellRendererComponent
        // is called (list reference is available then). Guard with a flag.
        private var mouseAdapterInstalled = false

        override fun getListCellRendererComponent(
            list: JList<out Comment>?,
            value: Comment?,
            index: Int,
            isSelected: Boolean,
            cellHasFocus: Boolean,
        ): Component {
            value ?: return row

            // Lazily attach the mouse adapter to detect trash-icon clicks.
            if (!mouseAdapterInstalled && list != null) {
                list.addMouseListener(object : MouseAdapter() {
                    override fun mouseClicked(e: MouseEvent) {
                        if (e.button != MouseEvent.BUTTON1) return
                        val idx = list.locationToIndex(e.point)
                        if (idx < 0) return
                        val cellBounds = list.getCellBounds(idx, idx) ?: return
                        if (!cellBounds.contains(e.point)) return
                        // Trash icon occupies the right TRASH_CLICK_WIDTH pixels of the cell.
                        if (e.x >= cellBounds.x + cellBounds.width - TRASH_CLICK_WIDTH) {
                            onTrashClick(idx)
                        }
                    }
                })
                mouseAdapterInstalled = true
            }

            label.icon = stateIcon(value.state)
            val locator = when (value.anchor.kind) {
                AnchorKind.RANGE -> "${value.anchor.path}:${value.anchor.lineStart}-${value.anchor.lineEnd}"
                AnchorKind.LINE -> "${value.anchor.path}:${value.anchor.line}"
                else -> value.anchor.path
            }
            val firstLine = value.body.lineSequence().firstOrNull().orEmpty().take(80)
            label.text = "<html><b>$locator</b> &mdash; ${escapeHtml(firstLine)}</html>"

            if (isSelected) {
                row.background = list?.selectionBackground
                label.background = list?.selectionBackground
                label.foreground = list?.selectionForeground
                row.isOpaque = true
                label.isOpaque = true
            } else {
                row.isOpaque = false
                label.isOpaque = false
                label.foreground = null
            }
            return row
        }

        private fun escapeHtml(s: String): String =
            s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")

        companion object {
            // -----------------------------------------------------------------------
            // State icons — shape + colour distinct for colour-blind accessibility
            // -----------------------------------------------------------------------

            private enum class StateIconShape { CIRCLE, CHECK }

            /**
             * Lightweight [Icon] implementation that draws either a filled circle
             * (opened comments, green) or a check-mark (resolved, purple) using
             * Java2D so no external image resources are needed.
             */
            private class StateIcon(
                private val color: JBColor,
                private val shape: StateIconShape,
            ) : Icon {
                override fun getIconWidth() = STATE_ICON_SIZE
                override fun getIconHeight() = STATE_ICON_SIZE

                override fun paintIcon(c: Component?, g: Graphics, x: Int, y: Int) {
                    val g2 = g.create() as Graphics2D
                    g2.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON)
                    g2.color = color
                    when (shape) {
                        StateIconShape.CIRCLE -> g2.fillOval(x, y, STATE_ICON_SIZE, STATE_ICON_SIZE)
                        StateIconShape.CHECK -> {
                            val s = STATE_ICON_SIZE.toFloat()
                            g2.stroke = BasicStroke(2f, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND)
                            val path = GeneralPath().apply {
                                moveTo(x + s * 0.15f, y + s * 0.50f)
                                lineTo(x + s * 0.40f, y + s * 0.75f)
                                lineTo(x + s * 0.85f, y + s * 0.20f)
                            }
                            g2.draw(path)
                        }
                    }
                    g2.dispose()
                }
            }

            // Cached icon instances — avoids allocating a new StateIcon on every
            // list repaint (getListCellRendererComponent is called once per visible row
            // per repaint cycle).
            private val RESOLVED_ICON: Icon = StateIcon(
                JBColor(0x9C27B0, 0xBA68C8),  // purple light/dark
                StateIconShape.CHECK,
            )
            private val OPEN_ICON: Icon = StateIcon(
                JBColor(0x4CAF50, 0x81C784),  // green light/dark
                StateIconShape.CIRCLE,
            )
            private val STALE_ICON: Icon = AllIcons.General.Warning

            fun stateIcon(state: String): Icon = when (state) {
                ReviewState.RESOLVED -> RESOLVED_ICON
                ReviewState.STALE -> STALE_ICON
                else -> OPEN_ICON  // OPEN (default)
            }
        }
    }
}
