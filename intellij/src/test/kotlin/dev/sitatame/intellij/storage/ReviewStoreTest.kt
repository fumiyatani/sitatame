package dev.sitatame.intellij.storage

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume
import org.junit.Before
import org.junit.Test
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.attribute.PosixFilePermissions
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import java.util.concurrent.atomic.AtomicInteger

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
    // rescue JSON content: full Review + Go key-name parity
    // -----------------------------------------------------------------------

    /**
     * The rescue JSON must include a full "review" object with the same
     * top-level key names as Go's rescuePayload, and the review sub-object
     * must carry all comments, not just a comment_count summary.
     */
    @Test
    fun writeRescue_containsFullReviewJson() {
        store.encodeFunc = { _ -> throw RuntimeException("injected encode failure") }

        val review = store.loadOrInit("", "")
        review.branch = "feature/test"
        review.reviewComment = "overall comment"
        review.comments.add(sampleComment("src/main.kt", 42, "rename this method"))
        review.comments.add(sampleComment("src/util.kt", 7, "extract helper"))

        val result = store.saveReview("", "")
        assertNotNull("rescue error expected", result.error)

        val rescuePath = java.nio.file.Paths.get(result.error!!.rescuePath)
        val raw = String(Files.readAllBytes(rescuePath))
        val root = Json.parseToJsonElement(raw).jsonObject

        // Top-level Go-compatible keys.
        assertEquals("schema", "rescue/1", root["schema"]?.jsonPrimitive?.content)
        assertTrue("saved_at must be present", root.containsKey("saved_at"))
        assertEquals("reason", "yaml encode failed", root["reason"]?.jsonPrimitive?.content)
        assertTrue("original_encode_error must be present", root.containsKey("original_encode_error"))
        assertTrue("review must be present", root.containsKey("review"))

        // review sub-object must carry comments array, not just comment_count.
        val reviewObj = root["review"]!!.jsonObject
        assertTrue("review.branch must be present", reviewObj.containsKey("branch"))
        assertTrue("review.comments must be present", reviewObj.containsKey("comments"))

        val commentsArr = reviewObj["comments"]!!.let {
            kotlinx.serialization.json.Json.parseToJsonElement(it.toString())
                .let { e -> e as? kotlinx.serialization.json.JsonArray }
        }
        assertNotNull("comments should be a JSON array", commentsArr)
        assertEquals("comment count in JSON", 2, commentsArr!!.size)
    }

    // -----------------------------------------------------------------------
    // rescue filename collision avoidance
    // -----------------------------------------------------------------------

    /**
     * Two back-to-back Encode failures at the same wall-clock second must
     * produce two distinct rescue files. The nanos suffix differentiates them.
     *
     * The clock advances by 1ns per call so that the second component is
     * always the same (both share "20260614T120000") but the nanos component
     * differs. This is independent of how many times the clock is consulted
     * internally (freshReview, writeRescue filename, writeRescue saved_at).
     */
    @Test
    fun writeRescue_nanosSuffixPreventsFilenameCollision() {
        // Always-incrementing nanos clock: each call returns +1ns so any two
        // calls in the same second will have different nanos values.
        val nanoCounter = AtomicInteger(0)
        val baseSecond = Instant.parse("2026-06-14T12:00:00Z")
        store.clock = object : Clock() {
            override fun getZone() = ZoneOffset.UTC
            override fun withZone(zone: java.time.ZoneId) = this
            override fun instant(): Instant =
                baseSecond.plusNanos(nanoCounter.getAndIncrement().toLong())
        }
        // Inject a broken encoder to trigger writeRescue.
        store.encodeFunc = { _ -> throw RuntimeException("injected encode failure") }

        // Pre-populate cache so loadOrInit does not consult clock again during save.
        val review = store.loadOrInit("", "")
        review.comments.add(sampleComment("x.kt", 1, "collision-test"))
        val result1 = store.saveReview("", "")

        store.invalidate()
        val review2 = store.loadOrInit("", "")
        review2.comments.add(sampleComment("y.kt", 2, "collision-test-2"))
        val result2 = store.saveReview("", "")

        // Both should have produced a rescue error.
        assertNotNull("first save should have rescue error", result1.error)
        assertNotNull("second save should have rescue error", result2.error)

        val path1 = result1.error!!.rescuePath
        val path2 = result2.error!!.rescuePath

        assertTrue("first rescue file must exist", Files.isRegularFile(java.nio.file.Paths.get(path1)))
        assertTrue("second rescue file must exist", Files.isRegularFile(java.nio.file.Paths.get(path2)))
        assertTrue("rescue paths must be distinct (nanos suffix prevents collision)", path1 != path2)

        // Both filenames must match the pattern review.md.rescue.<ts>-<nanos>.json
        for (p in listOf(path1, path2)) {
            val name = java.nio.file.Paths.get(p).fileName.toString()
            assertTrue(
                "filename '$name' must match review.md.rescue.<ts>-<nanos>.json",
                name.matches(Regex("review\\.md\\.rescue\\.\\d{8}T\\d{6}-\\d{9}\\.json"))
            )
        }
    }

    // -----------------------------------------------------------------------
    // rescue file permission (POSIX-only)
    // -----------------------------------------------------------------------

    /**
     * On POSIX filesystems, the rescue file must have permissions rw-------
     * (0600) to keep contents owner-private, mirroring Go's os.WriteFile
     * with 0o600. On Windows this test is skipped via [Assume].
     */
    @Test
    fun writeRescue_rescueFileHas0600Permissions() {
        // Skip on non-POSIX filesystems (Windows).
        val branchDir = java.nio.file.Paths.get(paths.branchDir())
        Files.createDirectories(branchDir)
        val isPosix = try {
            Files.getPosixFilePermissions(branchDir)
            true
        } catch (_: UnsupportedOperationException) {
            false
        }
        Assume.assumeTrue("Skipping 0600 permission test on non-POSIX filesystem", isPosix)

        store.encodeFunc = { _ -> throw RuntimeException("injected encode failure for perm test") }
        val review = store.loadOrInit("", "")
        review.comments.add(sampleComment("p.kt", 1, "perm-test"))

        val result = store.saveReview("", "")

        assertNotNull("rescue error expected", result.error)
        val rescuePath = java.nio.file.Paths.get(result.error!!.rescuePath)
        assertTrue("rescue file must exist", Files.isRegularFile(rescuePath))

        val perms = Files.getPosixFilePermissions(rescuePath)
        val expected = PosixFilePermissions.fromString("rw-------")
        assertEquals(
            "rescue file must have 0600 permissions (rw-------)",
            expected,
            perms,
        )
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
    // removeComment
    // -----------------------------------------------------------------------

    @Test
    fun removeComment_singleMatch_removesAndPersists() {
        store.addComment("", "") { _ -> sampleComment("src/a.kt", 1, "to remove") }
        store.addComment("", "") { _ -> sampleComment("src/b.kt", 2, "to keep") }

        val result = store.removeComment("", "") { c -> c.anchor.path == "src/a.kt" }

        assertNotNull("removeComment should return a SaveResult", result)
        val comments = store.snapshotComments("", "")
        assertEquals("one comment should remain", 1, comments.size)
        assertEquals("remaining comment should be b.kt", "src/b.kt", comments[0].anchor.path)
    }

    @Test
    fun removeComment_multipleMatch_removesOnlyFirstMatching() {
        store.addComment("", "") { _ -> sampleComment("src/dup.kt", 1, "dup 1") }
        store.addComment("", "") { _ -> sampleComment("src/dup.kt", 2, "dup 2") }
        store.addComment("", "") { _ -> sampleComment("src/other.kt", 3, "keep") }

        // Remove only the first comment matching the predicate (dup.kt line 1).
        store.removeComment("", "") { c -> c.anchor.path == "src/dup.kt" }

        val comments = store.snapshotComments("", "")
        assertEquals("two comments should remain (second dup.kt + other.kt)", 2, comments.size)
        // The first matching comment (line 1) is gone; line 2 and other.kt remain.
        assertTrue(
            "dup.kt line 2 should still be present",
            comments.any { it.anchor.path == "src/dup.kt" && it.anchor.line == 2 },
        )
        assertTrue(
            "other.kt should still be present",
            comments.any { it.anchor.path == "src/other.kt" },
        )
    }

    @Test
    fun removeComment_noMatch_returnsNull() {
        store.addComment("", "") { _ -> sampleComment("src/x.kt", 10, "existing") }

        val result = store.removeComment("", "") { c -> c.anchor.path == "src/nonexistent.kt" }

        assertNull("no match should return null", result)
        val comments = store.snapshotComments("", "")
        assertEquals("comment should be unchanged", 1, comments.size)
    }

    @Test
    fun removeComment_lastComment_deletesReviewMdAndDoesNotReviveOnReload() {
        store.addComment("", "") { _ -> sampleComment("src/last.kt", 5, "last one") }

        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        assertTrue("review.md should exist after addComment", Files.isRegularFile(reviewFile))

        val result = store.removeComment("", "") { c -> c.anchor.path == "src/last.kt" }

        // The save must succeed (file was deleted) and review.md must be gone.
        assertNotNull("result must not be null when a comment was removed", result)
        assertTrue("SaveResult.succeeded must be true when review.md was deleted", result!!.succeeded)
        assertFalse("review.md must be deleted after removing last comment", Files.exists(reviewFile))

        // Invalidate cache and reload to verify comments do not resurrect from disk.
        store.invalidate()
        val comments = store.snapshotComments("", "")
        assertEquals("no comments should remain after reload", 0, comments.size)
    }

    @Test
    fun removeComment_encodeFails_doesNotPublish() {
        // Add two comments so that after removing one the review is non-empty,
        // which forces saveReview to attempt encoding (and fail).
        store.addComment("", "") { _ -> sampleComment("src/enc.kt", 1, "enc test") }
        store.addComment("", "") { _ -> sampleComment("src/keep.kt", 2, "keep me") }

        // Inject a failing encoder for the next save.
        store.encodeFunc = { _ -> throw RuntimeException("injected encode failure") }

        // removeComment will call saveReview, which attempts encode and fails.
        val result = store.removeComment("", "") { c -> c.anchor.path == "src/enc.kt" }

        // SaveResult must not be null (comment was found and removed from memory).
        assertNotNull("result must not be null when a matching comment was removed", result)
        // succeeded must be false: encode failed, path is empty.
        assertFalse("SaveResult.succeeded must be false on encode failure", result!!.succeeded)
        assertNotNull("RescueError must be set on encode failure", result.error)
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
