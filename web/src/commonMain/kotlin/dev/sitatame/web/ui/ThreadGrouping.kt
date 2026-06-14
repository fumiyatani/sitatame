package dev.sitatame.web.ui

import dev.sitatame.web.api.CommentDto

// ---------------------------------------------------------------------------
// Thread key — canonical anchor identifier for grouping
// ---------------------------------------------------------------------------

/**
 * Structural key that uniquely identifies a comment anchor.
 *
 * Multiple [CommentDto] instances sharing the same [ThreadKey] form a
 * single reply thread.  Review-level comments have no [ThreadKey] and are
 * handled separately in the review-summary panel.
 */
sealed class ThreadKey {
    /** Anchored to a single diff line. */
    data class Line(val path: String, val side: String, val line: Int) : ThreadKey()

    /** Anchored to a contiguous range of diff lines. */
    data class Range(val path: String, val side: String, val lineStart: Int, val lineEnd: Int) : ThreadKey()

    /** Anchored to a whole file (no line). */
    data class File(val path: String) : ThreadKey()
}

// ---------------------------------------------------------------------------
// Thread state
// ---------------------------------------------------------------------------

/** Aggregate lifecycle state of a thread, derived from its comments. */
enum class ThreadState { Open, Stale, Resolved }

/**
 * A resolved thread: the canonical key, its ordered comments, and the
 * aggregate [ThreadState].
 */
data class Thread(
    val key: ThreadKey,
    val comments: List<CommentDto>,
    val state: ThreadState,
)

// ---------------------------------------------------------------------------
// Filter
// ---------------------------------------------------------------------------

/** Which threads to show in the diff view. */
enum class StateFilter { All, Open, Resolved, Stale }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Derives the [ThreadKey] for this comment.
 *
 * Returns null for review-level comments (kind = "review") and for any
 * comment whose anchor fields are incomplete (e.g. a line comment missing
 * [line] or [side]).
 */
fun CommentDto.threadKey(): ThreadKey? = when (kind) {
    "line" -> {
        val l = line ?: return null
        val s = side ?: return null
        val p = path ?: return null
        ThreadKey.Line(p, s, l)
    }
    "range" -> {
        val start = lineStart ?: return null
        val end = lineEnd ?: return null
        val s = side ?: return null
        val p = path ?: return null
        ThreadKey.Range(p, s, start, end)
    }
    "file" -> {
        val p = path ?: return null
        ThreadKey.File(p)
    }
    else -> null // "review" and unknown kinds
}

/**
 * Computes the aggregate [ThreadState] for a list of comments.
 *
 * Priority: Open > Stale > Resolved.  An empty list is treated as Resolved.
 */
fun computeThreadState(comments: List<CommentDto>): ThreadState = when {
    comments.any { it.state == "open" } -> ThreadState.Open
    comments.any { it.state == "stale" } -> ThreadState.Stale
    else -> ThreadState.Resolved
}

/**
 * Groups the comments anchored to [path] into ordered [Thread]s.
 *
 * - Comments whose [CommentDto.path] does not equal [path] are skipped.
 * - Review-level comments (kind = "review") are skipped.
 * - Thread order follows first appearance of each [ThreadKey] in the input.
 */
fun groupCommentsIntoThreads(comments: List<CommentDto>, path: String): List<Thread> {
    val ordered = LinkedHashMap<ThreadKey, MutableList<CommentDto>>()
    for (c in comments) {
        if (c.path != path) continue
        val key = c.threadKey() ?: continue
        ordered.getOrPut(key) { mutableListOf() }.add(c)
    }
    return ordered.map { (key, list) -> Thread(key, list, computeThreadState(list)) }
}

/**
 * Filters [threads] to only those matching [filter].
 *
 * [StateFilter.All] returns the original list unchanged.
 */
fun filterThreads(threads: List<Thread>, filter: StateFilter): List<Thread> = when (filter) {
    StateFilter.All -> threads
    StateFilter.Open -> threads.filter { it.state == ThreadState.Open }
    StateFilter.Resolved -> threads.filter { it.state == ThreadState.Resolved }
    StateFilter.Stale -> threads.filter { it.state == ThreadState.Stale }
}

/**
 * Returns true when [thread] should be collapsed by default.
 *
 * Only [ThreadState.Resolved] threads start collapsed; Open and Stale threads
 * are expanded so reviewers see them immediately.
 */
fun defaultCollapsed(thread: Thread): Boolean = thread.state == ThreadState.Resolved

// ---------------------------------------------------------------------------
// Reply helper
// ---------------------------------------------------------------------------

/**
 * Converts this [CommentDto] to a [CommentTarget] so that a reply form can
 * re-use the same anchor without re-parsing the diff row.
 *
 * Throws [IllegalArgumentException] for unknown [kind] values.
 */
fun CommentDto.toCommentTarget(): CommentTarget = when (kind) {
    "line" -> CommentTarget.Line(
        path = requireNotNull(path) { "line comment missing path" },
        line = requireNotNull(line) { "line comment missing line" },
        side = side ?: "head",
    )
    "range" -> CommentTarget.Range(
        path = requireNotNull(path) { "range comment missing path" },
        lineStart = requireNotNull(lineStart) { "range comment missing lineStart" },
        lineEnd = requireNotNull(lineEnd) { "range comment missing lineEnd" },
        side = side ?: "head",
    )
    "file" -> CommentTarget.File(
        path = requireNotNull(path) { "file comment missing path" },
    )
    "review" -> CommentTarget.Review
    else -> error("unknown kind: $kind")
}
