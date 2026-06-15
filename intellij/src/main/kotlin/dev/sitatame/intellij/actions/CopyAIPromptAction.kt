package dev.sitatame.intellij.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.DataKey
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.ide.CopyPasteManager
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.ui.components.JBScrollPane
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore
import java.awt.BorderLayout
import java.awt.Dimension
import java.awt.datatransfer.StringSelection
import javax.swing.JComponent
import javax.swing.JPanel
import javax.swing.JTextArea

/**
 * Build an AI-ready prompt summarising the open / selected comments and copy
 * it to the clipboard. The format mirrors what the CLI's `sitatame ai-prompt`
 * subcommand emits so an LLM hooked up to either workflow sees the same
 * instructions.
 *
 * Threading: [actionPerformed] runs on the EDT (Action framework contract).
 * The file I/O ([ReviewStore.snapshotComments]) and prompt building happen on
 * a background thread via [Task.Backgroundable], mirroring [AddCommentAction].
 * Results are handed back to the EDT via [ApplicationManager.invokeLater] for
 * clipboard write and the preview dialog. This avoids Slow Operations
 * Assertion violations (IntelliJ 2024.2+) on cold-cache / large review.md.
 */
class CopyAIPromptAction : AnAction() {

    private val log = Logger.getInstance(CopyAIPromptAction::class.java)

    companion object {
        /**
         * Tool window passes selected comments via a custom DataKey so this
         * action doesn't have to peek into the JBList from here.
         */
        val SELECTED_COMMENTS_KEY: DataKey<List<Comment>> =
            DataKey.create("sitatame.selectedComments")
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val repo = RepoContext.forProject(project)
            ?: run {
                notify(project, "sitatame: no Git repository detected for this project", NotificationType.WARNING)
                return
            }

        // Snapshot selected comments early on EDT (DataContext is only valid here).
        val selected = e.getData(SELECTED_COMMENTS_KEY).orEmpty()

        val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)

        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "Building AI prompt…", false) {
                override fun run(indicator: ProgressIndicator) {
                    try {
                        // Background: file I/O + prompt construction.
                        val all = store.snapshotComments(repo.repoRoot, repo.branch)
                        val targets = when {
                            selected.isNotEmpty() -> selected
                            else -> all.filter { it.state == ReviewState.OPEN }
                        }

                        ApplicationManager.getApplication().invokeLater {
                            if (targets.isEmpty()) {
                                notify(project, "sitatame: no open comments to copy", NotificationType.INFORMATION)
                                return@invokeLater
                            }
                            val prompt = buildPrompt(targets)
                            CopyPasteManager.getInstance().setContents(StringSelection(prompt))
                            showPreview(project, prompt)
                        }
                    } catch (ex: Exception) {
                        log.warn("CopyAIPromptAction: failed to load comments", ex)
                        ApplicationManager.getApplication().invokeLater {
                            notify(
                                project,
                                "sitatame: failed to build AI prompt — ${ex.message}",
                                NotificationType.ERROR,
                            )
                        }
                    }
                }
            }
        )
    }

    /**
     * The prompt format. Two sections so the LLM has both context and the
     * actionable list:
     *   - leading instructions
     *   - per-comment blocks with anchor + body
     *   - trailing "related diff (summary)" placeholder (Phase 2 will fill it
     *     with `git diff --stat` from Git4Idea).
     */
    fun buildPrompt(comments: List<Comment>): String = buildString {
        append("以下の修正指示に従って、対象ファイルを直してください。\n\n")
        for (c in comments) {
            val anchor = c.anchor
            val locator = when (anchor.kind) {
                AnchorKind.RANGE -> "${anchor.path}:${anchor.lineStart}-${anchor.lineEnd}"
                AnchorKind.LINE -> "${anchor.path}:${anchor.line}"
                else -> anchor.path
            }
            append("[${anchor.anchorId}] $locator (${anchor.kind}, ${c.state})\n")
            for (line in c.body.lines()) {
                append("> ").append(line).append('\n')
            }
            append('\n')
        }
        append("関連 diff (要約):\n")
        append("(`git diff --stat` をここに貼り付けてください)\n")
    }

    private fun notify(project: Project, content: String, type: NotificationType) {
        NotificationGroupManager.getInstance()
            .getNotificationGroup("sitatame.review")
            .createNotification(content, type)
            .notify(project)
    }

    private fun showPreview(project: Project, prompt: String) {
        PromptPreviewDialog(project, prompt).show()
    }

    private class PromptPreviewDialog(project: Project, private val prompt: String) :
        DialogWrapper(project, true) {

        init {
            title = "sitatame: AI prompt copied to clipboard"
            init()
        }

        override fun createCenterPanel(): JComponent {
            val area = JTextArea(prompt).apply {
                isEditable = false
                lineWrap = false
            }
            val panel = JPanel(BorderLayout())
            panel.add(JBScrollPane(area), BorderLayout.CENTER)
            panel.preferredSize = Dimension(720, 480)
            return panel
        }
    }
}
