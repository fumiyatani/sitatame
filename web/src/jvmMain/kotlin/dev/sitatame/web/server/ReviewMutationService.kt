package dev.sitatame.web.server

import dev.sitatame.web.api.CreateCommentRequest
import dev.sitatame.web.api.FileDto
import dev.sitatame.web.api.UpdateCommentStateRequest
import dev.sitatame.web.api.UpdateReviewCommentRequest
import dev.sitatame.web.roundtrip.Codec
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.nio.file.Files
import java.nio.file.Path
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap

/**
 * Handles mutating write operations on review.md.
 *
 * Each write operation is atomic at the path level:
 *  1. Acquire a per-path Mutex so concurrent requests are serialised.
 *  2. Read the current file bytes and compute the current ETag.
 *  3. Compare with the If-Match value supplied by the caller.
 *  4. If they match, apply the mutation via [Codec].
 *  5. Validate input (shape + semantic checks via [Validation]).
 *  6. Write atomically (tmp file → fsync → rename-to-bak → rename-to-final).
 *  7. Return the new ETag.
 *
 * The path-scoped [Mutex] lives inside a [ConcurrentHashMap] keyed on the
 * absolute [Path] of the review.md file. A new Mutex is created lazily on
 * the first mutation for a given path.
 */
class ReviewMutationService(
    private val paths: SitatamePaths,
) {

    private val locks = ConcurrentHashMap<Path, Mutex>()

    // -----------------------------------------------------------------------
    // Public API
    // -----------------------------------------------------------------------

    /**
     * Add a new comment to the review.
     *
     * [ifMatch] is the ETag value from the client's `If-Match` header.
     * [workspaceFiles] is the current list of diff files from the workspace
     * snapshot.  When provided, blob integrity is checked against the diff
     * index data (see [Validation.validate]).
     */
    suspend fun addComment(
        req: CreateCommentRequest,
        ifMatch: String,
        workspaceFiles: List<FileDto>? = null,
    ): MutationResult {
        val validationErrors = Validation.validate(req, workspaceFiles)
        if (validationErrors.isNotEmpty()) {
            return MutationResult.ValidationError(validationErrors)
        }

        val reviewPath = paths.reviewFile()
        return withPathLock(reviewPath) {
            val currentBytes = readBytes(reviewPath)
            val currentEtag = computeEtag(currentBytes)

            if (!etagMatches(ifMatch, currentEtag)) {
                return@withPathLock MutationResult.EtagMismatch(
                    expected = ifMatch,
                    actual = currentEtag,
                )
            }

            val anchorId = UUID.randomUUID().toString()
            val (newBytes, added) = Codec.addComment(currentBytes, req, anchorId)

            ensureDirExists(reviewPath.parent)
            writeAtomic(reviewPath, newBytes)

            MutationResult.Success(
                newEtag = computeEtag(newBytes),
                anchorId = added.anchorId,
            )
        }
    }

    /**
     * Change the state of a comment identified by [anchorId].
     *
     * [ifMatch] is the ETag value from the client's `If-Match` header.
     */
    suspend fun updateState(anchorId: String, req: UpdateCommentStateRequest, ifMatch: String): MutationResult {
        val stateErrors = Validation.validateState(req.state)
        if (stateErrors.isNotEmpty()) {
            return MutationResult.ValidationError(stateErrors)
        }

        val reviewPath = paths.reviewFile()
        if (!Files.isRegularFile(reviewPath)) {
            return MutationResult.NotFound("review.md does not exist")
        }

        return withPathLock(reviewPath) {
            val currentBytes = readBytes(reviewPath)
            val currentEtag = computeEtag(currentBytes)

            if (!etagMatches(ifMatch, currentEtag)) {
                return@withPathLock MutationResult.EtagMismatch(
                    expected = ifMatch,
                    actual = currentEtag,
                )
            }

            val newBytes = try {
                Codec.updateCommentState(currentBytes, anchorId, req.state)
            } catch (e: IllegalArgumentException) {
                return@withPathLock MutationResult.NotFound(e.message ?: "comment not found")
            }

            writeAtomic(reviewPath, newBytes)
            MutationResult.Success(newEtag = computeEtag(newBytes))
        }
    }

    /**
     * Replace the top-level review-level comment.
     *
     * [ifMatch] is the ETag value from the client's `If-Match` header.
     */
    suspend fun updateReviewComment(req: UpdateReviewCommentRequest, ifMatch: String): MutationResult {
        val reviewPath = paths.reviewFile()
        if (!Files.isRegularFile(reviewPath)) {
            return MutationResult.NotFound("review.md does not exist")
        }

        return withPathLock(reviewPath) {
            val currentBytes = readBytes(reviewPath)
            val currentEtag = computeEtag(currentBytes)

            if (!etagMatches(ifMatch, currentEtag)) {
                return@withPathLock MutationResult.EtagMismatch(
                    expected = ifMatch,
                    actual = currentEtag,
                )
            }

            val newBytes = Codec.updateReviewComment(currentBytes, req.text)
            writeAtomic(reviewPath, newBytes)
            MutationResult.Success(newEtag = computeEtag(newBytes))
        }
    }

    // -----------------------------------------------------------------------
    // Internal helpers
    // -----------------------------------------------------------------------

    private suspend fun <T> withPathLock(path: Path, block: suspend () -> T): T {
        val mutex = locks.computeIfAbsent(path.toAbsolutePath().normalize()) { Mutex() }
        return mutex.withLock { block() }
    }

    /** Read bytes from [path]; return empty array when the file does not exist. */
    private fun readBytes(path: Path): ByteArray =
        if (Files.isRegularFile(path)) Files.readAllBytes(path) else ByteArray(0)

    /**
     * Compare ETags with and without surrounding double-quotes.
     *
     * HTTP `If-Match` allows both `"sha256-..."` and `sha256-...`. We
     * normalise both forms before comparing so clients that strip quotes are
     * also handled.
     */
    private fun etagMatches(clientEtag: String, serverEtag: String): Boolean {
        val norm = { s: String -> s.trim().removeSurrounding("\"") }
        return norm(clientEtag) == norm(serverEtag)
    }

    private fun ensureDirExists(dir: Path?) {
        if (dir != null) Files.createDirectories(dir)
    }

    /**
     * Atomic write: tmp → fsync → rename-bak → rename-final.
     *
     * Mirrors Go's `internal/review/store.go#SaveReview` write sequence.
     */
    private fun writeAtomic(reviewPath: Path, bytes: ByteArray) {
        val dir = reviewPath.parent
        ensureDirExists(dir)

        val tmp = Files.createTempFile(dir, ".review.", ".tmp")
        try {
            // Write and fsync via a single FileOutputStream so the channel
            // covers the data we just wrote. Do NOT use Files.write() followed
            // by a separate outputStream() call — the second open truncates the
            // file before force() runs, producing an empty review.md.
            java.io.FileOutputStream(tmp.toFile()).use { fos ->
                fos.write(bytes)
                fos.flush()
                fos.fd.sync()
            }

            val bak = reviewPath.resolveSibling("review.md.bak")
            if (Files.isRegularFile(reviewPath)) {
                Files.move(reviewPath, bak, java.nio.file.StandardCopyOption.REPLACE_EXISTING)
            }
            Files.move(tmp, reviewPath)

            // Best-effort: fsync the directory so the rename is durable.
            try {
                java.io.FileInputStream(dir.toFile()).use { it.fd.sync() }
            } catch (_: Exception) {
                // Directory fsync is not supported on all platforms (e.g.
                // macOS may return EINVAL for directories). Ignore.
            }
        } catch (e: Exception) {
            runCatching { Files.deleteIfExists(tmp) }
            throw e
        }
    }
}

/** Result of a mutation operation. */
sealed class MutationResult {
    data class Success(val newEtag: String, val anchorId: String? = null) : MutationResult()
    data class EtagMismatch(val expected: String, val actual: String) : MutationResult()
    data class ValidationError(val errors: List<String>) : MutationResult()
    data class NotFound(val message: String) : MutationResult()
}
