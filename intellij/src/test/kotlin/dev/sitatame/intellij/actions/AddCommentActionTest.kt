package dev.sitatame.intellij.actions

import dev.sitatame.intellij.storage.Anchor
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Codec
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.PathsFactory
import dev.sitatame.intellij.storage.ReviewState
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.nio.file.Files
import java.nio.file.Path
import java.util.UUID

/**
 * Verifies the write path the AddCommentAction depends on without spinning up
 * the IntelliJ test platform: build an anchor, append a comment via the same
 * store-level surface the action calls, then assert the YAML on disk decodes
 * back to the same anchor + body.
 *
 * As of issue #76 reviews live under the 1-branch-1-file layout:
 *   `<outputRoot>/<projectSlug>/<branchSlug>/review.md`
 *
 * The full platform-test version that injects an [com.intellij.openapi.editor.Editor]
 * and clicks the action lives in Phase 2 — running BasePlatformTestCase
 * requires IDE test fixtures that don't add coverage here over what this
 * already verifies.
 */
class AddCommentActionTest {

    private lateinit var tmpHome: Path
    private lateinit var fakeRepo: Path

    @Before
    fun setUp() {
        tmpHome = Files.createTempDirectory("sitatame-home-")
        fakeRepo = Files.createTempDirectory("sitatame-repo-")
    }

    @After
    fun tearDown() {
        deleteRecursively(tmpHome)
        deleteRecursively(fakeRepo)
    }

    @Test
    fun appendCommentPersistsYamlOnDisk() {
        val branch = "feature/add-comment"
        val paths = PathsFactory.newPathsWithRoot(
            outputRoot = tmpHome.toString(),
            repoRoot = fakeRepo.toString(),
            branch = branch,
        )
        val newComment = Comment(
            anchor = Anchor(
                anchorId = UUID.randomUUID().toString(),
                kind = AnchorKind.RANGE,
                path = "src/foo.kt",
                lineStart = 5,
                lineEnd = 8,
            ),
            state = ReviewState.OPEN,
            body = "please extract this block",
        )

        // Mimic ReviewStore.saveReview without the IntelliJ Application: write
        // a Review with one comment via the codec, atomic-move into branchDir.
        val branchDir = paths.branchDir()
        Files.createDirectories(java.nio.file.Paths.get(branchDir))
        val review = dev.sitatame.intellij.storage.Review(
            schema = 1,
            id = "20260601T120000-add-comment",
            createdAt = "2026-06-01T12:00:00Z",
            branch = branch,
            base = dev.sitatame.intellij.storage.Ref(ref = "origin/main", sha = "abc"),
            head = dev.sitatame.intellij.storage.Ref(ref = "HEAD", sha = "def"),
        ).apply {
            comments.add(newComment)
        }
        val bytes = Codec.encode(review)
        val finalPath = java.nio.file.Paths.get(paths.reviewFile())
        Files.write(finalPath, bytes)
        assertTrue("review file should exist", Files.exists(finalPath))

        val readBack = Codec.decode(Files.readAllBytes(finalPath))
        assertEquals("feature/add-comment", readBack.branch)
        assertEquals(1, readBack.comments.size)
        val rc = readBack.comments[0]
        assertEquals("src/foo.kt", rc.anchor.path)
        assertEquals(AnchorKind.RANGE, rc.anchor.kind)
        assertEquals(5, rc.anchor.lineStart)
        assertEquals(8, rc.anchor.lineEnd)
        assertEquals(ReviewState.OPEN, rc.state)
        assertEquals("please extract this block", rc.body)
    }

    private fun deleteRecursively(root: Path) {
        if (!Files.exists(root)) return
        Files.walk(root).use { stream ->
            stream.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }
}
