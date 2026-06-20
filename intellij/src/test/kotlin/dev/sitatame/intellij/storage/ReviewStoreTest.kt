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

    /**
     * Regression for P1-1: recoverFromCrash must return true only when a bak
     * was actually promoted. When review.md already exists (normal startup),
     * it must return false even if .bak is also present.
     */
    @Test
    fun recoverFromCrash_returnsTrueOnlyWhenActuallyPromoted() {
        val branchDir = java.nio.file.Paths.get(paths.branchDir())
        Files.createDirectories(branchDir)

        // Case 1: .bak exists, review.md absent → real recovery → true.
        val bakFile = java.nio.file.Paths.get(paths.bakFile())
        Files.write(bakFile, "crash content".toByteArray())
        assertTrue(
            "recoverFromCrash must return true when bak is promoted",
            store.recoverFromCrash("", ""),
        )

        // review.md now exists. Add .bak back to simulate normal-startup state.
        Files.write(bakFile, "leftover bak".toByteArray())
        assertFalse(
            "recoverFromCrash must return false when review.md already exists",
            store.recoverFromCrash("", ""),
        )
    }

    /**
     * Regression for P1-2: after crash recovery, a warm in-memory cache must be
     * invalidated so that the next snapshotComments/loadOrInit returns the
     * content of the recovered file rather than the stale cached Review.
     *
     * Scenario:
     *  1. Add a comment → review.md written, cache warm with "before crash".
     *  2. Encode a distinct review into .bak directly (simulates the previous
     *     save's backup). Delete review.md to simulate the crash window (bak
     *     present, review.md absent).
     *  3. Call recoverFromCrash → .bak promoted to review.md, cache invalidated.
     *  4. snapshotComments must return the content from .bak (the recovered
     *     file), NOT the stale "before crash" entry that was in the warm cache.
     */
    @Test
    fun recoverFromCrash_invalidatesCacheSoSnapshotReflectsRecoveredContent() {
        // Step 1: add "before crash" comment — warms the cache for this branch.
        store.addComment("", "") { _ -> sampleComment("src/before.kt", 1, "before crash") }
        assertEquals("warm cache has 1 comment", 1, store.snapshotComments("", "").size)

        // Step 2: build a distinct Review and encode it directly into .bak,
        // then remove review.md to simulate the crash window.
        val recoveryReview = Review(
            schema = 1,
            id = "recovery-id",
            createdAt = "2026-06-14T10:00:00Z",
            branch = "",
        ).also { r ->
            r.comments.add(sampleComment("src/recovered.kt", 99, "after recovery"))
        }
        val bakFile = java.nio.file.Paths.get(paths.bakFile())
        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        Files.createDirectories(bakFile.parent)
        Files.write(bakFile, Codec.encode(recoveryReview))
        Files.deleteIfExists(reviewFile)
        assertFalse("review.md must not exist before recovery", Files.exists(reviewFile))

        // Step 3: recover — bak must be promoted and cache invalidated.
        val promoted = store.recoverFromCrash("", "")
        assertTrue("recovery must report promotion", promoted)
        assertTrue("review.md must exist after recovery", Files.isRegularFile(reviewFile))

        // Step 4: snapshot must come from the recovered file, not the stale cache.
        val recoveredSnapshot = store.snapshotComments("", "")
        assertEquals("recovered snapshot must have exactly one comment", 1, recoveredSnapshot.size)
        assertEquals(
            "recovered comment path must come from the recovered file",
            "src/recovered.kt",
            recoveredSnapshot[0].anchor.path,
        )
        assertEquals(
            "recovered comment body must come from the recovered file",
            "after recovery",
            recoveredSnapshot[0].body,
        )
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

    /**
     * When the last comment is removed and .bak deletion fails (POSIX only),
     * saveReview must return succeeded=false so the caller does not publish
     * REVIEW_CHANGED_TOPIC on an inconsistent state.
     *
     * The test seeds review.md and a .bak file directly on disk, then removes
     * write permission from the branch directory so Files.deleteIfExists on
     * .bak throws AccessDeniedException.
     */
    @Test
    fun removeComment_lastComment_bakDeleteFails_returnsFailedResult() {
        // This test manipulates POSIX file permissions to induce a deletion
        // failure; skip on non-POSIX filesystems.
        val branchDir = java.nio.file.Paths.get(paths.branchDir())
        val isPosix = try {
            Files.createDirectories(branchDir)
            Files.getPosixFilePermissions(branchDir)
            true
        } catch (_: UnsupportedOperationException) {
            false
        }
        Assume.assumeTrue("Skipping .bak deletion failure test on non-POSIX filesystem", isPosix)

        // Add a comment to create review.md.
        store.addComment("", "") { _ -> sampleComment("src/only.kt", 1, "only comment") }

        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        assertTrue("review.md should exist after addComment", Files.isRegularFile(reviewFile))

        // Seed a stale .bak directly on disk to simulate a previous write cycle.
        val bakFile = java.nio.file.Paths.get(paths.bakFile())
        Files.copy(reviewFile, bakFile)
        assertTrue(".bak must exist before the test", Files.isRegularFile(bakFile))

        // Remove write permission from the branch directory so Files.deleteIfExists on
        // .bak throws AccessDeniedException.
        val originalPerms = Files.getPosixFilePermissions(branchDir)
        Files.setPosixFilePermissions(branchDir, PosixFilePermissions.fromString("r-x------"))
        try {
            // Remove the last remaining comment so saveReview enters the empty-review path.
            val result = store.removeComment("", "") { c -> c.anchor.path == "src/only.kt" }

            assertNotNull("result must not be null when a comment was removed", result)
            assertFalse(
                "SaveResult.succeeded must be false when .bak deletion fails",
                result!!.succeeded,
            )
        } finally {
            // Restore permissions so tearDown can clean up temp dirs.
            Files.setPosixFilePermissions(branchDir, originalPerms)
        }
    }

    @Test
    fun removeComment_encodeFails_returnsFailedResult() {
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
    // setReviewComment / getReviewComment (P0/P1 regression)
    // -----------------------------------------------------------------------

    /**
     * Regression for P0/P1: setReviewComment must persist the body in
     * review.reviewComment (the top-level review_comment YAML scalar), not
     * in comments[]. Mirrors Go TUI confirmModal for KindReview (modal.go:219-223).
     */
    @Test
    fun setReviewComment_persistsInReviewCommentField_notInComments() {
        val result = store.setReviewComment("", "", "Overall LGTM, minor nits below.")
        assertTrue("setReviewComment should succeed", result.succeeded)

        val readBack = Codec.decode(Files.readAllBytes(java.nio.file.Paths.get(paths.reviewFile())))
        assertEquals(
            "reviewComment must be persisted as top-level review_comment",
            "Overall LGTM, minor nits below.",
            readBack.reviewComment,
        )
        assertEquals("comments[] must remain empty — not polluted by REVIEW kind", 0, readBack.comments.size)
    }

    @Test
    fun setReviewComment_overwrites_previousValue() {
        store.setReviewComment("", "", "first draft")
        val result = store.setReviewComment("", "", "second draft — LGTM")
        assertTrue("second setReviewComment should succeed", result.succeeded)

        val readBack = Codec.decode(Files.readAllBytes(java.nio.file.Paths.get(paths.reviewFile())))
        assertEquals(
            "reviewComment must reflect the latest overwrite (Go in-place semantics)",
            "second draft — LGTM",
            readBack.reviewComment,
        )
    }

    @Test
    fun setReviewComment_doesNotTouchExistingComments() {
        store.addComment("", "") { _ -> sampleComment("src/app.kt", 5, "rename this") }

        store.setReviewComment("", "", "overall comment")

        val readBack = Codec.decode(Files.readAllBytes(java.nio.file.Paths.get(paths.reviewFile())))
        assertEquals("reviewComment must be set", "overall comment", readBack.reviewComment)
        assertEquals("existing LINE comment must be preserved in comments[]", 1, readBack.comments.size)
        assertEquals("src/app.kt", readBack.comments[0].anchor.path)
    }

    @Test
    fun getReviewComment_returnsEmptyWhenNotSet() {
        // Fresh store: loadOrInit creates an empty review, reviewComment defaults to "".
        assertEquals("", store.getReviewComment("", ""))
    }

    @Test
    fun getReviewComment_returnsSetValue() {
        store.setReviewComment("", "", "hello review")
        assertEquals("hello review", store.getReviewComment("", ""))
    }

    @Test
    fun setReviewComment_emptyString_doesNotCreateFile() {
        // An empty reviewComment + no comments = empty review → file must not be created.
        val result = store.setReviewComment("", "", "")
        assertFalse("empty reviewComment should not create a file", result.succeeded)
        assertFalse(
            "review.md must not be created for a blank-only review",
            Files.exists(java.nio.file.Paths.get(paths.reviewFile())),
        )
    }

    /**
     * Regression for #2 (clear existing review comment with empty input):
     * Setting an existing review_comment to "" must clear it from the persisted
     * file. After clearing, reloading from disk must not resurface the old value.
     *
     * Mirrors Go TUI confirmModal: body = strings.TrimRight(ta.Value(), "\n")
     * then m.Review.ReviewComment = body (allows blank → clears the field).
     * AddReviewCommentAction now calls setReviewComment("") instead of
     * early-returning when existing != "" and the new body is "".
     */
    @Test
    fun setReviewComment_clearExistingComment_removesReviewCommentOnDisk() {
        // Step 1: write a non-empty review comment.
        val setResult = store.setReviewComment("", "", "initial comment")
        assertTrue("initial setReviewComment should succeed", setResult.succeeded)

        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        assertTrue("review.md must exist after setting a comment", Files.isRegularFile(reviewFile))
        assertEquals("in-memory value must be set", "initial comment", store.getReviewComment("", ""))

        // Step 2: clear by setting to empty string.
        // The review has no comments[], so clearing review_comment → empty review → file deleted.
        val clearResult = store.setReviewComment("", "", "")
        // setReviewComment("") on a no-comments review triggers the empty-review deletion path.
        // succeeded=false here means the file was deleted (path is ""), which is expected.
        // What we care about is that the field is gone from the persisted state.
        assertFalse("review.md must be deleted after clearing the only content", Files.exists(reviewFile))

        // Step 3: invalidate and reload from disk to verify the comment is truly gone.
        store.invalidate()
        assertEquals(
            "getReviewComment must return empty string after clear + reload",
            "",
            store.getReviewComment("", ""),
        )
    }

    /**
     * Regression for #2: clearing review_comment while LINE comments still exist
     * must not delete review.md (comments[] is non-empty). The file should remain
     * with reviewComment="" and the existing comments intact.
     */
    @Test
    fun setReviewComment_clearExistingComment_preservesComments() {
        // Seed a line comment so the review is non-empty even after clear.
        store.addComment("", "") { _ -> sampleComment("src/keep.kt", 1, "keep me") }
        store.setReviewComment("", "", "initial review comment")

        val reviewFile = java.nio.file.Paths.get(paths.reviewFile())
        assertTrue("review.md must exist with both a comment and a review_comment", Files.isRegularFile(reviewFile))

        // Clear the review comment; the line comment must keep the file alive.
        store.setReviewComment("", "", "")

        assertTrue("review.md must still exist (line comment remains)", Files.isRegularFile(reviewFile))

        // Reload from disk and verify: reviewComment gone, comments[] intact.
        store.invalidate()
        val readBack = Codec.decode(Files.readAllBytes(reviewFile))
        assertEquals(
            "reviewComment must be empty after clear",
            "",
            readBack.reviewComment,
        )
        assertEquals("comments[] must be preserved after clearing review_comment", 1, readBack.comments.size)
        assertEquals("src/keep.kt", readBack.comments[0].anchor.path)
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
