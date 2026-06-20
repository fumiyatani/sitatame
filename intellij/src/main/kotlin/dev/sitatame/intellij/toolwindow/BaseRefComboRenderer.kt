package dev.sitatame.intellij.toolwindow

import com.intellij.icons.AllIcons
import com.intellij.ui.components.JBLabel
import com.intellij.util.ui.JBUI
import com.intellij.util.ui.UIUtil
import dev.sitatame.intellij.git.BaseRefDiscovery
import java.awt.Component
import java.awt.Font
import javax.swing.JList
import javax.swing.ListCellRenderer

/**
 * Renders [BaseRefDiscovery.Entry] rows in the base ref selector: group headers
 * as muted, bold captions and branches as normal selectable rows with a branch
 * icon. Header rows are never selected by the combo (the tool window's item
 * listener ignores and reverts them), so they only ever appear inside the
 * popup, not as the closed-combo value.
 */
class BaseRefComboRenderer : ListCellRenderer<BaseRefDiscovery.Entry> {

    private val label = JBLabel()

    override fun getListCellRendererComponent(
        list: JList<out BaseRefDiscovery.Entry>,
        value: BaseRefDiscovery.Entry?,
        index: Int,
        isSelected: Boolean,
        cellHasFocus: Boolean,
    ): Component {
        when (value) {
            is BaseRefDiscovery.Entry.Header -> {
                label.text = value.title
                label.icon = null
                label.font = label.font.deriveFont(Font.BOLD)
                label.foreground = UIUtil.getContextHelpForeground()
                label.background = list.background
                label.isOpaque = true
                // Headers are not selectable; keep them visually flush-left.
                label.border = JBUI.Borders.empty(4, 6, 2, 6)
            }

            is BaseRefDiscovery.Entry.Branch -> {
                label.text = value.ref
                label.icon = AllIcons.Vcs.Branch
                label.font = label.font.deriveFont(Font.PLAIN)
                label.foreground = if (isSelected) list.selectionForeground else list.foreground
                label.background = if (isSelected) list.selectionBackground else list.background
                label.isOpaque = true
                // Indent branches under their group header inside the popup.
                label.border = JBUI.Borders.empty(2, if (index >= 0) 18 else 2, 2, 6)
            }

            null -> {
                label.text = ""
                label.icon = null
                label.isOpaque = false
                label.border = JBUI.Borders.empty()
            }
        }
        return label
    }
}
