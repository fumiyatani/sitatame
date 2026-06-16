package dev.sitatame.intellij.toolwindow

import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for [SitatameToolWindowContent.Companion.filterComments].
 *
 * Pure function tests — no IntelliJ Platform required.
 */
class FilterTest {

    // -----------------------------------------------------------------------
    // ALL
    // -----------------------------------------------------------------------

    @Test
    fun filterAll_returnsAllComments() {
        val comments = listOf(
            comment(ReviewState.OPEN),
            comment(ReviewState.RESOLVED),
            comment(ReviewState.STALE),
        )
        val result = SitatameToolWindowContent.filterComments(comments, FilterState.ALL)
        assertEquals("ALL should return all 3 comments", 3, result.size)
    }

    @Test
    fun filterAll_emptyInput_returnsEmpty() {
        val result = SitatameToolWindowContent.filterComments(emptyList(), FilterState.ALL)
        assertTrue("empty input → empty output", result.isEmpty())
    }

    // -----------------------------------------------------------------------
    // OPENED
    // -----------------------------------------------------------------------

    @Test
    fun filterOpened_returnsOnlyOpenComments() {
        val comments = listOf(
            comment(ReviewState.OPEN, "open-1"),
            comment(ReviewState.RESOLVED, "resolved-1"),
            comment(ReviewState.STALE, "stale-1"),
            comment(ReviewState.OPEN, "open-2"),
        )
        val result = SitatameToolWindowContent.filterComments(comments, FilterState.OPENED)
        assertEquals("OPENED should return 2 open comments", 2, result.size)
        assertTrue("all results should be OPEN", result.all { it.state == ReviewState.OPEN })
    }

    @Test
    fun filterOpened_noOpen_returnsEmpty() {
        val comments = listOf(
            comment(ReviewState.RESOLVED),
            comment(ReviewState.STALE),
        )
        val result = SitatameToolWindowContent.filterComments(comments, FilterState.OPENED)
        assertTrue("no open comments → empty result", result.isEmpty())
    }

    // -----------------------------------------------------------------------
    // RESOLVED
    // -----------------------------------------------------------------------

    @Test
    fun filterResolved_returnsOnlyResolvedComments() {
        val comments = listOf(
            comment(ReviewState.OPEN, "open-1"),
            comment(ReviewState.RESOLVED, "resolved-1"),
            comment(ReviewState.STALE, "stale-1"),
            comment(ReviewState.RESOLVED, "resolved-2"),
        )
        val result = SitatameToolWindowContent.filterComments(comments, FilterState.RESOLVED)
        assertEquals("RESOLVED should return 2 resolved comments", 2, result.size)
        assertTrue("all results should be RESOLVED", result.all { it.state == ReviewState.RESOLVED })
    }

    @Test
    fun filterResolved_noResolved_returnsEmpty() {
        val comments = listOf(
            comment(ReviewState.OPEN),
            comment(ReviewState.STALE),
        )
        val result = SitatameToolWindowContent.filterComments(comments, FilterState.RESOLVED)
        assertTrue("no resolved comments → empty result", result.isEmpty())
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun comment(state: String, body: String = "body") = Comment(
        anchor = Anchor(kind = AnchorKind.LINE, path = "src/A.kt", line = 1),
        state = state,
        body = body,
    )
}
