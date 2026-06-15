package dev.sitatame.intellij.actions

import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.PathsFactory
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import java.nio.file.Files
import java.nio.file.Path
import java.util.UUID

/**
 * Verifies the [ReviewStore.toggleComment] path exercised by the tool window's
 * "Toggle Resolved" action (right-click / Enter) without spinning up the
 * IntelliJ Platform.
 *
 * Two matching strategies are exercised:
 *  1. anchorId-based (exact identity) — used when the comment has a non-empty anchorId.
 *  2. path + line fallback — used for comments written before anchorId was introduced.
 *
 * The tool window's [dev.sitatame.intellij.toolwindow.SitatameToolWindowContent.commentMatches]
 * logic is tested indirectly: we replicate the same predicate here to confirm
 * the store round-trip behaves as expected.
 */
class ResolveToggleTest {

    private lateinit var tmpHome: Path
    private lateinit var fakeRepo: Path
    private lateinit var store: ReviewStore

    @Before
    fun setUp() {
        tmpHome = Files.createTempDirectory("sitatame-toggle-test-")
        fakeRepo = Files.createTempDirectory("sitatame-toggle-repo-")
        val paths = PathsFactory.newPathsWithRoot(
            outputRoot = tmpHome.toString(),
            repoRoot = fakeRepo.toString(),
            branch = "feature/toggle-test",
        )
        store = ReviewStore().apply { pathsOverride = paths }
    }

    @After
    fun tearDown() {
        deleteRecursively(tmpHome)
        deleteRecursively(fakeRepo)
    }

    // -----------------------------------------------------------------------
    // anchorId-based toggle (primary path)
    // -----------------------------------------------------------------------

    @Test
    fun toggle_openToResolved_viaAnchorId() {
        val anchorId = UUID.randomUUID().toString()
        store.addComment("", "") { _ ->
            Comment(
                anchor = Anchor(anchorId = anchorId, kind = AnchorKind.LINE, path = "src/A.kt", line = 10),
                state = ReviewState.OPEN,
                body = "needs rename",
            )
        }

        val result = store.toggleComment("", "") { c -> c.anchor.anchorId == anchorId }

        val comments = store.snapshotComments("", "")
        assertEquals("should have exactly one comment", 1, comments.size)
        assertEquals("state should flip to resolved", ReviewState.RESOLVED, comments[0].state)
        assertEquals("save result path must be non-empty", true, result?.succeeded)
    }

    @Test
    fun toggle_resolvedToOpen_viaAnchorId() {
        val anchorId = UUID.randomUUID().toString()
        store.addComment("", "") { _ ->
            Comment(
                anchor = Anchor(anchorId = anchorId, kind = AnchorKind.LINE, path = "src/B.kt", line = 5),
                state = ReviewState.RESOLVED,
                body = "was resolved",
            )
        }

        store.toggleComment("", "") { c -> c.anchor.anchorId == anchorId }

        val comments = store.snapshotComments("", "")
        assertEquals("state should flip back to open", ReviewState.OPEN, comments[0].state)
    }

    @Test
    fun toggle_toggleTwice_returnsToOriginalState() {
        val anchorId = UUID.randomUUID().toString()
        store.addComment("", "") { _ ->
            Comment(
                anchor = Anchor(anchorId = anchorId, kind = AnchorKind.LINE, path = "src/C.kt", line = 20),
                state = ReviewState.OPEN,
                body = "double-toggle",
            )
        }

        store.toggleComment("", "") { c -> c.anchor.anchorId == anchorId }
        store.invalidate()
        store.toggleComment("", "") { c -> c.anchor.anchorId == anchorId }

        val comments = store.snapshotComments("", "")
        assertEquals("after two toggles state should be open again", ReviewState.OPEN, comments[0].state)
    }

    // -----------------------------------------------------------------------
    // path + line fallback toggle
    // -----------------------------------------------------------------------

    /**
     * Verifies that a predicate built from path + line (without anchorId) can
     * still match and toggle a comment, even when the store has assigned an
     * anchorId during addComment. This mirrors the fallback branch in
     * [dev.sitatame.intellij.toolwindow.SitatameToolWindowContent.commentMatches]
     * when only one side carries an anchorId.
     */
    @Test
    fun toggle_openToResolved_viaPathLine_fallback() {
        store.addComment("", "") { _ ->
            Comment(
                anchor = Anchor(anchorId = "", kind = AnchorKind.LINE, path = "src/Legacy.kt", line = 42),
                state = ReviewState.OPEN,
                body = "legacy comment",
            )
        }

        // Predicate uses only path + kind + line (anchorId ignored).
        // ReviewStore.addComment will have assigned a UUID anchorId, so
        // c.anchor.anchorId is non-empty, but the predicate here does not
        // check it — demonstrating the fallback works independently.
        store.toggleComment("", "") { c ->
            c.anchor.path == "src/Legacy.kt" &&
                c.anchor.kind == AnchorKind.LINE &&
                c.anchor.line == 42
        }

        val comments = store.snapshotComments("", "")
        assertEquals("legacy comment should be resolved", ReviewState.RESOLVED, comments[0].state)
    }

    // -----------------------------------------------------------------------
    // no-match returns null
    // -----------------------------------------------------------------------

    @Test
    fun toggle_noMatch_returnsNull() {
        store.addComment("", "") { _ ->
            Comment(
                anchor = Anchor(anchorId = "id-xyz", kind = AnchorKind.LINE, path = "src/X.kt", line = 1),
                state = ReviewState.OPEN,
                body = "irrelevant",
            )
        }

        val result = store.toggleComment("", "") { c -> c.anchor.anchorId == "non-existent-id" }

        assertNull("toggle should return null when no comment matches", result)
        val comments = store.snapshotComments("", "")
        assertEquals("comment state should be unchanged", ReviewState.OPEN, comments[0].state)
    }

    // -----------------------------------------------------------------------
    // range anchor toggle
    // -----------------------------------------------------------------------

    @Test
    fun toggle_rangeComment_viaAnchorId() {
        val anchorId = UUID.randomUUID().toString()
        store.addComment("", "") { _ ->
            Comment(
                anchor = Anchor(
                    anchorId = anchorId,
                    kind = AnchorKind.RANGE,
                    path = "src/Range.kt",
                    lineStart = 10,
                    lineEnd = 20,
                ),
                state = ReviewState.OPEN,
                body = "range comment",
            )
        }

        store.toggleComment("", "") { c -> c.anchor.anchorId == anchorId }

        val comments = store.snapshotComments("", "")
        assertEquals("range comment should be resolved", ReviewState.RESOLVED, comments[0].state)
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun deleteRecursively(root: Path) {
        if (!Files.exists(root)) return
        Files.walk(root).use { stream ->
            stream.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }
}
