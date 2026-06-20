package dev.sitatame.intellij.toolwindow.panes

import com.intellij.icons.AllIcons
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.Messages
import com.intellij.ui.JBColor
import com.intellij.ui.components.JBList
import com.intellij.ui.components.JBScrollPane
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.REVIEW_CHANGED_TOPIC
import dev.sitatame.intellij.storage.ReviewChangedListener
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore
import dev.sitatame.intellij.toolwindow.FilterState
import dev.sitatame.intellij.toolwindow.SitatameToolWindowContent
import java.awt.BasicStroke
import java.awt.BorderLayout
import java.awt.Component
import java.awt.FlowLayout
import java.awt.Graphics
import java.awt.Graphics2D
import java.awt.RenderingHints
import java.awt.event.MouseAdapter
import java.awt.event.MouseEvent
import java.awt.geom.GeneralPath
import javax.swing.ButtonGroup
import javax.swing.DefaultListModel
import javax.swing.Icon
import javax.swing.JLabel
import javax.swing.JList
import javax.swing.JMenuItem
import javax.swing.JPanel
import javax.swing.JPopupMenu
import javax.swing.ListCellRenderer
import javax.swing.ListSelectionModel
import com.intellij.ui.components.JBRadioButton

/**
 * Middle pane of the 3-pane layout: shows comments filtered by state and
 * optionally by the selected file.
 *
 * State filter (All / Opened / Resolved) lives in this pane's toolbar.
 * File filter is driven externally via [setFileSelection].
 * Row selection drives the right pane via [onCommentSelected].
 */
