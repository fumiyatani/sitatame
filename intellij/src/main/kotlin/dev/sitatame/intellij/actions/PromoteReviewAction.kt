package dev.sitatame.intellij.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.project.Project
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.ReviewStore

/**
 * Move the current draft to reviews/ and announce the path via a notification
 * so downstream tooling can pick it up (the notification text includes the
 * `SITATAME_REVIEW=<abs path>` form the CLI consumes).
 */
class PromoteReviewAction : AnAction() {

    private val log = Logger.getInstance(PromoteReviewAction::class.java)

    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val repo = RepoContext.forProject(project)
            ?: run {
                notify(project, "sitatame: no Git repository for this project", NotificationType.WARNING)
                return
            }
        val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)

        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "Promoting sitatame draft", false) {
                override fun run(indicator: ProgressIndicator) {
                    val promotedPath = try {
                        store.promoteDraft(repo.repoRoot, repo.branch)
                    } catch (ex: Exception) {
                        log.warn("PromoteReviewAction: failed to promote", ex)
                        ApplicationManager.getApplication().invokeLater {
                            notify(project, "sitatame: promote failed — ${ex.message}", NotificationType.ERROR)
                        }
                        return
                    }
                    ApplicationManager.getApplication().invokeLater {
                        if (promotedPath == null) {
                            notify(project, "sitatame: no draft to promote", NotificationType.INFORMATION)
                        } else {
                            notify(
                                project,
                                "sitatame: SITATAME_REVIEW=$promotedPath",
                                NotificationType.INFORMATION,
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
}
