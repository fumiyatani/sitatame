package dev.sitatame.intellij.markers

import com.intellij.codeInsight.daemon.DaemonCodeAnalyzer
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.startup.ProjectActivity

/**
 * Subscribes to [ReviewChangedTopic] at project startup and triggers
 * [DaemonCodeAnalyzer.restart] on all currently open files whenever the
 * review store is mutated.
 *
 * Threading:
 * - The MessageBus deliver is synchronous on the publishing thread (EDT in the
 *   action layer). We jump to EDT explicitly via invokeLater so
 *   DaemonCodeAnalyzer.restart is always called on the EDT, matching the
 *   2024.2+ threading model.
 *
 * Lifecycle:
 * - The MessageBusConnection is scoped to the project's coroutine scope via
 *   [ProjectActivity.execute], which means it is disposed when the project
 *   closes. No manual disconnect is needed.
 */
class SitatameProjectActivity : ProjectActivity {

    override suspend fun execute(project: Project) {
        val connection = project.messageBus.connect()
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
    }
}
