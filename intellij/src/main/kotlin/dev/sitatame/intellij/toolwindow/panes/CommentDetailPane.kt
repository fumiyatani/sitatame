package dev.sitatame.intellij.toolwindow.panes

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.Messages
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore
import dev.sitatame.intellij.toolwindow.SitatameToolWindowContent
import com.intellij.util.ui.JBUI
import java.awt.BorderLayout
import java.awt.FlowLayout
import javax.swing.JButton
import javax.swing.JPanel
import javax.swing.JScrollPane
import javax.swing.JTextArea

/**
 * Right pane of the 3-pane layout: shows full detail for the selected comment.
 *
 * Shows: anchor location, state, body text.
 * Primary actions: Resolve/Reopen, Delete.
 * Reply button is present but disabled (tooltip "Coming soon").
 */
class CommentDetailPane(
    private val project: Project,
    private val onRefreshAll: () -> Unit,
) {

    private val log = Logger.getInstance(CommentDetailPane::class.java)

    private var currentComment: Comment? = null

    private val bodyArea = JTextArea().apply {
        isEditable = false
        lineWrap = true
        wrapStyleWord = true
        // Breathing room so wrapped text doesn't hug the pane edges (esp. the right).
        border = JBUI.Borders.empty(8, 10)
    }

    private val resolveButton = JButton("Resolve")
    private val deleteButton = JButton("Delete")
    private val replyButton = JButton("Reply").apply {
        isEnabled = false
        toolTipText = "Coming soon"
    }

    val component: JPanel = build()

    private fun build(): JPanel {
        val panel = JPanel(BorderLayout(0, 0))
        panel.add(JScrollPane(bodyArea), BorderLayout.CENTER)
        panel.add(buildButtonBar(), BorderLayout.SOUTH)

        resolveButton.addActionListener { resolveOrReopen() }
        deleteButton.addActionListener { deleteComment() }

        return panel
    }

    private fun buildButtonBar(): JPanel {
        val bar = JPanel(FlowLayout(FlowLayout.LEFT, 6, 4))
        bar.add(resolveButton)
        bar.add(deleteButton)
        bar.add(replyButton)
        return bar
    }

    // -----------------------------------------------------------------------
    // Public API
    // -----------------------------------------------------------------------

    /** Called by the middle pane when the row selection changes. */
    fun showComment(comment: Comment?) {
        currentComment = comment
        render()
    }

    // -----------------------------------------------------------------------
    // Private helpers
    // -----------------------------------------------------------------------

    private fun render() {
        val c = currentComment
        if (c == null) {
            bodyArea.text = ""
            resolveButton.isEnabled = false
            deleteButton.isEnabled = false
            replyButton.isEnabled = false
            return
        }

        val anchorDesc = CommentListPane.locatorFor(c.anchor)

        bodyArea.text = buildString {
            append("anchor: ").append(anchorDesc).append('\n')
            append("state:  ").append(c.state).append('\n')
            if (c.body.isNotBlank()) {
                append('\n')
                append(c.body)
            }
        }
        bodyArea.caretPosition = 0

        resolveButton.text = if (c.state == ReviewState.RESOLVED) "Reopen" else "Resolve"
        resolveButton.isEnabled = true
        deleteButton.isEnabled = true
        replyButton.isEnabled = false
    }

    private fun resolveOrReopen() {
        val selected = currentComment ?: return
        val repo = RepoContext.forProject(project) ?: return
        ApplicationManager.getApplication().executeOnPooledThread {
            val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)
            try {
                store.toggleComment(repo.repoRoot, repo.branch) { c ->
                    SitatameToolWindowContent.commentMatches(c, selected)
                }
            } catch (ex: Exception) {
                log.warn("CommentDetailPane: resolveOrReopen failed", ex)
            }
            store.invalidate()
            ApplicationManager.getApplication().invokeLater { onRefreshAll() }
        }
    }

    private fun deleteComment() {
        val selected = currentComment ?: return
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
                log.warn("CommentDetailPane: deleteComment failed", ex)
            }
            store.invalidate()
            ApplicationManager.getApplication().invokeLater { onRefreshAll() }
        }
    }
}
