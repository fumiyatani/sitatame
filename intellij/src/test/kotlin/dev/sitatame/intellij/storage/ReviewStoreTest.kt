package dev.sitatame.intellij.storage

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.nio.file.Files
import java.nio.file.Path
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset

/**
 * Unit tests for [ReviewStore] using [ReviewStore.pathsOverride] to bypass the
 * IntelliJ Platform ApplicationManager. Tests cover the 1-branch-1-file layout
 * introduced in issue #76.
 *
 * Threading: all calls are synchronous (tests are single-threaded). The lock
 * inside ReviewStore is transparent here.
 */
class ReviewStoreTest {

    private lateinit var tmpHome: Path
    private lateinit var fakeRepo: Path
    private lateinit var store: ReviewStore
    private lateinit var paths: SitatamePaths

    @Before
    fun setUp() {
        tmpHome = Files.createTempDirectory("sitatame-store-test-")
        fakeRepo = Files.createTempDirectory("sitatame-repo-")
        paths = PathsFactory.newPathsWithRoot(
            outputRoot = tmpHome.toString(),
            repoRoot = fakeRepo.toString(),
            branch = "feature/test-branch",
        )
        store = ReviewStore().apply {
            pathsOverride = paths
            // Fix clock for deterministic IDs in tests.
            clock = Clock.fixed(Instant.parse("2026-06-14T10:00:00Z"), ZoneOffset.UTC)
        }
    }

    @After
    fun tearDown() {
        deleteRecursively(tmpHome)
        deleteRecursively(fakeRepo)
    }

    // -----------------------------------------------------------------------
    // saveReview — normal path
    // -----------------------------------------------------------------------

    @Test
    fun saveReview_writesReviewMdToBranchDir() {
        val review = store.loadOrInit("", "")
        review.comments.add(sampleComment("src/main.kt", 10, "rename this"))

        val result = store.saveReview("", "")

        assertTrue("save should succeed", result.succeeded)
        assertNull("no rescue error expected", result.error)
        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        assertTrue("review.md should exist", Files.isRegularFile(reviewFile))
    }

    @Test
    fun saveReview_contentRoundtrips() {
        val review = store.loadOrInit("", "")
        review.comments.add(sampleComment("src/foo.kt", 5, "extract this block"))

        store.saveReview("", "")

        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        val readBack = Codec.decode(Files.readAllBytes(reviewFile))
        assertEquals(1, readBack.comments.size)
        assertEquals("src/foo.kt", readBack.comments[0].anchor.path)
        assertEquals(5, readBack.comments[0].anchor.line)
        assertEquals("extract this block", readBack.comments[0].body)
    }

    // -----------------------------------------------------------------------
    // saveReview — empty no-op
    // -----------------------------------------------------------------------

    @Test
    fun saveReview_emptyReviewIsNoop() {
        // Empty review: no comments, no reviewComment
        store.loadOrInit("", "")
        val result = store.saveReview("", "")

        assertFalse("empty review should not produce a file", result.succeeded)
        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        assertFalse("review.md should NOT be created for empty review", Files.exists(reviewFile))
    }

    // -----------------------------------------------------------------------
    // saveReview — .bak creation on overwrite
    // -----------------------------------------------------------------------

    @Test
    fun saveReview_createsBackupOnSecondWrite() {
        // First write
        val review1 = store.loadOrInit("", "")
        review1.comments.add(sampleComment("a.kt", 1, "first"))
        store.saveReview("", "")

        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        val bakFile = java.nio.file.Paths.get(paths.bakFile())
        assertFalse("no .bak after first write", Files.exists(bakFile))

        // Second write → should back up review.md to review.md.bak
        store.invalidate()
        val review2 = store.loadOrInit("", "")
        review2.comments.add(sampleComment("b.kt", 2, "second"))
        store.saveReview("", "")

        assertTrue("review.md should still exist", Files.isRegularFile(reviewFile))
        assertTrue(".bak should exist after second write", Files.isRegularFile(bakFile))
    }

    // -----------------------------------------------------------------------
    // detectReview
    // -----------------------------------------------------------------------

    @Test
    fun detectReview_returnsNullWhenNoFile() {
        val detected = store.detectReview("", "")
        assertNull("should be null when no review.md", detected)
    }

    @Test
    fun detectReview_returnsPathWhenFileExists() {
        val review = store.loadOrInit("", "")
        review.comments.add(sampleComment("x.kt", 1, "comment"))
        store.saveReview("", "")

        val detected = store.detectReview("", "")
        assertNotNull("should detect review.md", detected)
        assertEquals(paths.reviewFile(), detected.toString())
    }

    // -----------------------------------------------------------------------
    // recoverFromCrash
    // -----------------------------------------------------------------------

    @Test
    fun recoverFromCrash_restoresBakWhenReviewMdMissing() {
        // Simulate a crash: create .bak but no review.md
        val branchDir = java.nio.file.Paths.get(paths.branchDir())
        Files.createDirectories(branchDir)
        val bakFile = java.nio.file.Paths.get(paths.bakFile())
        Files.write(bakFile, "crash content".toByteArray())

        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        assertFalse("review.md must not exist before recovery", Files.exists(reviewFile))

        store.recoverFromCrash("", "")

        assertTrue("review.md should be restored from .bak", Files.isRegularFile(reviewFile))
        assertFalse(".bak should be gone after recovery", Files.exists(bakFile))
        assertEquals("crash content", String(Files.readAllBytes(reviewFile)))
    }

    @Test
    fun recoverFromCrash_noopWhenReviewMdExists() {
        val review = store.loadOrInit("", "")
        review.comments.add(sampleComment("z.kt", 3, "existing"))
        store.saveReview("", "")

        // Put a .bak alongside (shouldn't be touched)
        val bakFile = java.nio.file.Paths.get(paths.bakFile())
        Files.write(bakFile, "bak content".toByteArray())

        store.recoverFromCrash("", "")

        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        assertTrue("review.md should still exist", Files.isRegularFile(reviewFile))
        // .bak is preserved because review.md was already intact
        assertTrue(".bak should still exist", Files.isRegularFile(bakFile))
    }

    @Test
    fun recoverFromCrash_cleansTmpFiles() {
        val branchDir = java.nio.file.Paths.get(paths.branchDir())
        Files.createDirectories(branchDir)
        val orphan = branchDir.resolve(".review.abc123.tmp")
        Files.write(orphan, byteArrayOf())

        store.recoverFromCrash("", "")

        assertFalse("orphaned .tmp should be deleted", Files.exists(orphan))
    }

    // -----------------------------------------------------------------------
    // addComment convenience
    // -----------------------------------------------------------------------

    @Test
    fun addComment_appendsAndPersists() {
        val result = store.addComment("", "") { _ ->
            sampleComment("src/bar.kt", 42, "looks good")
        }
        assertTrue("addComment should succeed", result.succeeded)

        val readBack = Codec.decode(Files.readAllBytes(java.nio.file.Paths.get(paths.reviewFile())))
        assertEquals(1, readBack.comments.size)
        assertEquals("src/bar.kt", readBack.comments[0].anchor.path)
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun sampleComment(path: String, line: Int, body: String) = Comment(
        anchor = Anchor(
            kind = AnchorKind.LINE,
            path = path,
            line = line,
        ),
        state = ReviewState.OPEN,
        body = body,
    )

    private fun deleteRecursively(root: Path) {
        if (!Files.exists(root)) return
        Files.walk(root).use { stream ->
            stream.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }
}
