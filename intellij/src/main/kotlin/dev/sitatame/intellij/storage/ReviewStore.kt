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
 * The store is the single coordination point between the action layer
 * (AddComment / ResolveComment / Promote) and the tool window (which reads
 * the cached list to render the JBList). It deliberately keeps the cache
 * small — one [Review] per branch slug — because Phase 1 only deals with a
 * single in-progress draft at a time.
 *
 * Thread model:
 *  - Action layer runs on EDT (per the 2024.2+ threading rules) and dispatches
 *    file I/O off-EDT before returning to EDT to refresh the tool window. The
 *    store API itself is reentrant: every state read is a snapshot copy.
 *  - Mutations are serialised by a per-store lock so two background I/O
 *    threads writing different comments don't lose updates.
 */
@Service(Service.Level.APP)
class ReviewStore {

    private val log = Logger.getInstance(ReviewStore::class.java)
    private val lock = Any()

    /**
     * In-memory cache keyed by `<projectSlug>/<branchSlug>`. Holds the most
     * recently loaded or written draft Review for that branch.
     */
    private val cache = ConcurrentHashMap<String, Review>()

    var clock: Clock = Clock.systemUTC()

    private val settings: SitatameSettings
        get() = ApplicationManager.getApplication().getService(SitatameSettings::class.java)

    private fun paths(repoRoot: String, branch: String): SitatamePaths =
        PathsFactory.newPaths(repoRoot, branch, overrideHome = settings.state.sitatameHomeOverride)

    private fun cacheKey(p: SitatamePaths): String = "${p.projectSlug}/${p.slug}"

    /**
     * Load the most recently modified draft for the current branch, or return
     * a fresh empty [Review] if no draft exists. The store caches the result
     * so subsequent tool-window refreshes are cheap.
     */
    fun loadOrInitDraft(repoRoot: String, branch: String): Review {
        val p = paths(repoRoot, branch)
        val key = cacheKey(p)
        cache[key]?.let { return it }
        val latest = latestDraftPath(p)
        val review = if (latest != null) {
            try {
                Codec.decode(Files.readAllBytes(latest))
            } catch (e: Exception) {
                log.warn("failed to decode existing draft at $latest; starting fresh", e)
                freshReview(branch)
            }
        } else {
            freshReview(branch)
        }
        cache[key] = review
        return review
    }

    /**
     * Append a new comment to the current draft and persist atomically. Runs
     * on a background thread; caller MUST switch back to EDT before touching
     * UI. Returns the saved file path.
     */
    fun addComment(repoRoot: String, branch: String, mutate: (Review) -> Comment): SaveResult =
        synchronized(lock) {
            val p = paths(repoRoot, branch)
            val review = loadOrInitDraft(repoRoot, branch)
            val added = mutate(review)
            if (added.anchor.anchorId.isEmpty()) {
                added.anchor.anchorId = UUID.randomUUID().toString()
            }
            review.comments.add(added)
            persistDraft(p, review)
        }

    /**
     * Toggle the state of the comment whose anchor matches the given
     * predicate, or returns null if no such comment exists.
     */
    fun toggleComment(repoRoot: String, branch: String, predicate: (Comment) -> Boolean): SaveResult? =
        synchronized(lock) {
            val p = paths(repoRoot, branch)
            val review = loadOrInitDraft(repoRoot, branch)
            val target = review.comments.firstOrNull(predicate) ?: return null
            target.state = if (target.state == ReviewState.RESOLVED) ReviewState.OPEN else ReviewState.RESOLVED
            persistDraft(p, review)
        }

    /**
     * Persist any pending changes and return the saved file path. The store
     * does not auto-save on every mutation; the action layer triggers this
     * after a successful comment edit.
     */
    fun saveDraft(repoRoot: String, branch: String): SaveResult =
        synchronized(lock) {
            val p = paths(repoRoot, branch)
            val review = loadOrInitDraft(repoRoot, branch)
            persistDraft(p, review)
        }

