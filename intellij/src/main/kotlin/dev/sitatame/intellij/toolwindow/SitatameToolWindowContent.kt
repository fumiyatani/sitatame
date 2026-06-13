package dev.sitatame.intellij.toolwindow

import com.intellij.icons.AllIcons
import com.intellij.openapi.actionSystem.ActionManager
import com.intellij.openapi.actionSystem.ActionPlaces
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.DataProvider
import com.intellij.openapi.actionSystem.DefaultActionGroup
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.OpenFileDescriptor
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.ui.OnePixelSplitter
import com.intellij.ui.components.JBList
import com.intellij.ui.components.JBScrollPane
import dev.sitatame.intellij.actions.CopyAIPromptAction
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore
import java.awt.BorderLayout
import java.awt.Component
import javax.swing.DefaultListModel
import javax.swing.Icon
import javax.swing.JComponent
import javax.swing.JLabel
import javax.swing.JList
import javax.swing.JPanel
import javax.swing.JTextArea
import javax.swing.ListSelectionModel
import javax.swing.SwingConstants
import javax.swing.ListCellRenderer

/**
 * Two-pane tool window content:
 *
 *   - Left: list of all comments for the current branch, with an icon
 *     summarising state (open / resolved / stale).
 *   - Right: details pane for the selected comment (anchor + body).
 *
 * A toolbar above hosts Refresh / Promote / Copy AI prompt. Selection
 * changes refresh the right pane; double-click jumps the editor to the
 * anchored line via [OpenFileDescriptor].
 */
class SitatameToolWindowContent(private val project: Project) {

    private val listModel = DefaultListModel<Comment>()
    private val list = JBList(listModel).apply {
        selectionMode = ListSelectionModel.SINGLE_SELECTION
        cellRenderer = CommentRenderer()
    }
    private val detailsArea = JTextArea().apply {
        isEditable = false
        lineWrap = true
        wrapStyleWord = true
    }

    val component: JComponent = build()

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
                return null
            }
        }
        outer.add(buildToolbar().component, BorderLayout.NORTH)

        val splitter = OnePixelSplitter(false, 0.45f).apply {
            firstComponent = JBScrollPane(list)
            secondComponent = JBScrollPane(detailsArea)
        }
        outer.add(splitter, BorderLayout.CENTER)
        outer.add(buildFooter(), BorderLayout.SOUTH)

        list.addListSelectionListener { renderDetails() }
        list.addMouseListener(object : java.awt.event.MouseAdapter() {
            override fun mouseClicked(e: java.awt.event.MouseEvent) {
                if (e.clickCount == 2) jumpToSelected()
            }
        })

        refresh()
        return outer
    }

    private fun buildToolbar(): com.intellij.openapi.actionSystem.ActionToolbar {
        val group = DefaultActionGroup().apply {
            add(RefreshAction { refresh() })
            addSeparator()
            // Reuse the registered Promote and Copy actions so any keybindings
            // applied to them in the IDE also work from this toolbar.
            ActionManager.getInstance().getAction("sitatame.PromoteReview")?.let { add(it) }
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

    /**
     * Reload the comment list from the store. Triggered on tool window open,
     * after Refresh, and after each mutating action (see Phase 2: subscribe
     * to a topic instead of polling).
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
                listModel.clear()
                for (c in comments) listModel.addElement(c)
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

    private class CommentRenderer : ListCellRenderer<Comment> {
        private val delegate = JLabel()
        override fun getListCellRendererComponent(
            list: JList<out Comment>?,
            value: Comment?,
            index: Int,
            isSelected: Boolean,
            cellHasFocus: Boolean,
        ): Component {
            value ?: return delegate
            val icon: Icon = when (value.state) {
                ReviewState.RESOLVED -> AllIcons.Actions.Checked
                ReviewState.STALE -> AllIcons.General.Warning
                else -> AllIcons.General.Note
            }
            val locator = when (value.anchor.kind) {
                AnchorKind.RANGE -> "${value.anchor.path}:${value.anchor.lineStart}-${value.anchor.lineEnd}"
                AnchorKind.LINE -> "${value.anchor.path}:${value.anchor.line}"
                else -> value.anchor.path
            }
            delegate.icon = icon
            val firstLine = value.body.lineSequence().firstOrNull().orEmpty().take(80)
            delegate.text = "<html><b>$locator</b> &mdash; ${escapeHtml(firstLine)}</html>"
            if (isSelected) {
                delegate.background = list?.selectionBackground
                delegate.foreground = list?.selectionForeground
                delegate.isOpaque = true
            } else {
                delegate.isOpaque = false
            }
            return delegate
        }

        private fun escapeHtml(s: String): String =
            s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
    }
}
