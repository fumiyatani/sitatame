package dev.sitatame.intellij.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.ui.components.JBScrollPane
import dev.sitatame.intellij.git.BlobResolver
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.AnchorSide
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore
import java.awt.BorderLayout
import java.awt.Dimension
import javax.swing.JComponent
import javax.swing.JLabel
import javax.swing.JPanel
import javax.swing.JTextArea

/**
 * Captures a sitatame review comment for the current Editor selection / caret
 * line. Triggered from the editor popup menu and via Cmd+Shift+C.
 *
 * Threading: [actionPerformed] runs on the EDT (Action framework contract).
 * The modal dialog stays on EDT; the YAML write happens on a background
 * thread via [Task.Backgroundable] and the success/error toast hops back to
 * EDT through [ApplicationManager.invokeLater]. This matches the 2024.2+
 * threading model (no slow I/O on EDT) without pulling in coroutines, which
 * keeps the runtime classpath narrow.
 */
class AddCommentAction : AnAction() {

    private val log = Logger.getInstance(AddCommentAction::class.java)

    override fun update(e: AnActionEvent) {
        val project = e.project
        val editor = e.getData(CommonDataKeys.EDITOR)
        e.presentation.isEnabledAndVisible = project != null && editor != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val editor = e.getData(CommonDataKeys.EDITOR) ?: return
        val file = e.getData(CommonDataKeys.VIRTUAL_FILE) ?: return

        val repo = RepoContext.forFile(project, file)
            ?: run {
                val message = if (RepoContext.hasNoResolvableRef(project)) {
                    "sitatame: Git operation in progress, please retry after completion"
                } else {
                    "sitatame: file is not in a Git repository"
                }
                notify(project, message, NotificationType.WARNING)
                return
            }

        val anchor = anchorFor(editor, file, repo.repoRoot)
        val dialog = CommentDialog(project, anchor)
        if (!dialog.showAndGet()) return
        val body = dialog.body
        if (body.isBlank()) return

        val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)

        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "Saving sitatame comment", false) {
                override fun run(indicator: ProgressIndicator) {
                    // Populate blob SHA on the background thread to avoid EDT I/O.
                    // BlobResolver shells out to `git ls-files -s`; failures are
                    // silent (blob stays empty, stale detection degrades gracefully).
                    if (anchor.blob.isEmpty() && anchor.path.isNotEmpty()) {
                        anchor.blob = BlobResolver.headBlobSha(repo.repoRoot, anchor.path)
                    }
                    try {
                        val result = store.addComment(repo.repoRoot, repo.branch) { _ ->
                            Comment(
                                anchor = anchor,
                                state = ReviewState.OPEN,
                                body = body.trim(),
                            )
                        }
                        ApplicationManager.getApplication().invokeLater {
                            notify(
                                project,
                                "sitatame: saved comment to ${result.path}",
                                NotificationType.INFORMATION,
                            )
                        }
                    } catch (ex: Exception) {
                        log.warn("AddCommentAction: failed to persist draft", ex)
                        ApplicationManager.getApplication().invokeLater {
                            notify(
                                project,
                                "sitatame: failed to save comment — ${ex.message}",
                                NotificationType.ERROR,
                            )
                        }
                    }
                }
            }
        )
    }

    /**
     * Build an anchor from the editor state. Selection present → range,
     * no selection → line. Paths are stored repo-relative so the same review
     * file works across machines.
     */
    private fun anchorFor(editor: Editor, file: VirtualFile, repoRoot: String): Anchor {
        val doc = editor.document
        val selectionModel = editor.selectionModel
        val relPath = relativise(file.path, repoRoot)
        return if (selectionModel.hasSelection()) {
            val startLine = doc.getLineNumber(selectionModel.selectionStart) + 1
            val endLine = doc.getLineNumber(selectionModel.selectionEnd - 1).coerceAtLeast(0) + 1
            Anchor(
                kind = AnchorKind.RANGE,
                path = relPath,
                side = AnchorSide.HEAD,
                lineStart = startLine,
                lineEnd = endLine,
            )
        } else {
            val line = doc.getLineNumber(editor.caretModel.offset) + 1
            Anchor(
                kind = AnchorKind.LINE,
                path = relPath,
                side = AnchorSide.HEAD,
                line = line,
            )
        }
    }

    private fun relativise(absPath: String, repoRoot: String): String {
        if (repoRoot.isEmpty()) return absPath
        val prefix = if (repoRoot.endsWith('/')) repoRoot else "$repoRoot/"
        return if (absPath.startsWith(prefix)) absPath.removePrefix(prefix) else absPath
    }

    private fun notify(project: Project, content: String, type: NotificationType) {
        NotificationGroupManager.getInstance()
            .getNotificationGroup("sitatame.review")
            .createNotification(content, type)
            .notify(project)
    }

    /**
     * Simple modal dialog that asks the reviewer for a comment body. Kept
     * deliberately minimal — Phase 2 might add Markdown preview, snippet
     * insertion, etc.
     */
    private class CommentDialog(project: Project, private val anchor: Anchor) :
        DialogWrapper(project, true) {

        private val textArea = JTextArea(8, 60).apply {
            lineWrap = true
            wrapStyleWord = true
        }

        init {
            title = "Add sitatame review comment"
            init()
        }

        val body: String get() = textArea.text

        override fun createCenterPanel(): JComponent {
            val panel = JPanel(BorderLayout())
            val anchorDesc = when (anchor.kind) {
                AnchorKind.RANGE -> "${anchor.path}:${anchor.lineStart}-${anchor.lineEnd} (range)"
                else -> "${anchor.path}:${anchor.line} (line)"
            }
            panel.add(JLabel("Anchor: $anchorDesc"), BorderLayout.NORTH)
            panel.add(JBScrollPane(textArea), BorderLayout.CENTER)
            panel.preferredSize = Dimension(560, 320)
            return panel
        }

        override fun getPreferredFocusedComponent(): JComponent = textArea
    }
}
