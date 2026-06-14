package dev.sitatame.web.ui

/**
 * Sealed hierarchy describing what a comment will be anchored to.
 *
 * Used as the data passed from the diff view to [CommentModal] and
 * subsequently converted to a [dev.sitatame.web.api.CreateCommentRequest].
 */
sealed interface CommentTarget {
    /** A single diff line (kind = "line"). */
    data class Line(
        val path: String,
        val line: Int,
        /** "head" or "base" — derived from the diff row prefix. */
        val side: String,
    ) : CommentTarget

    /** A range of diff lines within a single hunk (kind = "range"). */
    data class Range(
        val path: String,
        val lineStart: Int,
        val lineEnd: Int,
        /** "head" or "base" — all lines in the range must share the same side. */
        val side: String,
    ) : CommentTarget

    /** A whole-file comment, not anchored to any line (kind = "file"). */
    data class File(val path: String) : CommentTarget

    /** Review-level overall comment (kind = "review"). */
    data object Review : CommentTarget
}

/** Human-readable label for a [CommentTarget], shown in the modal header. */
fun CommentTarget.label(): String = when (this) {
    is CommentTarget.Line -> "Line $line · ${path.substringAfterLast('/')}"
    is CommentTarget.Range -> "Lines $lineStart-$lineEnd · ${path.substringAfterLast('/')}"
    is CommentTarget.File -> "File comment · ${path.substringAfterLast('/')}"
    CommentTarget.Review -> "Overall review comment"
}
