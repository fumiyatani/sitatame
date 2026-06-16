package dev.sitatame.intellij.actions

import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit-level test for the prompt builder. The clipboard side-effect path is
 * exercised by manual / sandbox IDE runs; here we just guard the *format* so
 * an inadvertent template change shows up in CI.
 *
 * Using a plain JUnit test (not BasePlatformTestCase) keeps this test fast
 * and avoids spinning up the IntelliJ platform for what is pure string
 * formatting.
 */
class CopyAIPromptActionTest {

    @Test
    fun promptIncludesAnchorAndBody() {
        val action = CopyAIPromptAction()
        val prompt = action.buildPrompt(
            listOf(
                Comment(
                    anchor = Anchor(
                        anchorId = "abc-123",
                        kind = AnchorKind.LINE,
                        path = "src/foo.go",
                        line = 42,
                    ),
                    state = ReviewState.OPEN,
                    body = "rename this please",
                ),
                Comment(
                    anchor = Anchor(
                        anchorId = "def-456",
                        kind = AnchorKind.RANGE,
                        path = "src/bar.go",
                        lineStart = 10,
                        lineEnd = 15,
                    ),
                    state = ReviewState.OPEN,
                    body = "split into two functions",
                ),
            )
        )
        assertTrue("expected leading instruction", prompt.contains("以下の修正指示に従って"))
        assertTrue("expected line locator", prompt.contains("[abc-123] src/foo.go:42 (line, open)"))
        assertTrue("expected range locator", prompt.contains("[def-456] src/bar.go:10-15 (range, open)"))
        assertTrue("expected body quoted with '> '", prompt.contains("> rename this please"))
        assertTrue("expected related diff section", prompt.contains("関連 diff (要約):"))
    }

    @Test
    fun multilineBodyKeepsEveryLineQuoted() {
        val action = CopyAIPromptAction()
        val prompt = action.buildPrompt(
            listOf(
                Comment(
                    anchor = Anchor(
                        anchorId = "x",
                        kind = AnchorKind.LINE,
                        path = "src/x.kt",
                        line = 1,
                    ),
                    state = ReviewState.OPEN,
                    body = "first line\nsecond line\nthird line",
                ),
            )
        )
        assertTrue(prompt.contains("> first line"))
        assertTrue(prompt.contains("> second line"))
        assertTrue(prompt.contains("> third line"))
    }

    /**
     * Verifies that buildPrompt handles an empty list gracefully — the caller
     * (background task) is responsible for the "no open comments" notification
     * guard, but the pure function itself must not throw on an empty input and
     * should still emit the leading instruction + diff placeholder skeleton.
     */
    @Test
    fun buildPromptWithEmptyListEmitsSkeletonOnly() {
        val action = CopyAIPromptAction()
        val prompt = action.buildPrompt(emptyList())
        assertTrue("leading instruction must be present even for empty list",
            prompt.contains("以下の修正指示に従って"))
        assertTrue("diff placeholder must be present even for empty list",
            prompt.contains("関連 diff (要約):"))
        // No comment blocks expected.
        assertTrue("no anchor bracket expected for empty list",
            !prompt.contains("["))
    }

    @Test
    fun buildPromptResolvedCommentIncludesState() {
        val action = CopyAIPromptAction()
        val prompt = action.buildPrompt(
            listOf(
                Comment(
                    anchor = Anchor(
                        anchorId = "zzz",
                        kind = AnchorKind.LINE,
                        path = "src/z.kt",
                        line = 99,
                    ),
                    state = ReviewState.RESOLVED,
                    body = "already fixed",
                ),
            )
        )
        // buildPrompt formats whatever is passed; filtering by state is the
        // caller's responsibility.  Verify the state label appears so reviewers
        // reading the raw prompt can see it was resolved.
        assertTrue("resolved state should appear in output",
            prompt.contains("resolved"))
    }
}
