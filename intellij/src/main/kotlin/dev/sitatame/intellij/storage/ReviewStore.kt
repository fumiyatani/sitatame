package dev.sitatame.intellij.storage

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.Service
import com.intellij.openapi.diagnostic.Logger
import dev.sitatame.intellij.settings.SitatameSettings
import java.io.File
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption
import java.nio.file.attribute.PosixFilePermissions
import java.time.Clock
import java.time.LocalDateTime
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.Locale
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap

/**
 * Application-level singleton service for sitatame review storage.
 *
 * As of issue #76 the layout is 1-branch-1-file:
 *   `<OutputRoot>/<ProjectSlug>/<BranchSlug>/review.md`
 *
 * The store is the single coordination point between the action layer
 * (AddComment / ResolveComment) and the tool window (which reads the cached
 * Review to render the JBList).
 *
 * Thread model:
 *  - Action layer runs on EDT (per the 2024.2+ threading rules) and dispatches
 *    file I/O off-EDT before returning to EDT to refresh the tool window.
 *  - Mutations are serialised by a per-store lock so two background I/O
 *    threads writing different comments don't lose updates.
 */
@Service(Service.Level.APP)
class ReviewStore {

    private val log = Logger.getInstance(ReviewStore::class.java)
    private val lock = Any()

    /**
     * In-memory cache keyed by `<projectSlug>/<branchSlug>`. Holds the most
     * recently loaded or written Review for that branch.
     */
    private val cache = ConcurrentHashMap<String, Review>()

    var clock: Clock = Clock.systemUTC()

    /**
     * When set, [paths] returns this value directly instead of consulting
     * [SitatameSettings]. Intended for unit tests that run outside the
     * IntelliJ Platform (no ApplicationManager). Production code must leave
     * this null.
     */
    @Suppress("VisibleForTests")
    var pathsOverride: SitatamePaths? = null

    /**
     * When set, replaces [Codec.encode] in [saveReview]. Tests inject a
     * throwing lambda to exercise the rescue path without a real encode
     * failure. Production code must leave this null.
     */
    @Suppress("VisibleForTests")
    var encodeFunc: ((Review) -> ByteArray)? = null

    private val settings: SitatameSettings
        get() = ApplicationManager.getApplication().getService(SitatameSettings::class.java)

    private fun paths(repoRoot: String, branch: String): SitatamePaths =
        pathsOverride
            ?: PathsFactory.newPaths(repoRoot, branch, overrideHome = settings.state.sitatameHomeOverride)

    private fun cacheKey(p: SitatamePaths): String = "${p.projectSlug}/${p.slug}"

    /**
     * Load the current review for the branch, or return a fresh empty [Review]
     * if no review file exists yet. The store caches the result so subsequent
     * tool-window refreshes are cheap.
     */
    fun loadOrInit(repoRoot: String, branch: String): Review {
        val p = paths(repoRoot, branch)
        val key = cacheKey(p)
        cache[key]?.let { return it }
        val reviewPath = toPath(p.reviewFile())
        val review = if (Files.isRegularFile(reviewPath)) {
            try {
                Codec.decode(Files.readAllBytes(reviewPath))
            } catch (e: Exception) {
                log.warn("failed to decode existing review at $reviewPath; starting fresh", e)
                freshReview(branch)
            }
        } else {
            freshReview(branch)
        }
        cache[key] = review
        return review
    }

    /**
     * Append a new comment to the current review and persist atomically. Runs
     * on a background thread; caller MUST switch back to EDT before touching
     * UI. Returns the saved file path.
     *
     * Publishes [REVIEW_CHANGED_TOPIC] on success so subscribers (tool windows)
     * can auto-refresh without polling.
     */
    fun addComment(repoRoot: String, branch: String, mutate: (Review) -> Comment): SaveResult {
        val result = synchronized(lock) {
            val p = paths(repoRoot, branch)
            val review = loadOrInit(repoRoot, branch)
            val added = mutate(review)
            if (added.anchor.anchorId.isEmpty()) {
                added.anchor.anchorId = UUID.randomUUID().toString()
            }
            review.comments.add(added)
            saveReview(p, review)
        }
        if (result.succeeded) publishChanged(repoRoot, branch)
        return result
    }

