package dev.sitatame.intellij.inlay

import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for the pure logic extracted from [SitatameInlayService].
 *
 * [SitatameInlayService.groupByLine] is the only testable unit without a
 * running IntelliJ Platform instance because InlayModel / Editor / Graphics
 * are all EDT-bound singletons. The renderer's paint/layout paths are covered
 * by manual QA (see docs/manual-qa-checklist.md).
 */
class SitatameInlayFactoryTest {

    // -----------------------------------------------------------------------
    // groupByLine: basic routing
    // -----------------------------------------------------------------------

    @Test
    fun groupByLine_emptyList_returnsEmpty() {
        val result = SitatameInlayService.groupByLine(emptyList(), "src/Foo.kt")
        assertTrue("empty input should produce empty map", result.isEmpty())
    }

    @Test
    fun groupByLine_lineComment_groupedCorrectly() {
        val c = lineComment("src/Foo.kt", 10, "body A")
        val result = SitatameInlayService.groupByLine(listOf(c), "src/Foo.kt")
        assertEquals("should have one group", 1, result.size)
        assertEquals("group key should be line 10", 10, result.keys.first())
        assertEquals("group should contain the comment", listOf(c), result[10])
    }

    @Test
    fun groupByLine_multipleCommentsOnSameLine_groupedTogether() {
        val c1 = lineComment("src/Foo.kt", 5, "first")
        val c2 = lineComment("src/Foo.kt", 5, "second")
        val result = SitatameInlayService.groupByLine(listOf(c1, c2), "src/Foo.kt")
        assertEquals("one group for line 5", 1, result.size)
        assertEquals("group has both comments", 2, result[5]?.size)
    }

    @Test
    fun groupByLine_commentsOnDifferentLines_separateGroups() {
        val c1 = lineComment("src/Foo.kt", 3, "line 3")
        val c2 = lineComment("src/Foo.kt", 7, "line 7")
        val result = SitatameInlayService.groupByLine(listOf(c1, c2), "src/Foo.kt")
        assertEquals("two groups", 2, result.size)
        assertTrue("key 3 present", result.containsKey(3))
        assertTrue("key 7 present", result.containsKey(7))
    }

    @Test
    fun groupByLine_rangeComment_usesLineStart() {
        val c = rangeComment("src/Foo.kt", lineStart = 10, lineEnd = 20, body = "range body")
        val result = SitatameInlayService.groupByLine(listOf(c), "src/Foo.kt")
        assertEquals("range comment keyed by lineStart", 10, result.keys.first())
    }

    // -----------------------------------------------------------------------
    // groupByLine: path filtering
    // -----------------------------------------------------------------------

    @Test
    fun groupByLine_differentPath_excluded() {
        val c = lineComment("src/Bar.kt", 1, "unrelated")
        val result = SitatameInlayService.groupByLine(listOf(c), "src/Foo.kt")
        assertTrue("comment for different file should be excluded", result.isEmpty())
    }

    @Test
    fun groupByLine_mixedPaths_onlyMatchingIncluded() {
        val cFoo = lineComment("src/Foo.kt", 4, "relevant")
        val cBar = lineComment("src/Bar.kt", 4, "irrelevant")
        val result = SitatameInlayService.groupByLine(listOf(cFoo, cBar), "src/Foo.kt")
        assertEquals("only Foo.kt comment included", 1, result[4]?.size)
    }

    // -----------------------------------------------------------------------
    // groupByLine: anchor kind filtering
    // -----------------------------------------------------------------------

    @Test
    fun groupByLine_reviewLevelComment_excluded() {
        val c = Comment(
            anchor = Anchor(kind = AnchorKind.REVIEW, path = "src/Foo.kt", line = 0),
            state = ReviewState.OPEN,
            body = "review-level",
        )
        val result = SitatameInlayService.groupByLine(listOf(c), "src/Foo.kt")
        assertTrue("review-level anchor has no line and should be excluded", result.isEmpty())
    }

    @Test
    fun groupByLine_fileLevelComment_excluded() {
        val c = Comment(
            anchor = Anchor(kind = AnchorKind.FILE, path = "src/Foo.kt", line = 0),
            state = ReviewState.OPEN,
            body = "file-level",
        )
        val result = SitatameInlayService.groupByLine(listOf(c), "src/Foo.kt")
        assertTrue("file-level anchor should be excluded", result.isEmpty())
    }

    // -----------------------------------------------------------------------
    // groupByLine: sorted output
    // -----------------------------------------------------------------------

    @Test
    fun groupByLine_resultIsSortedByLineAscending() {
        val comments = listOf(
            lineComment("src/Foo.kt", 20, "c"),
            lineComment("src/Foo.kt", 1, "a"),
            lineComment("src/Foo.kt", 10, "b"),
        )
        val result = SitatameInlayService.groupByLine(comments, "src/Foo.kt")
        assertEquals("keys sorted ascending", listOf(1, 10, 20), result.keys.toList())
    }

    // -----------------------------------------------------------------------
    // SitatameCommentInlayRenderer: collapsed defaults
    // -----------------------------------------------------------------------

    @Test
    fun renderer_resolvedComment_collapsedByDefault() {
        val c = lineComment("src/Foo.kt", 1, "resolved", state = ReviewState.RESOLVED)
        val renderer = SitatameCommentInlayRenderer(listOf(c), onToggle = {})
        assertTrue("resolved comment should start collapsed", renderer.collapsed[0])
    }

    @Test
    fun renderer_openComment_expandedByDefault() {
        val c = lineComment("src/Foo.kt", 1, "open", state = ReviewState.OPEN)
        val renderer = SitatameCommentInlayRenderer(listOf(c), onToggle = {})
        assertTrue("open comment should start expanded (collapsed=false)", !renderer.collapsed[0])
    }

    @Test
    fun renderer_emptyCommentList_noIndexOutOfBounds() {
        // Regression: ensure the renderer can be constructed with an empty list
        // without throwing during initialization.
        val renderer = SitatameCommentInlayRenderer(emptyList(), onToggle = {})
        assertEquals("collapsed array should be empty", 0, renderer.collapsed.size)
    }

    @Test
    fun renderer_toggleCollapse_flipsState() {
        val c = lineComment("src/Foo.kt", 1, "open", state = ReviewState.OPEN)
        val renderer = SitatameCommentInlayRenderer(listOf(c), onToggle = {})
        renderer.toggleCollapse(0)
        assertTrue("should be collapsed after toggle", renderer.collapsed[0])
        renderer.toggleCollapse(0)
        assertTrue("should be expanded again after second toggle", !renderer.collapsed[0])
    }

    @Test
    fun renderer_toggleCollapse_outOfBounds_noException() {
        val renderer = SitatameCommentInlayRenderer(emptyList(), onToggle = {})
        renderer.toggleCollapse(99)  // must not throw
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun lineComment(
        path: String,
        line: Int,
        body: String,
        state: String = ReviewState.OPEN,
    ) = Comment(
        anchor = Anchor(kind = AnchorKind.LINE, path = path, line = line),
        state = state,
        body = body,
    )

    private fun rangeComment(
        path: String,
        lineStart: Int,
        lineEnd: Int,
        body: String,
        state: String = ReviewState.OPEN,
    ) = Comment(
        anchor = Anchor(kind = AnchorKind.RANGE, path = path, lineStart = lineStart, lineEnd = lineEnd),
        state = state,
        body = body,
    )
}
