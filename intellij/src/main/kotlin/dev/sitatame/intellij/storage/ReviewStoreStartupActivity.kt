package dev.sitatame.intellij.storage

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.service
import com.intellij.openapi.project.Project
import com.intellij.openapi.startup.ProjectActivity
import git4idea.repo.GitRepositoryManager

/**
 * Calls [ReviewStore.recoverFromCrash] once per project open.
 *
 * This wires the recovery path that was previously never called (issue #106
 * dead-recoverFromCrash defect). It promotes any orphaned `review.md.bak`
 * to `review.md` and deletes leftover `.tmp` files that accumulated from
 * interrupted write cycles.
 *
 * Threading:
 * - [ProjectActivity.execute] is called on a background thread by the
 *   platform (2024.2+ ProjectActivity contract), which is correct because
 *   [ReviewStore.recoverFromCrash] does file I/O and must not run on EDT.
 * - The balloon notification is dispatched to EDT via [invokeLater].
 *
 * Lifecycle:
 * - The activity runs once when the project is opened. No persistent state.
 *   If no Git repository is found the activity is a no-op.
 */
class ReviewStoreStartupActivity : ProjectActivity {

    override suspend fun execute(project: Project) {
        if (project.isDisposed) return

        val store = ApplicationManager.getApplication().service<ReviewStore>()
        val gitManager = GitRepositoryManager.getInstance(project)
        val repos = gitManager.repositories
        if (repos.isEmpty()) return

        var recovered = false
        for (repo in repos) {
            val repoRoot = repo.root.path
            // Use the current branch; fall back to HEAD ref name if detached.
            val branch = repo.currentBranchName ?: repo.currentRevision ?: continue
            // recoverFromCrash returns true only when review.md was absent and
            // .bak was actually promoted — a genuine crash recovery. Normal saves
            // leave .bak alongside review.md, so checking .bak existence alone
            // (as before) fired the notification on every startup after the first
            // save.
            if (store.recoverFromCrash(repoRoot, branch)) recovered = true
        }

        if (recovered) {
            com.intellij.openapi.application.ApplicationManager.getApplication().invokeLater {
                if (project.isDisposed) return@invokeLater
                NotificationGroupManager.getInstance()
                    .getNotificationGroup("sitatame.review")
                    ?.createNotification(
                        "sitatame: review recovered",
                        "A previous write was incomplete. review.md has been restored from backup.",
                        NotificationType.INFORMATION,
                    )
                    ?.notify(project)
            }
        }
    }
}