    /**
     * Toggle the state of the comment whose anchor matches the given
     * predicate, or returns null if no such comment exists.
     *
     * Publishes [REVIEW_CHANGED_TOPIC] on success.
     */
    fun toggleComment(repoRoot: String, branch: String, predicate: (Comment) -> Boolean): SaveResult? {
        val result = synchronized(lock) {
            val p = paths(repoRoot, branch)
            val review = loadOrInit(repoRoot, branch)
            val target = review.comments.firstOrNull(predicate) ?: return null
            target.state = if (target.state == ReviewState.RESOLVED) ReviewState.OPEN else ReviewState.RESOLVED
            saveReview(p, review)
        }
        if (result.succeeded) publishChanged(repoRoot, branch)
        return result
    }

    /**
     * Remove the **first** comment whose anchor matches the given predicate and
     * persist atomically. Returns null if no comment matches (no-op), or the
     * [SaveResult] of the updated review on success.
     *
     * Only the first matching comment is removed to avoid bulk-deleting multiple
     * comments that share the same path/line when anchorId is absent.
     *
     * Publishes [REVIEW_CHANGED_TOPIC] only when [SaveResult.succeeded] is true,
     * consistent with [addComment] and [toggleComment].
     */
    fun removeComment(repoRoot: String, branch: String, predicate: (Comment) -> Boolean): SaveResult? {
        val result = synchronized(lock) {
            val p = paths(repoRoot, branch)
            val review = loadOrInit(repoRoot, branch)
            val idx = review.comments.indexOfFirst(predicate)
            if (idx < 0) return null
            review.comments.removeAt(idx)
            saveReview(p, review)
        }
        if (result.succeeded) publishChanged(repoRoot, branch)
        return result
    }

    /**
     * Atomically persist [review] to `<branchDir>/review.md`.
     *
     * - Empty review (no comments, blank review_comment) is a no-op; returns a
     *   [SaveResult] with an empty path.
     * - Existing `review.md` is backed up to `review.md.bak` before the new
     *   version is renamed into place.
     * - On encode failure, a rescue JSON file is written to
     *   `<branchDir>/review.md.rescue.<timestamp>.json` and a [RescueError] is
     *   returned via [SaveResult.error].
     *
     * Mirrors Go's `Store.SaveReview`.
     */
    fun saveReview(repoRoot: String, branch: String): SaveResult =
        synchronized(lock) {
            val p = paths(repoRoot, branch)
            val review = loadOrInit(repoRoot, branch)
            saveReview(p, review)
        }

    /**
     * Return the path of the current `review.md` if it exists, or null if none
     * is present. Mirrors Go's `Store.DetectReview`.
     */
    fun detectReview(repoRoot: String, branch: String): Path? {
        val p = paths(repoRoot, branch)
        val reviewPath = toPath(p.reviewFile())
        return if (Files.isRegularFile(reviewPath)) reviewPath else null
    }

    /**
     * Repair an incomplete write that left `review.md` missing but
     * `review.md.bak` present (crash window between bak-rename and
     * tmp-rename). Also cleans up orphaned `.tmp` files.
     *
     * Safe to call at startup: no-op when review.md exists or .bak is absent.
     * Mirrors Go's `Store.RecoverFromCrash`.
     */
    fun recoverFromCrash(repoRoot: String, branch: String) {
        val p = paths(repoRoot, branch)
        val branchDir = toPath(p.branchDir())

        // Clean up orphaned .tmp files.
        if (Files.isDirectory(branchDir)) {
            Files.list(branchDir).use { stream ->
                stream.filter { it.fileName.toString().endsWith(".tmp") }
                    .forEach { runCatching { Files.deleteIfExists(it) } }
            }
        }

        val final = toPath(p.reviewFile())
        val bak = toPath(p.bakFile())
        if (!Files.isRegularFile(final) && Files.isRegularFile(bak)) {
            try {
                Files.move(bak, final, StandardCopyOption.ATOMIC_MOVE)
                log.info("sitatame: crash recovery: restored review.md from review.md.bak")
            } catch (e: Exception) {
                log.warn("sitatame: crash recovery: rename review.md.bak -> review.md failed", e)
            }
        }
    }

    /** Snapshot of the cached comments for the tool window. */
    fun snapshotComments(repoRoot: String, branch: String): List<Comment> {
        val review = loadOrInit(repoRoot, branch)
        return review.comments.toList()
    }

