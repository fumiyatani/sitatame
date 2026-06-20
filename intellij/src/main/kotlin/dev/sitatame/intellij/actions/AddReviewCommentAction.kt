package dev.sitatame.intellij.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.ActionUpdateThread
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
 * The review comment is stored in `review.reviewComment` (the top-level
 * `review_comment` field in review.md), NOT in `comments[]`. This matches the
 * Go TUI's `confirmModal` for `KindReview` which sets
 * `m.Review.ReviewComment = body` and does NOT append to comments[]. Any
 * previous review comment is overwritten (same in-place edit semantics as the
 * Go TUI's Shift+R, which pre-loads the existing text for editing).
 *
 * The dialog pre-loads the existing review comment (if any) so the user can
 * edit in place, matching `openReviewModal` in modal.go:206.
 *
 * The comment is not pinned to a file; it surfaces in the tool window comment
 * list (with a "(review)" locator) but not in editor gutters or inlays.
 *
 * Threading mirrors [AddCommentAction]: [actionPerformed] is on the EDT, the
 * modal stays on the EDT, and the YAML write hops to a background thread via
 * [ReviewStore.setReviewComment].
 */
class AddReviewCommentAction : AnAction() {

    private val log = Logger.getInstance(AddReviewCommentAction::class.java)

    // update() only reads e.project (not Swing hierarchy) → safe on BGT.
    override fun getActionUpdateThread(): ActionUpdateThread = ActionUpdateThread.BGT

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

        val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)

        // Background: load existing review comment (cold cache may read disk).
        // After loading, hop back to the EDT to display the dialog.
        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "Loading sitatame review comment", false) {
                override fun run(indicator: ProgressIndicator) {
                    // getReviewComment calls loadOrInit which may do Files.readAllBytes
                    // on a cold cache. Keep this off the EDT.
                    val existing = store.getReviewComment(repo.repoRoot, repo.branch)

                    ApplicationManager.getApplication().invokeLater {
                        val dialog = ReviewCommentDialog(project, existing)
                        if (!dialog.showAndGet()) return@invokeLater
                        // Mirrors Go confirmModal: strings.TrimRight(body, "\n").
                        // trimEnd preserves leading/internal whitespace; only trailing
                        // newlines (and \r) are stripped, matching the TUI's behaviour.
                        val body = dialog.body.trimEnd('\n', '\r')

                        // #2: allow clearing an existing comment with an empty body.
                        // Only no-op when there was no previous comment AND the new
                        // body is also blank (truly nothing to do).
                        if (existing.isEmpty() && body.isEmpty()) return@invokeLater

                        ProgressManager.getInstance().run(
                            object : Task.Backgroundable(project, "Saving sitatame review comment", false) {
                                override fun run(indicator: ProgressIndicator) {
                                    try {
                                        // Store as review_comment (top-level scalar), not in
                                        // comments[]. Mirrors Go: m.Review.ReviewComment = body.
                                        val result = store.setReviewComment(repo.repoRoot, repo.branch, body)
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

    private class ReviewCommentDialog(project: Project, existing: String) :
        DialogWrapper(project, true) {

        private val textArea = JTextArea(8, 60).apply {
            lineWrap = true
            wrapStyleWord = true
            // Pre-load the existing review comment so the user edits in place,
            // matching Go's openReviewModal which pre-loads m.Review.ReviewComment.
            text = existing
        }

        init {
            title = "Edit sitatame review-level comment"
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
