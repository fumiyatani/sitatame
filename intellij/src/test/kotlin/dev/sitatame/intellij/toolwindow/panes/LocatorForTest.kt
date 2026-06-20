package dev.sitatame.intellij.toolwindow.panes

import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Unit tests for [CommentListPane.locatorFor] — the shared anchor-to-label
 * helper used by both the comment list rows and the detail pane. Covers all
 * four anchor kinds so the FILE / REVIEW scopes (added for TUI parity) render a
 * meaningful locator rather than a bare or empty path.
 */
class LocatorForTest {

    @Test
    fun line_showsPathAndLine() {
        val a = Anchor(kind = AnchorKind.LINE, path = "src/A.kt", line = 42)
        assertEquals("src/A.kt:42", CommentListPane.locatorFor(a))
    }

    @Test
    fun range_showsPathAndSpan() {
        val a = Anchor(kind = AnchorKind.RANGE, path = "src/A.kt", lineStart = 10, lineEnd = 15)
        assertEquals("src/A.kt:10-15", CommentListPane.locatorFor(a))
    }

    @Test
    fun file_showsPathWithFileTag() {
        val a = Anchor(kind = AnchorKind.FILE, path = "src/A.kt")
        assertEquals("src/A.kt (file)", CommentListPane.locatorFor(a))
    }

    @Test
    fun review_showsReviewTagWithoutPath() {
        val a = Anchor(kind = AnchorKind.REVIEW, path = "")
        assertEquals("(review)", CommentListPane.locatorFor(a))
    }
}
