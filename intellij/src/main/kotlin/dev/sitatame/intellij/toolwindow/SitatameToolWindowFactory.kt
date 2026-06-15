package dev.sitatame.intellij.toolwindow

import com.intellij.openapi.Disposable
import com.intellij.openapi.project.DumbAware
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.content.ContentFactory

/**
 * Registers the right-anchored sitatame review tool window. Listing /
 * navigation / copy-AI-prompt all live inside
 * [SitatameToolWindowContent].
 */
class SitatameToolWindowFactory : ToolWindowFactory, DumbAware {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        // toolWindow implements Disposable; we use it as the lifecycle scope for
        // the MessageBus connection so the subscriber is cleaned up automatically
        // when the tool window is closed/disposed.
        val content = SitatameToolWindowContent(project, toolWindow as Disposable)
        val contentFactory = ContentFactory.getInstance()
        val container = contentFactory.createContent(content.component, "Comments", false)
        toolWindow.contentManager.addContent(container)
    }
}
