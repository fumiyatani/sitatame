package dev.sitatame.intellij.storage

import org.snakeyaml.engine.v2.nodes.Node

/**
 * Port of `internal/review/types.go`.
 *
 * These are mutable data containers, not data classes: the action layer
 * mutates them in place when toggling comment state or appending new
 * comments, and the codec writes them back. Using `var` everywhere mirrors
 * the Go side's struct semantics more faithfully than tossing around immutable
 * `copy()` calls would.
 */

/** Review document state constants. Matches Go's `State` enum strings. */
object ReviewState {
    const val OPEN = "open"
    const val RESOLVED = "resolved"
    const val STALE = "stale"

    fun valid(s: String): Boolean = s == OPEN || s == RESOLVED || s == STALE
}

/** Anchor kind constants. Matches Go's `Kind` enum strings. */
object AnchorKind {
    const val REVIEW = "review"
    const val FILE = "file"
    const val LINE = "line"
    const val RANGE = "range"

    fun valid(k: String): Boolean = k == REVIEW || k == FILE || k == LINE || k == RANGE
}

/** Anchor side constants. Matches Go's `Side` enum strings. */
object AnchorSide {
    const val HEAD = "head"
    const val BASE = "base"
}

class Ref(
    var ref: String = "",
    var sha: String = "",
)

class FileMeta(
    var path: String = "",
    var blobBase: String = "",
    var blobHead: String = "",
    var status: String = "",
    var renameFrom: String = "",
    var renameTo: String = "",
    var similarity: Int = 0,
) {
    /** Unknown YAML keys preserved for round-trip. */
    val extras: MutableMap<String, Node> = linkedMapOf()
}

class Anchor(
    var anchorId: String = "",
    var kind: String = AnchorKind.LINE,
    var path: String = "",
    var side: String = AnchorSide.HEAD,
    var blob: String = "",
    var line: Int = 0,
    var lineStart: Int = 0,
    var lineEnd: Int = 0,
    var renameFrom: String = "",
    var renameTo: String = "",
    var similarity: Int = 0,
)

class Comment(
    var anchor: Anchor = Anchor(),
    var state: String = ReviewState.OPEN,
    var body: String = "",
) {
    val extras: MutableMap<String, Node> = linkedMapOf()
}

class Review(
    var schema: Int = 1,
    var id: String = "",
    var createdAt: String = "",
    var branch: String = "",
    var base: Ref = Ref(),
    var head: Ref = Ref(),
    var files: MutableList<FileMeta> = mutableListOf(),
    var reviewComment: String = "",
    var comments: MutableList<Comment> = mutableListOf(),
    var body: String = "",
) {
    val extras: MutableMap<String, Node> = linkedMapOf()
}
