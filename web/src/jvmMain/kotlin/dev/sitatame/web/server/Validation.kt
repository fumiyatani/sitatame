package dev.sitatame.web.server

import dev.sitatame.web.api.CreateCommentRequest
import dev.sitatame.web.api.FileDto

/**
 * Input validation for write-path requests.
 *
 * Shape + semantic rules are ported from Go `internal/review/validate.go`
 * and `internal/tui/modal.go`.
 *
 * Blob semantic checks: when [files] is provided, the [CreateCommentRequest.blob]
 * value is compared against the file entry's [FileDto.blobBase] or
 * [FileDto.blobHead] (depending on [CreateCommentRequest.side]).  A mismatch
 * indicates a stale comment anchor (the diff has changed since the Frontend
 * loaded it) and is returned as a 422 error.  The comparison uses a
 * prefix-match strategy because the diff `index` header uses abbreviated SHAs
 * (typically 7 characters) while clients may supply the full 40-character SHA
 * or vice-versa.
 */
object Validation {

    /** Allowed kind values. */
    private val VALID_KINDS = setOf("review", "file", "line", "range")

    /** Allowed side values. */
    private val VALID_SIDES = setOf("head", "base")

    /** Allowed state values for UpdateCommentStateRequest. */
    val VALID_STATES = setOf("open", "resolved", "stale")

    /**
     * Validate a CreateCommentRequest.
     *
     * Returns an empty list on success. Non-empty list contains human-readable
     * error strings suitable for a 422 response body.
     *
     * [files] is optional.  When provided and [CreateCommentRequest.blob] is
     * non-null, a blob-integrity check is performed: the supplied blob SHA must
     * be a prefix of (or equal to) the current diff's blob SHA for the same
     * path/side.  A mismatch means the diff has changed since the Frontend
     * loaded the workspace and the anchor is stale.
     */
    fun validate(req: CreateCommentRequest, files: List<FileDto>? = null): List<String> {
        val errors = mutableListOf<String>()

        // kind
        if (req.kind !in VALID_KINDS) {
            errors.add("kind must be one of: ${VALID_KINDS.sorted().joinToString()}, got \"${req.kind}\"")
        }

        // body
        if (req.body.isBlank()) {
            errors.add("body must not be empty")
        }

        when (req.kind) {
            "line" -> {
                if (req.path.isNullOrBlank()) {
                    errors.add("path is required for kind=line")
                }
                if (req.line == null) {
                    errors.add("line is required for kind=line")
                } else if (req.line <= 0) {
                    errors.add("line must be positive, got ${req.line}")
                }
                if (req.side !in VALID_SIDES) {
                    errors.add("side must be 'head' or 'base' for kind=line, got \"${req.side}\"")
                }
                // line_start / line_end must not be present
                if (req.lineStart != null || req.lineEnd != null) {
                    errors.add("line_start and line_end must not be set for kind=line")
                }
            }

            "range" -> {
                if (req.path.isNullOrBlank()) {
                    errors.add("path is required for kind=range")
                }
                if (req.lineStart == null) {
                    errors.add("line_start is required for kind=range")
                } else if (req.lineStart <= 0) {
                    errors.add("line_start must be positive, got ${req.lineStart}")
                }
                if (req.lineEnd == null) {
                    errors.add("line_end is required for kind=range")
                } else if (req.lineEnd <= 0) {
                    errors.add("line_end must be positive, got ${req.lineEnd}")
                }
                if (req.lineStart != null && req.lineEnd != null && req.lineStart > req.lineEnd) {
                    errors.add("line_start (${req.lineStart}) must be <= line_end (${req.lineEnd})")
                }
                if (req.side !in VALID_SIDES) {
                    errors.add("side must be 'head' or 'base' for kind=range, got \"${req.side}\"")
                }
                // line must not be present
                if (req.line != null) {
                    errors.add("line must not be set for kind=range (use line_start/line_end)")
                }
            }

            "file" -> {
                if (req.path.isNullOrBlank()) {
                    errors.add("path is required for kind=file")
                }
                // line / line_start / line_end must not be present
                if (req.line != null || req.lineStart != null || req.lineEnd != null) {
                    errors.add("line, line_start, and line_end must not be set for kind=file")
                }
            }

            "review" -> {
                // No path or line constraints for review-level comments.
                // side is ignored.
                if (req.path != null && req.path.isNotBlank()) {
                    // Allow path for review kind (ignored) — do not error.
                }
            }
        }

        // Blob integrity check: when the caller supplies a blob SHA and we have
        // workspace file data, verify the blob matches the current diff.
        if (req.blob != null && files != null && req.path != null) {
            val fileEntry = files.firstOrNull { it.path == req.path }
            if (fileEntry != null) {
                val currentBlob = if (req.side == "base") fileEntry.blobBase else fileEntry.blobHead
                if (currentBlob != null && !blobShasCompatible(req.blob, currentBlob)) {
                    errors.add(
                        "blob mismatch for ${req.path} (side=${req.side}): " +
                            "client sent ${req.blob}, current diff has $currentBlob — " +
                            "the diff may have changed since you opened the page (stale anchor)"
                    )
                }
            }
        }

        return errors
    }

    /**
     * Returns true when two blob SHAs refer to the same object.
     *
     * Git abbreviated SHAs are prefixes of the full 40-character SHA. We
     * accept a match when either value is a prefix of the other. This covers
     * the case where the diff index line yields 7-char abbreviations and the
     * client sends them back as-is.
     */
    internal fun blobShasCompatible(a: String, b: String): Boolean {
        if (a == b) return true
        val (shorter, longer) = if (a.length <= b.length) a to b else b to a
        return longer.startsWith(shorter, ignoreCase = true)
    }

    /**
     * Validate a state transition value.
     *
     * Returns an empty list on success. Non-empty list contains error strings
     * for a 422 response.
     */
    fun validateState(state: String): List<String> {
        return if (state in VALID_STATES) emptyList()
        else listOf("state must be one of: ${VALID_STATES.sorted().joinToString()}, got \"$state\"")
    }

    /**
     * Validate a UUID v4 string.
     *
     * A minimal check: 8-4-4-4-12 hex with the version nibble = 4.
     * Returns true when the string is a valid UUID v4.
     */
    fun isValidUuidV4(id: String): Boolean {
        val uuidRegex = Regex(
            "^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$",
            RegexOption.IGNORE_CASE,
        )
        return uuidRegex.matches(id)
    }
}