    /** Resolve the branch directory path (`<OutputRoot>/<ProjectSlug>/<BranchSlug>/`). */
    fun branchDir(repoRoot: String, branch: String): String =
        paths(repoRoot, branch).branchDir()

    /** Reset the in-memory cache. Tests + Settings panel rely on this. */
    fun invalidate() {
        cache.clear()
    }

    // -- Internals -----------------------------------------------------------

    /**
     * Publish a [REVIEW_CHANGED_TOPIC] message on the application message bus.
     * Guarded by a runCatching so a missing ApplicationManager in tests does
     * not surface as an uncaught exception.
     */
    private fun publishChanged(repoRoot: String, branch: String) {
        runCatching {
            ApplicationManager.getApplication().messageBus
                .syncPublisher(REVIEW_CHANGED_TOPIC)
                .onChanged(repoRoot, branch)
        }
    }

    private fun freshReview(branch: String): Review {
        val now = LocalDateTime.now(clock).atOffset(ZoneOffset.UTC)
        return Review(
            schema = 1,
            id = "",
            createdAt = now.format(DateTimeFormatter.ISO_OFFSET_DATE_TIME),
            branch = branch,
        )
    }

    /**
     * Internal: atomically persist [review] to `<branchDir>/review.md`.
     * Caller must hold [lock].
     */
    private fun saveReview(p: SitatamePaths, review: Review): SaveResult {
        // Empty review: delete the existing file (if any) so invalidate + reload
        // does not resurrect stale comments from disk.
        if (review.comments.isEmpty() && review.reviewComment.trim().isEmpty()) {
            val reviewPath = toPath(p.reviewFile())
            val bakPath = toPath(p.bakFile())
            // Delete .bak first so that if we crash after this point there is
            // nothing for recoverFromCrash to resurrect from. If .bak deletion
            // fails, abort and return succeeded=false so the caller does not
            // publish REVIEW_CHANGED_TOPIC on an inconsistent state.
            try {
                Files.deleteIfExists(bakPath)
            } catch (e: Exception) {
                log.warn("failed to delete .bak file during empty-review cleanup: $bakPath", e)
                return SaveResult(path = "", id = review.id)
            }
            val deleted = Files.deleteIfExists(reviewPath)
            // Return the path that was deleted (or empty string when no file existed).
            return SaveResult(path = if (deleted) reviewPath.toString() else "", id = review.id)
        }

        if (review.id.isEmpty()) {
            review.id = generateId(review.reviewComment)
        }

        val branchDir = toPath(p.branchDir())
        Files.createDirectories(branchDir)
        tryRestrictPermissions(branchDir)

        val encode = encodeFunc ?: { r -> Codec.encode(r) }
        val bytes: ByteArray = try {
            encode(review)
        } catch (encodeErr: Exception) {
            // Rescue: write raw JSON so the user can recover content.
            val rescuePath = writeRescue(p, review, encodeErr)
            return SaveResult(path = "", id = review.id, error = RescueError(rescuePath, encodeErr))
        }

        val final = toPath(p.reviewFile())
        val bak = toPath(p.bakFile())

        // Step 1: write to a tmp file in branchDir (same filesystem → atomic rename).
        val tmp = Files.createTempFile(branchDir, ".review.", ".tmp")
        try {
            Files.write(tmp, bytes)
            // Step 2: back up existing review.md → review.md.bak.
            if (Files.isRegularFile(final)) {
                Files.move(final, bak, StandardCopyOption.REPLACE_EXISTING)
            }
            // Step 3: atomic rename tmp → review.md.
            Files.move(tmp, final, StandardCopyOption.ATOMIC_MOVE)
        } catch (e: Exception) {
            runCatching { Files.deleteIfExists(tmp) }
            throw e
        }

        cache[cacheKey(p)] = review
        return SaveResult(path = final.toString(), id = review.id)
    }