class CommentListPane(
    private val project: Project,
    private val onCommentSelected: (Comment?) -> Unit,
    private val onJumpRequested: (Comment) -> Unit,
    private val onRefreshAll: () -> Unit,
) {

    private val log = Logger.getInstance(CommentListPane::class.java)

    private var currentFilter: FilterState = FilterState.ALL
    private var currentSelection: FileSelection = FileSelection.All

    /** Full snapshot of all branch comments — filtered subset is shown in the list. */
    private var allComments: List<Comment> = emptyList()

    private val listModel = DefaultListModel<Comment>()
    val list = JBList(listModel).apply {
        selectionMode = ListSelectionModel.SINGLE_SELECTION
        cellRenderer = CommentCellRenderer { idx -> onTrashClicked(idx) }
    }

    val component: JPanel = build()

    init {
        list.addListSelectionListener {
            if (!it.valueIsAdjusting) onCommentSelected(list.selectedValue)
        }
        list.addMouseListener(object : MouseAdapter() {
            override fun mouseClicked(e: MouseEvent) {
                if (e.clickCount == 2) list.selectedValue?.let { onJumpRequested(it) }
            }

            override fun mousePressed(e: MouseEvent) { maybeShowPopup(e) }
            override fun mouseReleased(e: MouseEvent) { maybeShowPopup(e) }

            private fun maybeShowPopup(e: MouseEvent) {
                if (!e.isPopupTrigger) return
                val index = list.locationToIndex(e.point)
                if (index < 0 || list.getCellBounds(index, index)?.contains(e.point) != true) return
                list.selectedIndex = index
                buildContextMenu()?.show(list, e.x, e.y)
            }
        })
    }

    private fun build(): JPanel {
        val panel = JPanel(BorderLayout())
        panel.add(buildFilterBar(), BorderLayout.NORTH)
        panel.add(JBScrollPane(list), BorderLayout.CENTER)
        return panel
    }

    private fun buildFilterBar(): JPanel {
        val panel = JPanel(FlowLayout(FlowLayout.LEFT, 4, 2))
        val group = ButtonGroup()

        fun addRadio(labelText: String, state: FilterState, selected: Boolean) {
            val btn = JBRadioButton(labelText, selected)
            group.add(btn)
            btn.addActionListener {
                currentFilter = state
                rebuildList()
            }
            panel.add(btn)
        }

        addRadio("All", FilterState.ALL, selected = true)
        addRadio("Opened", FilterState.OPENED, selected = false)
        addRadio("Resolved", FilterState.RESOLVED, selected = false)

        return panel
    }

    private fun buildContextMenu(): JPopupMenu? {
        val selected = list.selectedValue ?: return null
        val menu = JPopupMenu()
        val toggleLabel = if (selected.state == ReviewState.RESOLVED) "Reopen" else "Mark Resolved"
        val toggleItem = JMenuItem(toggleLabel)
        toggleItem.addActionListener { toggleSelected() }
        menu.add(toggleItem)
        val deleteItem = JMenuItem("Delete", AllIcons.General.Remove)
        deleteItem.addActionListener { deleteSelected() }
        menu.add(deleteItem)
        return menu
    }

    // -----------------------------------------------------------------------
    // Public API
    // -----------------------------------------------------------------------

    /** Update the full snapshot and rebuild the list. Called by [ThreePaneContent] on refresh. */
    fun setComments(comments: List<Comment>) {
        allComments = comments
        rebuildList()
    }

    /** Called when a file is selected in the left pane. */
    fun setFileSelection(sel: FileSelection) {
        currentSelection = sel
        rebuildList()
    }

    /** Toggle open/resolved for the currently selected comment. */
    fun toggleSelected() {
        val selected = list.selectedValue ?: return
        val repo = RepoContext.forProject(project) ?: return
        ApplicationManager.getApplication().executeOnPooledThread {
            val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)
            try {
                store.toggleComment(repo.repoRoot, repo.branch) { c ->
                    SitatameToolWindowContent.commentMatches(c, selected)
                }
            } catch (ex: Exception) {
                log.warn("CommentListPane: toggleSelected failed", ex)
            }
            store.invalidate()
            ApplicationManager.getApplication().invokeLater { onRefreshAll() }
        }
    }

    /** Delete the currently selected comment (with confirmation dialog). */
    fun deleteSelected() {
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
                    SitatameToolWindowContent.commentMatches(c, selected)
                }
            } catch (ex: Exception) {
                log.warn("CommentListPane: deleteSelected failed", ex)
            }
            store.invalidate()
            ApplicationManager.getApplication().invokeLater { onRefreshAll() }
        }
    }

    // -----------------------------------------------------------------------
    // Private helpers
    // -----------------------------------------------------------------------

    private fun onTrashClicked(index: Int) {
        list.selectedIndex = index
        deleteSelected()
    }

    private fun rebuildList() {
        val filtered = filterAndSubset(allComments, currentFilter, currentSelection)
        listModel.clear()
        for (c in filtered) listModel.addElement(c)
        // Preserve selection across rebuild where possible.
        if (listModel.isEmpty) onCommentSelected(null)
    }

    // -----------------------------------------------------------------------
    // Companion — pure functions (testable without IntelliJ Platform)
    // -----------------------------------------------------------------------

    companion object {

        /**
         * Apply [filter] and [selection] to [comments] and return the matching subset.
         *
         * Pure function — testable without IntelliJ Platform.
         *
         * [FileSelection.All] → all comments pass the file filter.
         * [FileSelection.One] → only comments whose anchor.path matches the relPath.
         * State filter is applied on top.
         */
        fun filterAndSubset(
            comments: List<Comment>,
            filter: FilterState,
            selection: FileSelection,
        ): List<Comment> {
            val fileFiltered = when (selection) {
                is FileSelection.All -> comments
                is FileSelection.One -> comments.filter { c ->
                    c.anchor.path == selection.relPath ||
                        c.anchor.path.endsWith("/${selection.relPath}") ||
                        selection.relPath.endsWith("/${c.anchor.path}")
                }
            }
            return SitatameToolWindowContent.filterComments(fileFiltered, filter)
        }

        /**
         * Human-readable locator for a comment's [anchor], shown as the bold
         * prefix of each list row and in the detail pane. Covers all four anchor
         * kinds so REVIEW / FILE comments (added for TUI parity) are not rendered
         * as a bare/empty path.
         *
         * Pure function — testable without the IntelliJ Platform.
         */
        fun locatorFor(anchor: Anchor): String = when (anchor.kind) {
            AnchorKind.RANGE -> "${anchor.path}:${anchor.lineStart}-${anchor.lineEnd}"
            AnchorKind.LINE -> "${anchor.path}:${anchor.line}"
            AnchorKind.FILE -> "${anchor.path} (file)"
            AnchorKind.REVIEW -> "(review)"
            else -> anchor.path
        }

        // Icon size constants shared with CommentCellRenderer
        internal const val STATE_ICON_SIZE = 12
        internal const val TRASH_CLICK_WIDTH = STATE_ICON_SIZE + 8

        // -----------------------------------------------------------------------
        // State icons — same shapes as the original CommentRenderer
        // -----------------------------------------------------------------------

        private enum class StateIconShape { CIRCLE, CHECK }

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

        private val RESOLVED_ICON: Icon = StateIcon(JBColor(0x9C27B0, 0xBA68C8), StateIconShape.CHECK)
        private val OPEN_ICON: Icon = StateIcon(JBColor(0x4CAF50, 0x81C784), StateIconShape.CIRCLE)
        private val STALE_ICON: Icon = AllIcons.General.Warning

        fun stateIcon(state: String): Icon = when (state) {
            ReviewState.RESOLVED -> RESOLVED_ICON
            ReviewState.STALE -> STALE_ICON
            else -> OPEN_ICON
        }
    }

    // -----------------------------------------------------------------------
    // Cell renderer
    // -----------------------------------------------------------------------

    private class CommentCellRenderer(
        private val onTrashClick: (Int) -> Unit,
    ) : ListCellRenderer<Comment> {

        private val label = JLabel()
        private val trashLabel = JLabel(AllIcons.General.Remove)
        private val row = JPanel(BorderLayout(4, 0)).apply {
            add(label, BorderLayout.CENTER)
            add(trashLabel, BorderLayout.EAST)
        }
        private var mouseAdapterInstalled = false

        override fun getListCellRendererComponent(
            list: JList<out Comment>?,
            value: Comment?,
            index: Int,
            isSelected: Boolean,
            cellHasFocus: Boolean,
        ): Component {
            value ?: return row
            if (!mouseAdapterInstalled && list != null) {
                list.addMouseListener(object : MouseAdapter() {
                    override fun mouseClicked(e: MouseEvent) {
                        if (e.button != MouseEvent.BUTTON1) return
                        val idx = list.locationToIndex(e.point)
                        if (idx < 0) return
                        val cellBounds = list.getCellBounds(idx, idx) ?: return
                        if (!cellBounds.contains(e.point)) return
                        if (e.x >= cellBounds.x + cellBounds.width - TRASH_CLICK_WIDTH) {
                            onTrashClick(idx)
                        }
                    }
                })
                mouseAdapterInstalled = true
            }

            label.icon = stateIcon(value.state)
            val locator = locatorFor(value.anchor)
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
    }
}
