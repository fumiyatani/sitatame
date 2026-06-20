package dev.sitatame.intellij.actions

import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Unit tests for the pure navigation helpers behind
 * [GoToNextCommentAction] / [GoToPrevCommentAction]. They exercise the
 * line-selection logic without spinning up the IntelliJ Platform or an Editor,
 * so a failure indicates a logic regression in caret-target computation rather
 * than a Platform integration issue.
 */
class GoToCommentNavigationTest {

    // -----------------------------------------------------------------------
    // commentedLines: which lines carry a navigable comment in a file
    // -----------------------------------------------------------------------

    @Test
    fun commentedLines_collectsLineAndRangeStarts_sortedAndDeduped() {
        val comments = listOf(
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 30),
            comment(path = "src/A.kt", kind = AnchorKind.RANGE, lineStart = 10, lineEnd = 15),
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 10), // dup of range start
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 5),
        )
        assertEquals(listOf(5, 10, 30), GoToCommentAction.commentedLines(comments, "src/A.kt"))
    }

    @Test
    fun commentedLines_ignoresOtherFiles() {
        val comments = listOf(
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 7),
            comment(path = "src/B.kt", kind = AnchorKind.LINE, line = 9),
        )
        assertEquals(listOf(7), GoToCommentAction.commentedLines(comments, "src/A.kt"))
    }

    @Test
    fun commentedLines_skipsFileAndReviewAnchors() {
        val comments = listOf(
            comment(path = "src/A.kt", kind = AnchorKind.FILE),
            comment(path = "", kind = AnchorKind.REVIEW),
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 42),
        )
        assertEquals(listOf(42), GoToCommentAction.commentedLines(comments, "src/A.kt"))
    }

    @Test
    fun commentedLines_skipsNonPositiveLines() {
        val comments = listOf(
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 0),
            comment(path = "src/A.kt", kind = AnchorKind.RANGE, lineStart = 0, lineEnd = 0),
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 3),
        )
        assertEquals(listOf(3), GoToCommentAction.commentedLines(comments, "src/A.kt"))
    }

    // -----------------------------------------------------------------------
    // nextLine: strict forward / backward target relative to the caret
    // -----------------------------------------------------------------------

    private val lines = listOf(5, 10, 30)

    @Test
    fun nextLine_forward_strictlyAfterCaret() {
        assertEquals(10, GoToCommentAction.nextLine(lines, caretLine = 5, forward = true))
        assertEquals(10, GoToCommentAction.nextLine(lines, caretLine = 7, forward = true))
        assertEquals(5, GoToCommentAction.nextLine(lines, caretLine = 1, forward = true))
    }

    @Test
    fun nextLine_forward_noneAfterLast() {
        assertNull(GoToCommentAction.nextLine(lines, caretLine = 30, forward = true))
        assertNull(GoToCommentAction.nextLine(lines, caretLine = 99, forward = true))
    }

    @Test
    fun nextLine_backward_strictlyBeforeCaret() {
        assertEquals(10, GoToCommentAction.nextLine(lines, caretLine = 30, forward = false))
        assertEquals(10, GoToCommentAction.nextLine(lines, caretLine = 12, forward = false))
        assertEquals(30, GoToCommentAction.nextLine(lines, caretLine = 99, forward = false))
    }

    @Test
    fun nextLine_backward_noneBeforeFirst() {
        assertNull(GoToCommentAction.nextLine(lines, caretLine = 5, forward = false))
        assertNull(GoToCommentAction.nextLine(lines, caretLine = 1, forward = false))
    }

    @Test
    fun nextLine_emptyList_returnsNull() {
        assertNull(GoToCommentAction.nextLine(emptyList(), caretLine = 5, forward = true))
        assertNull(GoToCommentAction.nextLine(emptyList(), caretLine = 5, forward = false))
    }

    // -----------------------------------------------------------------------
    // clampLine: 1-based target → 0-based document row with boundary handling
    // -----------------------------------------------------------------------

    @Test
    fun clampLine_normalCase_subtractsOne() {
        // line 5 of a 10-line document → row 4
        assertEquals(4, GoToCommentAction.clampLine(5, lineCount = 10))
    }

    @Test
    fun clampLine_lastLine_clampsToLastRow() {
        // line 99 of a 10-line document → row 9 (last valid)
        assertEquals(9, GoToCommentAction.clampLine(99, lineCount = 10))
    }

    @Test
    fun clampLine_firstLine_returnsZero() {
        assertEquals(0, GoToCommentAction.clampLine(1, lineCount = 10))
    }

    @Test
    fun clampLine_zeroOrNegativeLine_clampsToZero() {
        // 0-based arithmetic: (0 - 1) = -1, coerced to 0
        assertEquals(0, GoToCommentAction.clampLine(0, lineCount = 10))
        assertEquals(0, GoToCommentAction.clampLine(-5, lineCount = 10))
    }

    @Test
    fun clampLine_emptyDocument_returnsZero() {
        // lineCount == 0 → coerceAtLeast(0) makes the max 0, so result is 0
        assertEquals(0, GoToCommentAction.clampLine(1, lineCount = 0))
        assertEquals(0, GoToCommentAction.clampLine(99, lineCount = 0))
    }

    @Test
    fun clampLine_singleLineDocument_returnsZero() {
        assertEquals(0, GoToCommentAction.clampLine(1, lineCount = 1))
    }

    // -----------------------------------------------------------------------
    // commentedLines: REVIEW/FILE anchors are excluded from navigation targets
    // (regression guard for P0: REVIEW comments must not appear in comments[])
    // -----------------------------------------------------------------------

    @Test
    fun commentedLines_reviewKindIsNeverNavigable() {
        // REVIEW comments have no caret target and must be excluded even if
        // they share a path (which they shouldn't, but defensive).
        val comments = listOf(
            comment(path = "src/A.kt", kind = AnchorKind.REVIEW),
            comment(path = "", kind = AnchorKind.REVIEW),
        )
        assertEquals(
            "REVIEW-kind comments must not produce navigable lines",
            emptyList<Int>(),
            GoToCommentAction.commentedLines(comments, "src/A.kt"),
        )
    }

    @Test
    fun commentedLines_fileKindIsNeverNavigable() {
        val comments = listOf(
            comment(path = "src/A.kt", kind = AnchorKind.FILE),
            comment(path = "src/A.kt", kind = AnchorKind.LINE, line = 7),
        )
        assertEquals(
            "FILE-kind comments must not produce navigable lines",
            listOf(7),
            GoToCommentAction.commentedLines(comments, "src/A.kt"),
        )
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun comment(
        path: String,
        kind: String,
        line: Int = 0,
        lineStart: Int = 0,
        lineEnd: Int = 0,
    ) = Comment(
        anchor = Anchor(
            kind = kind,
            path = path,
            line = line,
            lineStart = lineStart,
            lineEnd = lineEnd,
        ),
        body = "body",
    )
}
