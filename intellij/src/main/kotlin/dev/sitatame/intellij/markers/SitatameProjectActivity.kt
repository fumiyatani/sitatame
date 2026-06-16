package dev.sitatame.intellij.markers

import com.intellij.codeInsight.daemon.DaemonCodeAnalyzer
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.startup.ProjectActivity
import com.intellij.openapi.vfs.VirtualFileManager
import dev.sitatame.intellij.listeners.buildGitListener
import dev.sitatame.intellij.listeners.buildVfsListener
import git4idea.repo.GitRepository

/**
 * Subscribes to [ReviewChangedTopic] at project startup and triggers
 * [DaemonCodeAnalyzer.restart] on all currently open files whenever the
 * review store is mutated.
 *
 * Also registers:
 * - [dev.sitatame.intellij.listeners.SitatameVfsListener]: reacts to external
 *   edits of `review.md` files (TUI, other editors, manual edits) so the tool
 *   window refreshes without a manual Refresh click.
 * - [dev.sitatame.intellij.listeners.SitatameGitListener]: reacts to branch
 *   switches so the tool window automatically shows the new branch's review.
 *
 * Threading:
 * - The MessageBus delivery is synchronous on the publishing thread (EDT in the
 *   action layer). We jump to EDT explicitly via invokeLater so
 *   DaemonCodeAnalyzer.restart is always called on the EDT, matching the
 *   2024.2+ threading model.
 *
 * Lifecycle:
 * - The MessageBusConnection is scoped to the project's coroutine scope via
 *   [ProjectActivity.execute], which means it is disposed when the project
 *   closes. No manual disconnect is needed.
 * - The VFS listener is also registered with the project's coroutine-scope
 *   Disposable so it is automatically removed on project close.
 */
class SitatameProjectActivity : ProjectActivity {

    override suspend fun execute(project: Project) {
        val connection = project.messageBus.connect()

        // (1) Daemon restart on review mutations (existing behaviour).
        connection.subscribe(ReviewChangedTopic.TOPIC, ReviewChangedListener {
            // Restart highlighting for all open editors so gutter icons refresh.
            ApplicationManager.getApplication().invokeLater {
                if (project.isDisposed) return@invokeLater
                val analyzer = DaemonCodeAnalyzer.getInstance(project)
                val fem = FileEditorManager.getInstance(project)
                for (editor in fem.allEditors) {
                    val psiFile = com.intellij.psi.PsiManager.getInstance(project)
                        .findFile(editor.file ?: continue) ?: continue
                    analyzer.restart(psiFile)
                }
            }
        })

        // (2) VFS listener: detect external review.md edits (TUI / other IDE / hand-edit).
        // Bound to the project's coroutine-scope Disposable (project itself implements
        // Disposable) so it is removed automatically on project close.
        VirtualFileManager.getInstance().addAsyncFileListener(
            buildVfsListener(),
            project,  // Disposable — project implements com.intellij.openapi.Disposable
        )

        // (3) Git branch-switch listener: refresh when HEAD moves to another branch.
        connection.subscribe(GitRepository.GIT_REPO_CHANGE, buildGitListener())
    }
}
