package dev.sitatame.intellij.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.project.Project
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewStore

/**
 * Toggle the resolved/open state of the comment under the caret.
 *
 * Matching is by (file, line) inclusion: a Range comment matches when the
 * caret's 1-based line lies inside `[lineStart, lineEnd]`; a Line comment
 * matches when its `line` equals the caret line. If multiple comments
 * overlap, the first match wins — Phase 2 will show a chooser popup.
 */
class ResolveCommentAction : AnAction() {

    private val log = Logger.getInstance(ResolveCommentAction::class.java)

    override fun update(e: AnActionEvent) {
        val project = e.project
        val editor = e.getData(CommonDataKeys.EDITOR)
        e.presentation.isEnabledAndVisible = project != null && editor != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val editor = e.getData(CommonDataKeys.EDITOR) ?: return
        val file = e.getData(CommonDataKeys.VIRTUAL_FILE) ?: return
        val repo = RepoContext.forFile(project, file) ?: return

        val caretLine = editor.document.getLineNumber(editor.caretModel.offset) + 1
        val relPath = relativise(file.path, repo.repoRoot)
        val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)

        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "Toggling sitatame comment", false) {
                override fun run(indicator: ProgressIndicator) {
                    val result = try {
                        store.toggleComment(repo.repoRoot, repo.branch) { c -> matches(c, relPath, caretLine) }
                    } catch (ex: Exception) {
                        log.warn("ResolveCommentAction: failed to toggle", ex)
                        ApplicationManager.getApplication().invokeLater {
                            notify(project, "sitatame: toggle failed — ${ex.message}", NotificationType.ERROR)
                        }
                        return
                    }
                    ApplicationManager.getApplication().invokeLater {
                        if (result == null) {
                            notify(project, "sitatame: no comment under cursor", NotificationType.INFORMATION)
                        } else {
                            notify(project, "sitatame: toggled comment in ${result.path}", NotificationType.INFORMATION)
                        }
                    }
                }
            }
        )
    }

    private fun matches(c: Comment, relPath: String, caretLine: Int): Boolean {
        if (c.anchor.path != relPath) return false
        return when (c.anchor.kind) {
            AnchorKind.LINE -> c.anchor.line == caretLine
            AnchorKind.RANGE -> caretLine in c.anchor.lineStart..c.anchor.lineEnd
            else -> false
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
}
