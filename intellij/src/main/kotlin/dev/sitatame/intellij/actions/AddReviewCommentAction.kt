package dev.sitatame.intellij.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.ui.components.JBScrollPane
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
 * Authors a *review-level* sitatame comment — a remark about the change as a
 * whole rather than a specific line. Mirrors the Go TUI's `Shift+R`
 * ("review-level comment") binding, which the IntelliJ plugin previously had
 * no way to produce: [AddCommentAction] only emits LINE / RANGE anchors.
 *
 * The anchor carries no path or line, so the comment is not pinned to a file;
 * it surfaces in the tool window comment list (with a "(review)" locator) but
 * not in editor gutters or inlays.
 *
 * Threading mirrors [AddCommentAction]: [actionPerformed] is on the EDT, the
 * modal stays on the EDT, and the YAML write hops to a background thread.
 */
class AddReviewCommentAction : AnAction() {

    private val log = Logger.getInstance(AddReviewCommentAction::class.java)

    override fun update(e: AnActionEvent) {
        // Review-level comments are not tied to an editor, so the action is
        // enabled whenever a project is open.
        e.presentation.isEnabledAndVisible = e.project != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return

        val repo = RepoContext.forProject(project)
            ?: run {
                val message = if (RepoContext.hasNoResolvableRef(project)) {
                    "sitatame: Git operation in progress, please retry after completion"
                } else {
                    "sitatame: project is not in a Git repository"
                }
                notify(project, message, NotificationType.WARNING)
                return
            }

        val dialog = ReviewCommentDialog(project)
        if (!dialog.showAndGet()) return
        val body = dialog.body
        if (body.isBlank()) return

        val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)

        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "Saving sitatame review comment", false) {
                override fun run(indicator: ProgressIndicator) {
                    try {
                        val result = store.addComment(repo.repoRoot, repo.branch) { _ ->
                            Comment(
                                anchor = Anchor(
                                    kind = AnchorKind.REVIEW,
                                    side = AnchorSide.HEAD,
                                ),
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
                                    "sitatame: saved review comment to ${result.path}",
                                    NotificationType.INFORMATION,
                                )
                            }
                        } else {
                            val rescuePath = result.error?.rescuePath ?: ""
                            log.warn("AddReviewCommentAction: encode failed; rescue at $rescuePath")
                            ApplicationManager.getApplication().invokeLater {
                                notify(
                                    project,
                                    "sitatame: failed to save review comment — encode error" +
                                        (if (rescuePath.isNotEmpty()) "; rescue written to $rescuePath" else ""),
                                    NotificationType.ERROR,
                                )
                            }
                        }
                    } catch (ex: Exception) {
                        log.warn("AddReviewCommentAction: failed to persist", ex)
                        ApplicationManager.getApplication().invokeLater {
                            notify(
                                project,
                                "sitatame: failed to save review comment — ${ex.message}",
                                NotificationType.ERROR,
                            )
                        }
                    }
                }
            }
        )
    }

    private fun notify(project: Project, content: String, type: NotificationType) {
        NotificationGroupManager.getInstance()
            .getNotificationGroup("sitatame.review")
            .createNotification(content, type)
            .notify(project)
    }

    private class ReviewCommentDialog(project: Project) : DialogWrapper(project, true) {

        private val textArea = JTextArea(8, 60).apply {
            lineWrap = true
            wrapStyleWord = true
        }

        init {
            title = "Add sitatame review-level comment"
            init()
        }

        val body: String get() = textArea.text

        override fun createCenterPanel(): JComponent {
            val panel = JPanel(BorderLayout())
            panel.add(JLabel("Review-level comment (applies to the whole change)"), BorderLayout.NORTH)
            panel.add(JBScrollPane(textArea), BorderLayout.CENTER)
            panel.preferredSize = Dimension(560, 320)
            return panel
        }

        override fun getPreferredFocusedComponent(): JComponent = textArea
    }
}
