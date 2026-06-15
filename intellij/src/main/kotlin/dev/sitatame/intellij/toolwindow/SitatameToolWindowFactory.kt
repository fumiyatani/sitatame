package dev.sitatame.intellij.toolwindow

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
        // Use ToolWindow.disposable (the platform-provided Disposable for this
        // tool window) rather than casting toolWindow to Disposable directly.
        // ToolWindow does not extend Disposable in the public SDK contract, so
        // the cast would throw ClassCastException on some platform versions.
        val content = SitatameToolWindowContent(project, toolWindow.disposable)
        val contentFactory = ContentFactory.getInstance()
        val container = contentFactory.createContent(content.component, "Comments", false)
        toolWindow.contentManager.addContent(container)
    }
}
