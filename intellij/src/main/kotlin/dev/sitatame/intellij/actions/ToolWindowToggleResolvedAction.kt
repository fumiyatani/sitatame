package dev.sitatame.intellij.actions

import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.DataKey

/**
 * Toggle the resolved/open state of the comment currently selected in the
 * Sitatame tool window.
 *
 * This action is registered in plugin.xml without a default keyboard shortcut
 * so users can bind it freely via **Settings → Keymap → "Sitatame: Toggle
 * Resolved (Tool Window)"**. Space is the suggested binding documented in the
 * tool window help text.
 *
 * The tool window wires itself via [TOGGLE_SELECTED_KEY]: it registers a
 * [Runnable] through its [DataProvider][com.intellij.openapi.actionSystem.DataProvider]
 * that calls [SitatameToolWindowContent.toggleSelected] when invoked.
 */
class ToolWindowToggleResolvedAction : AnAction() {

    companion object {
        /**
         * DataKey used by the Sitatame tool window to expose a [Runnable] that
         * calls `toggleSelected()` on the current selection.
         *
         * The tool window's outer panel implements [DataProvider] and returns a
         * [Runnable] for this key; [ToolWindowToggleResolvedAction] invokes it
         * so it never touches the tool window internals directly.
         */
        val TOGGLE_SELECTED_KEY: DataKey<Runnable> =
            DataKey.create("sitatame.toolWindowToggleSelected")
    }

    override fun update(e: AnActionEvent) {
        e.presentation.isEnabled = e.getData(TOGGLE_SELECTED_KEY) != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        e.getData(TOGGLE_SELECTED_KEY)?.run()
    }
}
