package dev.sitatame.intellij.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.LogicalPosition
import com.intellij.openapi.editor.ScrollType
import com.intellij.openapi.project.Project
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewStore

/**
 * Moves the editor caret to the next / previous sitatame-commented line in the
 * current file. Mirrors the Go TUI's keyboard-driven comment traversal, which
 * lets a reviewer step through commented lines without leaving the editor for
 * the tool window.
 *
 * Only LINE and RANGE anchors carry a concrete line; FILE / REVIEW comments are
 * skipped because they have no caret target. A RANGE anchor's [Anchor.lineStart]
 * is used as its representative line.
 *
 * Threading: comment lookup reads the in-memory [ReviewStore] cache (no
 * subprocess), so it stays on the EDT alongside the editor caret move.
 */
class GoToNextCommentAction : GoToCommentAction(forward = true)

class GoToPrevCommentAction : GoToCommentAction(forward = false)

abstract class GoToCommentAction(private val forward: Boolean) : AnAction() {

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

        val relPath = relativise(file.path, repo.repoRoot)
        val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)
        val comments = store.snapshotComments(repo.repoRoot, repo.branch)

        val lines = commentedLines(comments, relPath)
        if (lines.isEmpty()) {
            notify(project, "sitatame: no comments in this file", NotificationType.INFORMATION)
            return
        }

        val caretLine = editor.document.getLineNumber(editor.caretModel.offset) + 1
        val target = nextLine(lines, caretLine, forward)
        if (target == null) {
            val where = if (forward) "after" else "before"
            notify(project, "sitatame: no comment $where the caret", NotificationType.INFORMATION)
            return
        }

        moveCaretTo(editor, target)
    }

    private fun moveCaretTo(editor: Editor, line1Based: Int) {
        val zeroBased = (line1Based - 1).coerceIn(0, (editor.document.lineCount - 1).coerceAtLeast(0))
        editor.caretModel.moveToLogicalPosition(LogicalPosition(zeroBased, 0))
        editor.scrollingModel.scrollToCaret(ScrollType.CENTER)
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

    companion object {

        /**
         * Sorted, de-duplicated list of 1-based lines that carry a LINE or RANGE
         * comment anchored to [relPath]. RANGE comments contribute their
         * [Anchor.lineStart]. FILE / REVIEW comments are excluded.
         *
         * Pure function — testable without the IntelliJ Platform.
         */
        fun commentedLines(comments: List<Comment>, relPath: String): List<Int> =
            comments.asSequence()
                .filter { it.anchor.path == relPath }
                .mapNotNull { c ->
                    when (c.anchor.kind) {
                        AnchorKind.LINE -> c.anchor.line.takeIf { it > 0 }
                        AnchorKind.RANGE -> c.anchor.lineStart.takeIf { it > 0 }
                        else -> null
                    }
                }
                .toSortedSet()
                .toList()

        /**
         * Given the sorted [lines] and the current [caretLine] (1-based), return
         * the next commented line strictly after the caret (when [forward]) or
         * strictly before it (when not forward), or null if there is none in that
         * direction. "Strictly" so repeated invocations advance instead of
         * sticking on the line the caret already sits on.
         *
         * Pure function — testable without the IntelliJ Platform.
         */
        fun nextLine(lines: List<Int>, caretLine: Int, forward: Boolean): Int? =
            if (forward) {
                lines.firstOrNull { it > caretLine }
            } else {
                lines.lastOrNull { it < caretLine }
            }
    }
}
