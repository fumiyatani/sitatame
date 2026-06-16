package dev.sitatame.intellij.markers

import com.intellij.icons.AllIcons
import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for the pure helper functions in [SitatameLineMarkerProvider].
 *
 * No IntelliJ Platform or ReviewStore is required; we test the anchor-to-line
 * resolution logic and icon aggregation directly.
 */
class SitatameLineMarkerProviderTest {

    private val provider = SitatameLineMarkerProvider()

    // -----------------------------------------------------------------------
    // commentsForLine — LINE anchor
    // -----------------------------------------------------------------------

    @Test
    fun commentsForLine_lineAnchor_exactMatch() {
        val comments = listOf(comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 10))
        val result = provider.commentsForLine(comments, "src/A.kt", 10)
        assertEquals(1, result.size)
    }

    @Test
    fun commentsForLine_lineAnchor_differentLine_noMatch() {
        val comments = listOf(comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 10))
        val result = provider.commentsForLine(comments, "src/A.kt", 11)
        assertTrue(result.isEmpty())
    }

    @Test
    fun commentsForLine_lineAnchor_differentPath_noMatch() {
        val comments = listOf(comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 10))
        val result = provider.commentsForLine(comments, "src/B.kt", 10)
        assertTrue(result.isEmpty())
    }

    // -----------------------------------------------------------------------
    // commentsForLine — RANGE anchor
    // -----------------------------------------------------------------------

    @Test
    fun commentsForLine_rangeAnchor_lineInsideRange() {
        val comments = listOf(comment(path = "src/A.kt", kind = AnchorKind.RANGE, lineStart = 5, lineEnd = 15))
        val result = provider.commentsForLine(comments, "src/A.kt", 10)
        assertEquals(1, result.size)
    }

    @Test
    fun commentsForLine_rangeAnchor_lineAtStart() {
        val comments = listOf(comment(path = "src/A.kt", kind = AnchorKind.RANGE, lineStart = 5, lineEnd = 15))
        val result = provider.commentsForLine(comments, "src/A.kt", 5)
        assertEquals(1, result.size)
    }

    @Test
    fun commentsForLine_rangeAnchor_lineAtEnd() {
        val comments = listOf(comment(path = "src/A.kt", kind = AnchorKind.RANGE, lineStart = 5, lineEnd = 15))
        val result = provider.commentsForLine(comments, "src/A.kt", 15)
        assertEquals(1, result.size)
    }

    @Test
    fun commentsForLine_rangeAnchor_lineOutsideRange_noMatch() {
        val comments = listOf(comment(path = "src/A.kt", kind = AnchorKind.RANGE, lineStart = 5, lineEnd = 15))
        val result = provider.commentsForLine(comments, "src/A.kt", 16)
        assertTrue(result.isEmpty())
    }

    // -----------------------------------------------------------------------
    // commentsForLine — FILE and REVIEW anchors are excluded
    // -----------------------------------------------------------------------

    @Test
    fun commentsForLine_fileAnchor_excluded() {
        val comments = listOf(comment(path = "src/A.kt", kind = AnchorKind.FILE))
        val result = provider.commentsForLine(comments, "src/A.kt", 1)
        assertTrue("FILE anchor must not appear in gutter", result.isEmpty())
    }

    @Test
    fun commentsForLine_reviewAnchor_excluded() {
        val comments = listOf(comment(path = "src/A.kt", kind = AnchorKind.REVIEW))
        val result = provider.commentsForLine(comments, "src/A.kt", 1)
        assertTrue("REVIEW anchor must not appear in gutter", result.isEmpty())
    }

    // -----------------------------------------------------------------------
    // commentsForLine — multiple comments, only some match
    // -----------------------------------------------------------------------

    @Test
    fun commentsForLine_multipleComments_onlyMatchingReturned() {
        val comments = listOf(
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 10),
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 20),
            comment(path = "src/B.kt", kind = AnchorKind.LINE, line = 10),
            comment(path = "src/A.kt", kind = AnchorKind.RANGE, lineStart = 8, lineEnd = 12),
        )
        val result = provider.commentsForLine(comments, "src/A.kt", 10)
        // LINE at 10 and RANGE 8-12 both cover line 10.
        assertEquals(2, result.size)
    }

    // -----------------------------------------------------------------------
    // aggregateIcon
    // -----------------------------------------------------------------------

    @Test
    fun aggregateIcon_allResolved_returnsChecked() {
        val comments = listOf(
            comment(state = ReviewState.RESOLVED),
            comment(state = ReviewState.RESOLVED),
        )
        assertEquals(AllIcons.Actions.Checked, provider.aggregateIcon(comments))
    }

    @Test
    fun aggregateIcon_anyOpen_returnsNote() {
        val comments = listOf(
            comment(state = ReviewState.RESOLVED),
            comment(state = ReviewState.OPEN),
        )
        assertEquals(AllIcons.General.Note, provider.aggregateIcon(comments))
    }

    @Test
    fun aggregateIcon_singleOpen_returnsNote() {
        val comments = listOf(comment(state = ReviewState.OPEN))
        assertEquals(AllIcons.General.Note, provider.aggregateIcon(comments))
    }

    @Test
    fun aggregateIcon_stale_returnsWarning() {
        val comments = listOf(comment(state = ReviewState.STALE))
        assertEquals(AllIcons.General.Warning, provider.aggregateIcon(comments))
    }

    @Test
    fun aggregateIcon_staleMixedWithOpen_returnsWarning() {
        val comments = listOf(
            comment(state = ReviewState.OPEN),
            comment(state = ReviewState.STALE),
        )
        assertEquals(AllIcons.General.Warning, provider.aggregateIcon(comments))
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun comment(
        path: String = "src/Default.kt",
        kind: String = AnchorKind.LINE,
        line: Int = 1,
        lineStart: Int = 0,
        lineEnd: Int = 0,
        state: String = ReviewState.OPEN,
    ) = Comment(
        anchor = Anchor(
            kind = kind,
            path = path,
            line = line,
            lineStart = lineStart,
            lineEnd = lineEnd,
        ),
        state = state,
        body = "body",
    )
}
