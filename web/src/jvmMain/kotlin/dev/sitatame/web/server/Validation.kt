package dev.sitatame.web.server

import dev.sitatame.web.api.CreateCommentRequest

/**
 * Input validation for write-path requests.
 *
 * Shape + semantic rules are ported from Go `internal/review/validate.go`
 * and `internal/tui/modal.go`. The Backend trusts that the Frontend sends
 * correct side/blob metadata (Frontend controls the diff view), so blob
 * semantic checks (stale detection) are not performed here; they run at
 * GET /workspace time via ReviewLoader.
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
     */
    fun validate(req: CreateCommentRequest): List<String> {
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

        return errors
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
