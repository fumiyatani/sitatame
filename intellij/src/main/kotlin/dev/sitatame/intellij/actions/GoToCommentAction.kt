package dev.sitatame.intellij.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.ActionUpdateThread
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.LogicalPosition
import com.intellij.openapi.editor.ScrollType
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
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
 * Threading: [snapshotComments] is called on a background thread because a cold
 * cache miss triggers [ReviewStore.loadOrInit] which reads the review.md file
 * from disk. Caret movement is dispatched back to the EDT via [invokeLater],
 * mirroring [CopyAIPromptAction]'s approach. When the cache is already warm
 * (the common case after [SitatameLineMarkerProvider] has run) the background
 * hop is cheap.
 */
class GoToNextCommentAction : GoToCommentAction(forward = true)

class GoToPrevCommentAction : GoToCommentAction(forward = false)

abstract class GoToCommentAction(private val forward: Boolean) : AnAction() {

    // update() reads only project and EDITOR from DataContext — no Swing hierarchy
    // access → safe on BGT. CommonDataKeys.EDITOR resolves via DataContext without
    // touching the Swing component tree directly.
    override fun getActionUpdateThread(): ActionUpdateThread = ActionUpdateThread.BGT

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

        // Capture the caret position here on the EDT before hopping to the
        // background thread (DataContext and editor state are only valid on EDT).
        val caretLine = editor.document.getLineNumber(editor.caretModel.offset) + 1

        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "Loading sitatame comments", false) {
                override fun run(indicator: ProgressIndicator) {
                    // snapshotComments may read review.md on first access (cold cache),
                    // so it must run on a background thread to avoid EDT I/O.
                    val comments = store.snapshotComments(repo.repoRoot, repo.branch)
                    val lines = commentedLines(comments, relPath)

                    ApplicationManager.getApplication().invokeLater {
                        if (lines.isEmpty()) {
                            notify(project, "sitatame: no comments in this file", NotificationType.INFORMATION)
                            return@invokeLater
                        }

                        val target = nextLine(lines, caretLine, forward)
                        if (target == null) {
                            val where = if (forward) "after" else "before"
                            notify(project, "sitatame: no comment $where the caret", NotificationType.INFORMATION)
                            return@invokeLater
                        }

                        moveCaretTo(editor, target)
                    }
                }
            }
        )
    }

    private fun moveCaretTo(editor: Editor, line1Based: Int) {
        val zeroBased = clampLine(line1Based, editor.document.lineCount)
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

        /**
         * Clamp a 1-based target [line] into a valid 0-based document row index.
         *
         * The IntelliJ [Editor.document.lineCount] is 0 when the document is empty
         * (lineCount == 0 → the only valid row is 0 via coerceAtLeast). For a
         * normal document the valid range is 0..(lineCount - 1).
         *
         * Pure function — testable without the IntelliJ Platform. [moveCaretTo]
         * applies this then hands the result to [LogicalPosition].
         */
        fun clampLine(line1Based: Int, lineCount: Int): Int =
            (line1Based - 1).coerceIn(0, (lineCount - 1).coerceAtLeast(0))
    }
}
