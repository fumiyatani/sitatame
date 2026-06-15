package dev.sitatame.intellij.toolwindow

import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Direct unit tests for [SitatameToolWindowContent.Companion.commentMatches].
 *
 * These tests exercise the matching logic without spinning up the IntelliJ
 * Platform or a ReviewStore, so failures indicate a logic regression in the
 * pure function itself — not a store round-trip issue.
 */
class CommentMatchesTest {

    // -----------------------------------------------------------------------
    // anchorId-based matching (primary path)
    // -----------------------------------------------------------------------

    @Test
    fun anchorId_match_openComment() {
        val c = comment(anchorId = "id-abc", state = ReviewState.OPEN)
        val target = comment(anchorId = "id-abc", state = ReviewState.OPEN)
        assertTrue(commentMatches(c, target))
    }

    @Test
    fun anchorId_match_resolvedComment() {
        val c = comment(anchorId = "id-abc", state = ReviewState.RESOLVED)
        val target = comment(anchorId = "id-abc", state = ReviewState.RESOLVED)
        assertTrue(commentMatches(c, target))
    }

    @Test
    fun anchorId_noMatch_differentIds() {
        val c = comment(anchorId = "id-abc")
        val target = comment(anchorId = "id-xyz")
        assertFalse(commentMatches(c, target))
    }

    // -----------------------------------------------------------------------
    // One side has anchorId, the other does not → falls back to coordinates
    // -----------------------------------------------------------------------

    @Test
    fun oneSideHasAnchorId_fallsBackToLineFallback_matches() {
        // c has a populated anchorId (written by a newer client),
        // target has an empty anchorId (older in-memory snapshot).
        // Because one side is empty, anchorId comparison is skipped and
        // coordinates must match.
        val c = comment(anchorId = "id-abc", path = "src/A.kt", kind = AnchorKind.LINE, line = 10)
        val target = comment(anchorId = "", path = "src/A.kt", kind = AnchorKind.LINE, line = 10)
        assertTrue(commentMatches(c, target))
    }

    @Test
    fun oneSideHasAnchorId_fallsBackToLineFallback_noMatchOnDifferentLine() {
        val c = comment(anchorId = "id-abc", path = "src/A.kt", kind = AnchorKind.LINE, line = 10)
        val target = comment(anchorId = "", path = "src/A.kt", kind = AnchorKind.LINE, line = 99)
        assertFalse(commentMatches(c, target))
    }

    // -----------------------------------------------------------------------
    // Line fallback (both anchorIds empty)
    // -----------------------------------------------------------------------

    @Test
    fun lineFallback_pathKindLine_allMatch() {
        val c = comment(anchorId = "", path = "src/B.kt", kind = AnchorKind.LINE, line = 42)
        val target = comment(anchorId = "", path = "src/B.kt", kind = AnchorKind.LINE, line = 42)
        assertTrue(commentMatches(c, target))
    }

    @Test
    fun lineFallback_differentPath_noMatch() {
        val c = comment(anchorId = "", path = "src/B.kt", kind = AnchorKind.LINE, line = 42)
        val target = comment(anchorId = "", path = "src/C.kt", kind = AnchorKind.LINE, line = 42)
        assertFalse(commentMatches(c, target))
    }

    @Test
    fun lineFallback_differentKind_noMatch() {
        // Same path and nominal line numbers, but different anchor kinds →
        // the `else -> false` branch should fire.
        val c = comment(anchorId = "", path = "src/B.kt", kind = AnchorKind.LINE, line = 42)
        val target = comment(anchorId = "", path = "src/B.kt", kind = AnchorKind.RANGE, lineStart = 42, lineEnd = 42)
        assertFalse(commentMatches(c, target))
    }

    // -----------------------------------------------------------------------
    // Range fallback (both anchorIds empty)
    // -----------------------------------------------------------------------

    @Test
    fun rangeFallback_startEnd_match() {
        val c = comment(anchorId = "", path = "src/D.kt", kind = AnchorKind.RANGE, lineStart = 5, lineEnd = 10)
        val target = comment(anchorId = "", path = "src/D.kt", kind = AnchorKind.RANGE, lineStart = 5, lineEnd = 10)
        assertTrue(commentMatches(c, target))
    }

    @Test
    fun rangeFallback_differentEnd_noMatch() {
        val c = comment(anchorId = "", path = "src/D.kt", kind = AnchorKind.RANGE, lineStart = 5, lineEnd = 10)
        val target = comment(anchorId = "", path = "src/D.kt", kind = AnchorKind.RANGE, lineStart = 5, lineEnd = 11)
        assertFalse(commentMatches(c, target))
    }

    // -----------------------------------------------------------------------
    // Multiple-comment scenario: only the target comment matches
    //
    // commentMatches is a pure predicate — "multiple comments" is exercised
    // by calling it for each comment in a list and asserting only the
    // expected one returns true.
    // -----------------------------------------------------------------------

    @Test
    fun multipleComments_onlyTargetMatches() {
        val target = comment(anchorId = "id-target", path = "src/E.kt", kind = AnchorKind.LINE, line = 7)
        val others = listOf(
            comment(anchorId = "id-other1", path = "src/E.kt", kind = AnchorKind.LINE, line = 7),
            comment(anchorId = "id-other2", path = "src/F.kt", kind = AnchorKind.LINE, line = 7),
            comment(anchorId = "", path = "src/E.kt", kind = AnchorKind.LINE, line = 99),
        )

        val matched = (others + target).filter { commentMatches(it, target) }

        assertTrue("exactly one comment should match", matched.size == 1)
        assertTrue("the matched comment must be the target", matched[0].anchor.anchorId == "id-target")
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun commentMatches(c: Comment, target: Comment): Boolean =
        SitatameToolWindowContent.commentMatches(c, target)

    private fun comment(
        anchorId: String = "",
        path: String = "src/Default.kt",
        kind: String = AnchorKind.LINE,
        line: Int = 1,
        lineStart: Int = 0,
        lineEnd: Int = 0,
        state: String = ReviewState.OPEN,
        body: String = "body",
    ) = Comment(
        anchor = Anchor(
            anchorId = anchorId,
            kind = kind,
            path = path,
            line = line,
            lineStart = lineStart,
            lineEnd = lineEnd,
        ),
        state = state,
        body = body,
    )
}