    /**
     * Write an in-memory Review as rescue JSON when Codec.encode fails.
     * Returns the written path, or an empty string on failure.
     *
     * Mirrors Go's `store.writeRescue`.
     */
    /**
     * Write an in-memory Review as rescue JSON when Codec.encode fails.
     * Returns the written path, or an empty string on failure.
     *
     * Uses [RescuePayload] + kotlinx.serialization-json for full Review
     * serialization, matching the schema produced by Go's `store.writeRescue`.
     */
    private fun writeRescue(p: SitatamePaths, review: Review, encodeErr: Exception): String {
        return try {
            val branchDir = toPath(p.branchDir())
            Files.createDirectories(branchDir)
            val now = LocalDateTime.now(clock).atOffset(ZoneOffset.UTC)
            val ts = now.format(DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmmss", Locale.ROOT))
            // Append nanoseconds (zero-padded to 9 digits) to prevent filename
            // collision when two Encode failures occur within the same second.
            // Mirrors Go's writeRescue nanos suffix. Glob review.md.rescue.*.json
            // is still satisfied.
            val nanos = String.format(Locale.ROOT, "%09d", now.nano)
            val filename = "review.md.rescue.$ts-$nanos.json"
            val rescuePath = branchDir.resolve(filename)

            val payload = RescuePayload(
                schema = "rescue/1",
                savedAt = now.format(DateTimeFormatter.ISO_OFFSET_DATE_TIME),
                reason = "yaml encode failed",
                originalEncodeError = encodeErr.message ?: "",
                review = review.toDto(),
            )
            val json = rescueJson.encodeToString(RescuePayload.serializer(), payload)
            Files.writeString(rescuePath, json)

            // Apply 0600 permissions to the rescue file so its content is
            // owner-private, matching Go's os.WriteFile(path, b, 0o600).
            // On Windows, POSIX permissions are unsupported; skip silently.
            try {
                Files.setPosixFilePermissions(
                    rescuePath,
                    PosixFilePermissions.fromString("rw-------")
                )
            } catch (_: UnsupportedOperationException) {
                // Non-POSIX FS (Windows) — nothing to do.
            }
            log.warn("sitatame: encode failed; rescue written to $rescuePath", encodeErr)
            rescuePath.toString()
        } catch (e: Exception) {
            log.error("sitatame: rescue write also failed", e)
            ""
        }
    }

    /**
     * Allocate a review id of the form `yyyyMMddTHHmmss-<slug>`. The id is
     * stored in review.md's `id:` field for cross-tool correlation.
     */
    private fun generateId(reviewComment: String): String {
        val ts = LocalDateTime.now(clock).atOffset(ZoneOffset.UTC)
            .format(DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmmss", Locale.ROOT))
        val slug = slugifyReviewComment(reviewComment)
        return "$ts-$slug"
    }

    private fun slugifyReviewComment(s: String): String {
        val first = s.substringBefore('\n').trim()
        if (first.isEmpty()) return "review"
        val sb = StringBuilder()
        for (r in first) {
            sb.append(
                when {
                    r in 'a'..'z' || r in 'A'..'Z' || r in '0'..'9' -> r
                    r == '.' || r == '_' || r == '-' -> r
                    else -> '_'
                }
            )
        }
        var out = sb.toString()
        if (out.length > 32) out = out.substring(0, 32)
        out = out.trim('_').trimStart('.')
        return if (out.isEmpty()) "review" else out
    }

    /**
     * Best-effort POSIX rwx------ on the branch dir. Mirrors Go's 0o700. On
     * Windows this is a silent no-op.
     */
    private fun tryRestrictPermissions(dir: Path) {
        try {
            val perms = PosixFilePermissions.fromString("rwx------")
            Files.setPosixFilePermissions(dir, perms)
        } catch (_: UnsupportedOperationException) {
            // Non-POSIX FS — Windows. Nothing to do.
        } catch (e: Exception) {
            log.debug("could not restrict permissions on $dir", e)
        }
    }

    data class SaveResult(
        val path: String,
        val id: String,
        val error: RescueError? = null,
    ) {
        val succeeded: Boolean get() = error == null && path.isNotEmpty()
    }

    /**
     * Returned (via [SaveResult.error]) when Codec.encode fails and a rescue
     * file was written. Mirrors Go's `review.RescueError`.
     */
    class RescueError(
        val rescuePath: String,
        val encodeErr: Exception,
    ) : Exception("encode failed (${encodeErr.message}); rescue written to $rescuePath", encodeErr)
}

/** Convenience: turn a File into a Path. */
internal fun fileToPath(f: File): Path = f.toPath()
