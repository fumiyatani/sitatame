package dev.sitatame.intellij.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.ActionUpdateThread
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.ui.components.JBScrollPane
import dev.sitatame.intellij.git.BlobResolver
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.markers.ReviewChangedTopic
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
 * Authors a *file-level* sitatame comment — a remark scoped to a whole file
 * rather than a specific line. Mirrors the Go TUI's behaviour of pressing `c`
 * while the cursor is on a file header, which the IntelliJ plugin previously
 * had no way to produce: [AddCommentAction] only emits LINE / RANGE anchors.
 *
 * The anchor carries a path but no line, so the comment is associated with the
 * file (and surfaces in the changed-files / comment-list panes filtered to that
 * file) but is not pinned to a gutter line.
 *
 * Threading mirrors [AddCommentAction]: [actionPerformed] is on the EDT, the
 * modal stays on the EDT, and the YAML write (plus blob SHA resolution) hops to
 * a background thread.
 */
class AddFileCommentAction : AnAction() {

    private val log = Logger.getInstance(AddFileCommentAction::class.java)

    // update() reads only project and VIRTUAL_FILE from DataContext — no Swing
    // hierarchy access → safe on BGT.
    override fun getActionUpdateThread(): ActionUpdateThread = ActionUpdateThread.BGT

    override fun update(e: AnActionEvent) {
        val project = e.project
        val file = e.getData(CommonDataKeys.VIRTUAL_FILE)
        e.presentation.isEnabledAndVisible = project != null && file != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
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

        val relPath = relativise(file.path, repo.repoRoot)

        val dialog = FileCommentDialog(project, relPath)
        if (!dialog.showAndGet()) return
        val body = dialog.body
        if (body.isBlank()) return

        val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)

        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "Saving sitatame file comment", false) {
                override fun run(indicator: ProgressIndicator) {
                    // Mirrors Go's fileScopeSide (modal.go:136-141): deleted files
                    // have no head blob, so the FILE anchor must use SideBase.
                    // Using SideHead for a deleted file would produce an empty blob
                    // that Go's validateAnchor immediately marks stale.
                    val isDeleted = BlobResolver.isDeletedFromIndex(repo.repoRoot, relPath)
                    val side = if (isDeleted) AnchorSide.BASE else AnchorSide.HEAD
                    val blob = if (isDeleted) {
                        // Known limitation: deleted-file FILE comments are always
                        // stale in Go/TUI. Go's validateAnchor (internal/review/
                        // validate.go:216-250) requires SideBase + non-empty blob for
                        // a deleted-file FILE anchor. Resolving the base blob SHA
                        // (git diff --raw <baseRef>..HEAD to obtain the source SHA)
                        // is not yet implemented; this empty string causes Go to mark
                        // the anchor stale on every load. Tracked as a known gap —
                        // base blob resolution is future work.
                        ""
                    } else {
                        if (relPath.isNotEmpty()) BlobResolver.headBlobSha(repo.repoRoot, relPath) else ""
                    }
                    val anchor = Anchor(
                        kind = AnchorKind.FILE,
                        path = relPath,
                        side = side,
                        blob = blob,
                    )
                    try {
                        val result = store.addComment(repo.repoRoot, repo.branch) { _ ->
                            Comment(
                                anchor = anchor,
                                state = ReviewState.OPEN,
                                body = body.trim(),
                            )
                        }
                        if (result.succeeded) {
                            ApplicationManager.getApplication().messageBus
                                .syncPublisher(ReviewChangedTopic.TOPIC)
                                .reviewChanged()
                            ApplicationManager.getApplication().invokeLater {
                                notify(
                                    project,
                                    "sitatame: saved file comment to ${result.path}",
                                    NotificationType.INFORMATION,
                                )
                            }
                        } else {
                            val rescuePath = result.error?.rescuePath ?: ""
                            log.warn("AddFileCommentAction: encode failed; rescue at $rescuePath")
                            ApplicationManager.getApplication().invokeLater {
                                notify(
                                    project,
                                    "sitatame: failed to save file comment — encode error" +
                                        (if (rescuePath.isNotEmpty()) "; rescue written to $rescuePath" else ""),
                                    NotificationType.ERROR,
                                )
                            }
                        }
                    } catch (ex: Exception) {
                        log.warn("AddFileCommentAction: failed to persist", ex)
                        ApplicationManager.getApplication().invokeLater {
                            notify(
                                project,
                                "sitatame: failed to save file comment — ${ex.message}",
                                NotificationType.ERROR,
                            )
                        }
                    }
                }
            }
        )
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

    private class FileCommentDialog(project: Project, private val relPath: String) :
        DialogWrapper(project, true) {

        private val textArea = JTextArea(8, 60).apply {
            lineWrap = true
            wrapStyleWord = true
        }

        init {
            title = "Add sitatame file-level comment"
            init()
        }

        val body: String get() = textArea.text

        override fun createCenterPanel(): JComponent {
            val panel = JPanel(BorderLayout())
            panel.add(JLabel("File: $relPath (file-level comment)"), BorderLayout.NORTH)
            panel.add(JBScrollPane(textArea), BorderLayout.CENTER)
            panel.preferredSize = Dimension(560, 320)
            return panel
        }

        override fun getPreferredFocusedComponent(): JComponent = textArea
    }
}
