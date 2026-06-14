package dev.sitatame.web.ui

import dev.sitatame.web.api.CommentDto
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class ThreadGroupingTest {

    // -----------------------------------------------------------------------
    // Fixtures
    // -----------------------------------------------------------------------

    private fun lineComment(
        anchorId: String,
        path: String,
        line: Int,
        side: String = "head",
        state: String = "open",
    ) = CommentDto(
        anchorId = anchorId,
        kind = "line",
        path = path,
        side = side,
        line = line,
        state = state,
        body = "body of $anchorId",
    )

    private fun rangeComment(
        anchorId: String,
        path: String,
        lineStart: Int,
        lineEnd: Int,
        side: String = "head",
        state: String = "open",
    ) = CommentDto(
        anchorId = anchorId,
        kind = "range",
        path = path,
        side = side,
        lineStart = lineStart,
        lineEnd = lineEnd,
        state = state,
        body = "body of $anchorId",
    )

    private fun fileComment(
        anchorId: String,
        path: String,
        state: String = "open",
    ) = CommentDto(
        anchorId = anchorId,
        kind = "file",
        path = path,
        state = state,
        body = "body of $anchorId",
    )

    private fun reviewComment(
        anchorId: String,
        state: String = "open",
    ) = CommentDto(
        anchorId = anchorId,
        kind = "review",
        path = "",  // review-level comments have no path; empty string satisfies non-null Dto
        state = state,
        body = "body of $anchorId",
    )

    // -----------------------------------------------------------------------
    // groupCommentsIntoThreads
    // -----------------------------------------------------------------------

    @Test
    fun `single line comment produces one thread`() {
        val comments = listOf(lineComment("c1", "foo/bar.kt", 10))
        val threads = groupCommentsIntoThreads(comments, "foo/bar.kt")
        assertEquals(1, threads.size)
        assertEquals(1, threads[0].comments.size)
        assertEquals(ThreadKey.Line("foo/bar.kt", "head", 10), threads[0].key)
    }

    @Test
    fun `three comments on same anchor produce one thread with three comments`() {
        val comments = listOf(
            lineComment("c1", "a.kt", 5),
            lineComment("c2", "a.kt", 5),
            lineComment("c3", "a.kt", 5),
        )
        val threads = groupCommentsIntoThreads(comments, "a.kt")
        assertEquals(1, threads.size)
        assertEquals(3, threads[0].comments.size)
    }

    @Test
    fun `comments on two different anchors produce two threads`() {
        val comments = listOf(
            lineComment("c1", "a.kt", 5),
            lineComment("c2", "a.kt", 10),
        )
        val threads = groupCommentsIntoThreads(comments, "a.kt")
        assertEquals(2, threads.size)
    }

    @Test
    fun `review-kind comments are skipped`() {
        val comments = listOf(
            reviewComment("r1"),
            lineComment("c1", "a.kt", 5),
        )
        val threads = groupCommentsIntoThreads(comments, "a.kt")
        assertEquals(1, threads.size)
        assertEquals(ThreadKey.Line("a.kt", "head", 5), threads[0].key)
    }

    @Test
    fun `comments for a different path are skipped`() {
        val comments = listOf(
            lineComment("c1", "other.kt", 5),
            lineComment("c2", "a.kt", 5),
        )
        val threads = groupCommentsIntoThreads(comments, "a.kt")
        assertEquals(1, threads.size)
        assertEquals(ThreadKey.Line("a.kt", "head", 5), threads[0].key)
    }

    @Test
    fun `thread order follows first appearance of each anchor`() {
        val comments = listOf(
            lineComment("c1", "a.kt", 20),
            lineComment("c2", "a.kt", 5),
            lineComment("c3", "a.kt", 20), // reply to first thread
        )
        val threads = groupCommentsIntoThreads(comments, "a.kt")
        assertEquals(2, threads.size)
        // First thread is line 20 (first appearance), second is line 5
        assertEquals(ThreadKey.Line("a.kt", "head", 20), threads[0].key)
        assertEquals(ThreadKey.Line("a.kt", "head", 5), threads[1].key)
        // Reply is in the first thread
        assertEquals(2, threads[0].comments.size)
    }

    @Test
    fun `multiple file-level comments on same path are grouped into one thread`() {
        val comments = listOf(
            fileComment("f1", "a.kt"),
            fileComment("f2", "a.kt"),
        )
        val threads = groupCommentsIntoThreads(comments, "a.kt")
        assertEquals(1, threads.size)
        assertEquals(2, threads[0].comments.size)
        assertEquals(ThreadKey.File("a.kt"), threads[0].key)
    }

    @Test
    fun `range comment produces a range thread key`() {
        val comments = listOf(rangeComment("r1", "a.kt", lineStart = 3, lineEnd = 7))
        val threads = groupCommentsIntoThreads(comments, "a.kt")
        assertEquals(1, threads.size)
        assertEquals(ThreadKey.Range("a.kt", "head", 3, 7), threads[0].key)
    }

    // -----------------------------------------------------------------------
    // computeThreadState
    // -----------------------------------------------------------------------

    @Test
    fun `any open comment makes thread Open`() {
        val comments = listOf(
            lineComment("c1", "a.kt", 1, state = "resolved"),
            lineComment("c2", "a.kt", 1, state = "open"),
        )
        assertEquals(ThreadState.Open, computeThreadState(comments))
    }

    @Test
    fun `all resolved comments make thread Resolved`() {
        val comments = listOf(
            lineComment("c1", "a.kt", 1, state = "resolved"),
            lineComment("c2", "a.kt", 1, state = "resolved"),
        )
        assertEquals(ThreadState.Resolved, computeThreadState(comments))
    }

    @Test
    fun `any stale comment (no open) makes thread Stale`() {
        val comments = listOf(
            lineComment("c1", "a.kt", 1, state = "stale"),
            lineComment("c2", "a.kt", 1, state = "resolved"),
        )
        assertEquals(ThreadState.Stale, computeThreadState(comments))
    }

    @Test
    fun `open takes priority over stale`() {
        val comments = listOf(
            lineComment("c1", "a.kt", 1, state = "stale"),
            lineComment("c2", "a.kt", 1, state = "open"),
            lineComment("c3", "a.kt", 1, state = "resolved"),
        )
        assertEquals(ThreadState.Open, computeThreadState(comments))
    }

    @Test
    fun `empty list is treated as Resolved`() {
        assertEquals(ThreadState.Resolved, computeThreadState(emptyList()))
    }

    // -----------------------------------------------------------------------
    // filterThreads
    // -----------------------------------------------------------------------

    private fun makeThread(state: ThreadState): Thread {
        val key = ThreadKey.File("dummy.kt")
        val comment = fileComment("x", "dummy.kt", state = state.name.lowercase())
        return Thread(key, listOf(comment), state)
    }

    @Test
    fun `StateFilter All returns all threads`() {
        val threads = listOf(makeThread(ThreadState.Open), makeThread(ThreadState.Resolved), makeThread(ThreadState.Stale))
        assertEquals(3, filterThreads(threads, StateFilter.All).size)
    }

    @Test
    fun `StateFilter Open returns only open threads`() {
        val threads = listOf(makeThread(ThreadState.Open), makeThread(ThreadState.Resolved))
        val result = filterThreads(threads, StateFilter.Open)
        assertEquals(1, result.size)
        assertEquals(ThreadState.Open, result[0].state)
    }

    @Test
    fun `StateFilter Resolved returns only resolved threads`() {
        val threads = listOf(makeThread(ThreadState.Open), makeThread(ThreadState.Resolved))
        val result = filterThreads(threads, StateFilter.Resolved)
        assertEquals(1, result.size)
        assertEquals(ThreadState.Resolved, result[0].state)
    }

    @Test
    fun `StateFilter Stale returns only stale threads`() {
        val threads = listOf(makeThread(ThreadState.Open), makeThread(ThreadState.Stale))
        val result = filterThreads(threads, StateFilter.Stale)
        assertEquals(1, result.size)
        assertEquals(ThreadState.Stale, result[0].state)
    }

    // -----------------------------------------------------------------------
    // defaultCollapsed
    // -----------------------------------------------------------------------

    @Test
    fun `Resolved thread is collapsed by default`() {
        assertTrue(defaultCollapsed(makeThread(ThreadState.Resolved)))
    }

    @Test
    fun `Open thread is not collapsed by default`() {
        assertFalse(defaultCollapsed(makeThread(ThreadState.Open)))
    }

    @Test
    fun `Stale thread is not collapsed by default`() {
        assertFalse(defaultCollapsed(makeThread(ThreadState.Stale)))
    }

    // -----------------------------------------------------------------------
    // CommentDto.toCommentTarget()
    // -----------------------------------------------------------------------

    @Test
    fun `line comment converts to CommentTarget Line`() {
        val dto = lineComment("c1", "a.kt", 42, side = "base")
        val target = dto.toCommentTarget()
        assertEquals(CommentTarget.Line("a.kt", 42, "base"), target)
    }

    @Test
    fun `range comment converts to CommentTarget Range`() {
        val dto = rangeComment("r1", "a.kt", lineStart = 5, lineEnd = 10, side = "head")
        val target = dto.toCommentTarget()
        assertEquals(CommentTarget.Range("a.kt", 5, 10, "head"), target)
    }

    @Test
    fun `file comment converts to CommentTarget File`() {
        val dto = fileComment("f1", "a.kt")
        val target = dto.toCommentTarget()
        assertEquals(CommentTarget.File("a.kt"), target)
    }

    @Test
    fun `review comment converts to CommentTarget Review`() {
        val dto = reviewComment("r1")
        val target = dto.toCommentTarget()
        assertEquals(CommentTarget.Review, target)
    }

    @Test
    fun `unknown kind throws error`() {
        val dto = CommentDto(
            anchorId = "x",
            kind = "unknown_kind",
            path = "a.kt",
            state = "open",
            body = "body",
        )
        assertThrows<IllegalStateException> { dto.toCommentTarget() }
    }

    // -----------------------------------------------------------------------
    // threadKey() edge cases
    // -----------------------------------------------------------------------

    @Test
    fun `review comment has null threadKey`() {
        assertNull(reviewComment("r1").threadKey())
    }

    @Test
    fun `line comment missing side has null threadKey`() {
        val dto = CommentDto(anchorId = "c1", kind = "line", path = "a.kt", side = null, line = 5, state = "open", body = "b")
        assertNull(dto.threadKey())
    }

    @Test
    fun `line comment missing line has null threadKey`() {
        val dto = CommentDto(anchorId = "c1", kind = "line", path = "a.kt", side = "head", line = null, state = "open", body = "b")
        assertNull(dto.threadKey())
    }
}
