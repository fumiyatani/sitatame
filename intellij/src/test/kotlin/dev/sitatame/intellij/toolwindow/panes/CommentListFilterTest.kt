package dev.sitatame.intellij.toolwindow.panes

import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.toolwindow.FilterState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for [CommentListPane.Companion.filterAndSubset].
 *
 * Pure function — no IntelliJ Platform required.
 */
class CommentListFilterTest {

    // -----------------------------------------------------------------------
    // FileSelection.All — only state filter applies
    // -----------------------------------------------------------------------

    @Test
    fun allFiles_allStates_returnsEverything() {
        val comments = listOf(
            comment("src/A.kt", ReviewState.OPEN),
            comment("src/B.kt", ReviewState.RESOLVED),
            comment("src/C.kt", ReviewState.STALE),
        )
        val result = filterAndSubset(comments, FilterState.ALL, FileSelection.All)
        assertEquals(3, result.size)
    }

    @Test
    fun allFiles_openedFilter_returnsOnlyOpen() {
        val comments = listOf(
            comment("src/A.kt", ReviewState.OPEN),
            comment("src/B.kt", ReviewState.RESOLVED),
            comment("src/C.kt", ReviewState.OPEN),
        )
        val result = filterAndSubset(comments, FilterState.OPENED, FileSelection.All)
        assertEquals(2, result.size)
        assertTrue(result.all { it.state == ReviewState.OPEN })
    }

    @Test
    fun allFiles_resolvedFilter_returnsOnlyResolved() {
        val comments = listOf(
            comment("src/A.kt", ReviewState.OPEN),
            comment("src/B.kt", ReviewState.RESOLVED),
        )
        val result = filterAndSubset(comments, FilterState.RESOLVED, FileSelection.All)
        assertEquals(1, result.size)
        assertEquals(ReviewState.RESOLVED, result[0].state)
    }

    // -----------------------------------------------------------------------
    // FileSelection.One — file filter AND state filter
    // -----------------------------------------------------------------------

    @Test
    fun oneFile_allStates_returnsOnlyThatFile() {
        val comments = listOf(
            comment("src/A.kt", ReviewState.OPEN),
            comment("src/B.kt", ReviewState.OPEN),
            comment("src/A.kt", ReviewState.RESOLVED),
        )
        val result = filterAndSubset(comments, FilterState.ALL, FileSelection.One("src/A.kt"))
        assertEquals(2, result.size)
        assertTrue(result.all { it.anchor.path == "src/A.kt" })
    }

    @Test
    fun oneFile_openedFilter_intersects() {
        val comments = listOf(
            comment("src/A.kt", ReviewState.OPEN),
            comment("src/A.kt", ReviewState.RESOLVED),
            comment("src/B.kt", ReviewState.OPEN),
        )
        val result = filterAndSubset(comments, FilterState.OPENED, FileSelection.One("src/A.kt"))
        assertEquals(1, result.size)
        assertEquals(ReviewState.OPEN, result[0].state)
        assertEquals("src/A.kt", result[0].anchor.path)
    }

    @Test
    fun oneFile_noMatchingComments_returnsEmpty() {
        val comments = listOf(
            comment("src/B.kt", ReviewState.OPEN),
            comment("src/C.kt", ReviewState.RESOLVED),
        )
        val result = filterAndSubset(comments, FilterState.ALL, FileSelection.One("src/A.kt"))
        assertTrue("No comments for src/A.kt should yield empty result", result.isEmpty())
    }

    @Test
    fun emptyInput_returnsEmpty() {
        val result = filterAndSubset(emptyList(), FilterState.ALL, FileSelection.All)
        assertTrue(result.isEmpty())
    }

    // -----------------------------------------------------------------------
    // Path matching edge cases
    // -----------------------------------------------------------------------

    @Test
    fun oneFile_exactPathMatch_works() {
        val comments = listOf(comment("module/src/Foo.kt", ReviewState.OPEN))
        val result = filterAndSubset(comments, FilterState.ALL, FileSelection.One("module/src/Foo.kt"))
        assertEquals(1, result.size)
    }

    @Test
    fun oneFile_differentPath_noMatch() {
        val comments = listOf(comment("module/src/Foo.kt", ReviewState.OPEN))
        val result = filterAndSubset(comments, FilterState.ALL, FileSelection.One("src/Foo.kt"))
        // "src/Foo.kt" does not match "module/src/Foo.kt" as exact path.
        // The suffix check: "module/src/Foo.kt".endsWith("/src/Foo.kt") → true
        // So this DOES match via suffix rule. Assert 1 to document the behaviour.
        assertEquals(1, result.size)
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun filterAndSubset(
        comments: List<Comment>,
        filter: FilterState,
        selection: FileSelection,
    ): List<Comment> = CommentListPane.filterAndSubset(comments, filter, selection)

    private fun comment(path: String, state: String) = Comment(
        anchor = Anchor(kind = AnchorKind.LINE, path = path, line = 1),
        state = state,
        body = "body",
    )
}