    /**
     * Promote the most recent draft to reviews/. Returns the new path, or
     * null if there is no draft to promote.
     */
    fun promoteDraft(repoRoot: String, branch: String): String? =
        synchronized(lock) {
            val p = paths(repoRoot, branch)
            val draftPath = latestDraftPath(p) ?: return null
            Files.createDirectories(toPath(p.reviewsDir()))
            val id = draftPath.fileName.toString().removeSuffix(".md")
            val target = toPath(p.reviewFile(id))
            Files.move(draftPath, target, StandardCopyOption.ATOMIC_MOVE)
            target.toString()
        }

    /** Snapshot of the cached comments for the tool window. */
    fun snapshotComments(repoRoot: String, branch: String): List<Comment> {
        val review = loadOrInitDraft(repoRoot, branch)
        return review.comments.toList()
    }

    /** Resolve the directory where drafts for the current branch live. */
    fun draftsDir(repoRoot: String, branch: String): String =
        paths(repoRoot, branch).draftsDir()

    /** Resolve the directory where promoted reviews for the branch live. */
    fun reviewsDir(repoRoot: String, branch: String): String =
        paths(repoRoot, branch).reviewsDir()

    /** Reset the in-memory cache. Tests + Settings panel rely on this. */
    fun invalidate() {
        cache.clear()
    }

    // -- Internals -----------------------------------------------------------

    private fun freshReview(branch: String): Review {
        val now = LocalDateTime.now(clock).atOffset(ZoneOffset.UTC)
        return Review(
            schema = 1,
            id = "",
            createdAt = now.format(DateTimeFormatter.ISO_OFFSET_DATE_TIME),
            branch = branch,
        )
    }

    private fun persistDraft(p: SitatamePaths, review: Review): SaveResult {
        if (review.id.isEmpty()) {
            review.id = generateId(p, review.reviewComment)
        }
        val draftsDir = toPath(p.draftsDir())
        Files.createDirectories(draftsDir)
        tryRestrictPermissions(draftsDir)

        val final = toPath(p.draftFile(review.id))
        val bytes = Codec.encode(review)
        val tmp = Files.createTempFile(draftsDir, ".${review.id}.", ".tmp")
        try {
            Files.write(tmp, bytes)
            Files.move(tmp, final, StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
        } catch (e: Exception) {
            try { Files.deleteIfExists(tmp) } catch (_: Exception) { }
            throw e
        }
        cache[cacheKey(p)] = review
        return SaveResult(path = final.toString(), id = review.id)
    }

    private fun latestDraftPath(p: SitatamePaths): Path? {
        val dir = toPath(p.draftsDir())
        if (!Files.isDirectory(dir)) return null
        return Files.list(dir).use { stream ->
            stream
                .filter { Files.isRegularFile(it) && it.fileName.toString().endsWith(".md") }
                .toArray { arrayOfNulls<Path>(it) }
                .filterNotNull()
                .maxByOrNull { Files.getLastModifiedTime(it).toMillis() }
        }
    }

    /**
     * Allocate a draft id of the form `yyyyMMddTHHmmss-<slug>` and append
     * `-1`, `-2`, ... when the base is taken. Matches Go's [GenerateID]
     * semantics from `store.go`.
     */
    private fun generateId(p: SitatamePaths, reviewComment: String): String {
        val ts = LocalDateTime.now(clock).atOffset(ZoneOffset.UTC)
            .format(DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmmss", Locale.ROOT))
        val slug = slugifyReviewComment(reviewComment)
        val base = "$ts-$slug"
        if (!isIdTaken(p, base)) return base
        for (i in 1..99) {
            val cand = "$base-$i"
            if (!isIdTaken(p, cand)) return cand
        }
        error("could not allocate id under $base after 99 tries")
    }

    private fun isIdTaken(p: SitatamePaths, id: String): Boolean =
        Files.exists(toPath(p.draftFile(id))) || Files.exists(toPath(p.reviewFile(id)))

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
     * Best-effort POSIX rwx------ on the drafts/reviews dir. Mirrors the Go
     * side's 0o700. On Windows this is a silent no-op.
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

    data class SaveResult(val path: String, val id: String)
}

/** Convenience: turn a File into a Path. */
internal fun fileToPath(f: File): Path = f.toPath()
