package dev.sitatame.intellij.inlay

import com.intellij.openapi.editor.EditorCustomElementRenderer
import com.intellij.openapi.editor.Inlay
import com.intellij.openapi.editor.markup.TextAttributes
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import java.awt.Color
import java.awt.Font
import java.awt.Graphics
import java.awt.Rectangle

/**
 * Block inlay renderer for a group of [Comment]s anchored to the same source
 * line. One renderer instance is created per group; it paints a compact card
 * for each comment below the anchor line.
 *
 * Layout per comment (row height = [ROW_HEIGHT]):
 *   ┌─────────────────────────────────────────────────┐
 *   │ [●|○] <body text, truncated>        [Resolve] │
 *   └─────────────────────────────────────────────────┘
 *
 * Resolved comments are collapsed by default (height = [COLLAPSED_HEIGHT]).
 * Open/Stale comments start expanded. Collapse state is in-memory only;
 * persistent storage is deferred to Phase 2 (#117 follow-up).
 *
 * Button hit-testing is handled by [SitatameInlayMouseListener], which is
 * wired by [SitatameInlayService]. This renderer is paint-only and does not
 * hold mutable state beyond [collapsed].
 */
internal class SitatameCommentInlayRenderer(
    val comments: List<Comment>,
    /** Called when the Resolve/Reopen button is clicked for a comment at [index]. */
    val onToggle: (index: Int) -> Unit,
) : EditorCustomElementRenderer {

    /** Per-comment collapse state. Initial value: resolved → true, else false. */
    val collapsed: BooleanArray = BooleanArray(comments.size) { i ->
        comments[i].state == ReviewState.RESOLVED
    }

    companion object {
        internal const val ROW_HEIGHT = 24
        internal const val COLLAPSED_HEIGHT = 16
        internal const val PADDING_H = 8
        internal const val PADDING_V = 4
        internal const val BUTTON_WIDTH = 72
        internal const val BUTTON_HEIGHT = 16

        /** Background colour for the inlay card (light neutral). */
        private val BG_COLOR = Color(0xF5, 0xF5, 0xF0)

        /** Border colour. */
        private val BORDER_COLOR = Color(0xCC, 0xCC, 0xCC)

        /** Resolved badge colour. */
        private val RESOLVED_COLOR = Color(0x6A, 0xA8, 0x4F)

        /** Open badge colour. */
        private val OPEN_COLOR = Color(0x4A, 0x90, 0xD9)
    }

    override fun calcWidthInPixels(inlay: Inlay<*>): Int {
        // Fill editor width — the platform clips to the content area.
        return inlay.editor.contentComponent.width.coerceAtLeast(400)
    }

    override fun calcHeightInPixels(inlay: Inlay<*>): Int {
        var total = 0
        for (i in comments.indices) {
            total += if (collapsed[i]) COLLAPSED_HEIGHT else ROW_HEIGHT
        }
        // Add 2px top/bottom margin for the whole block.
        return total + 4
    }

    override fun paint(
        inlay: Inlay<*>,
        g: Graphics,
        targetRegion: Rectangle,
        textAttributes: TextAttributes,
    ) {
        val x = targetRegion.x
        var y = targetRegion.y + 2   // 2px top margin

        for (i in comments.indices) {
            val c = comments[i]
            val rowH = if (collapsed[i]) COLLAPSED_HEIGHT else ROW_HEIGHT
            paintCommentRow(g, c, i, x, y, targetRegion.width, rowH)
            y += rowH
        }
    }

    private fun paintCommentRow(
        g: Graphics,
        comment: Comment,
        index: Int,
        x: Int,
        y: Int,
        width: Int,
        rowH: Int,
    ) {
        // Background
        g.color = BG_COLOR
        g.fillRect(x, y, width - PADDING_H, rowH)

        // Border
        g.color = BORDER_COLOR
        g.drawRect(x, y, width - PADDING_H - 1, rowH - 1)

        val isResolved = comment.state == ReviewState.RESOLVED

        // State badge (filled circle)
        val badgeColor = if (isResolved) RESOLVED_COLOR else OPEN_COLOR
        g.color = badgeColor
        val badgeX = x + PADDING_H
        val badgeY = y + (rowH - 8) / 2
        g.fillOval(badgeX, badgeY, 8, 8)

        // Body text (truncated to available width)
        val textX = badgeX + 12
        val buttonAreaWidth = BUTTON_WIDTH + PADDING_H * 2
        val maxTextWidth = width - textX - buttonAreaWidth - PADDING_H
        val bodyLine = comment.body.lineSequence().firstOrNull()?.take(80) ?: ""
        val displayText = truncateText(g, bodyLine, maxTextWidth)

        g.color = Color.DARK_GRAY
        val font = g.font.deriveFont(Font.PLAIN, 11f)
        g.font = font
        val textY = y + (rowH + g.fontMetrics.ascent - g.fontMetrics.descent) / 2
        g.drawString(displayText, textX, textY)

        // "Resolve" / "Reopen" button (paint only — hit-test by SitatameInlayMouseListener)
        val btnLabel = if (isResolved) "Reopen" else "Resolve"
        val btnX = x + width - buttonAreaWidth - PADDING_H
        val btnY = y + (rowH - BUTTON_HEIGHT) / 2
        paintButton(g, btnLabel, btnX, btnY)
    }

    private fun paintButton(g: Graphics, label: String, x: Int, y: Int) {
        g.color = Color(0xE0, 0xE8, 0xF0)
        g.fillRoundRect(x, y, BUTTON_WIDTH, BUTTON_HEIGHT, 4, 4)
        g.color = BORDER_COLOR
        g.drawRoundRect(x, y, BUTTON_WIDTH - 1, BUTTON_HEIGHT - 1, 4, 4)

        g.color = Color.DARK_GRAY
        val font = g.font.deriveFont(Font.PLAIN, 10f)
        g.font = font
        val fm = g.fontMetrics
        val tx = x + (BUTTON_WIDTH - fm.stringWidth(label)) / 2
        val ty = y + (BUTTON_HEIGHT + fm.ascent - fm.descent) / 2
        g.drawString(label, tx, ty)
    }

    private fun truncateText(g: Graphics, text: String, maxWidth: Int): String {
        val fm = g.fontMetrics
        if (maxWidth <= 0) return ""
        if (fm.stringWidth(text) <= maxWidth) return text
        val ellipsis = "…"
        val ellipsisWidth = fm.stringWidth(ellipsis)
        var end = text.length
        while (end > 0 && fm.stringWidth(text.substring(0, end)) + ellipsisWidth > maxWidth) {
            end--
        }
        return text.substring(0, end) + ellipsis
    }

    /**
     * Returns the comment [index] whose "Resolve/Reopen" button contains
     * ([clickX], [clickY]) within [targetRegion], or -1 if no button was hit.
     * Called by [SitatameInlayMouseListener].
     */
    internal fun hitTestButton(targetRegion: Rectangle, clickX: Int, clickY: Int): Int {
        var y = targetRegion.y + 2
        val width = targetRegion.width
        for (i in comments.indices) {
            val rowH = if (collapsed[i]) COLLAPSED_HEIGHT else ROW_HEIGHT
            val btnX = targetRegion.x + width - (BUTTON_WIDTH + PADDING_H * 2) - PADDING_H
            val btnY = y + (rowH - BUTTON_HEIGHT) / 2
            if (clickX in btnX until btnX + BUTTON_WIDTH && clickY in btnY until btnY + BUTTON_HEIGHT) {
                return i
            }
            y += rowH
        }
        return -1
    }

    /**
     * Toggle the [collapsed] flag for the comment at [index].
     * The inlay must be repainted by the caller after this call.
     */
    internal fun toggleCollapse(index: Int) {
        if (index in collapsed.indices) {
            collapsed[index] = !collapsed[index]
        }
    }
}
