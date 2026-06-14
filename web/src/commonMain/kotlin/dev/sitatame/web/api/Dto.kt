package dev.sitatame.web.api

import kotlinx.serialization.Serializable

/**
 * Wire DTOs shared between the Ktor backend (jvmMain) and the Compose for Web
 * frontend (wasmJsMain).
 *
 * The shapes are deliberately denormalised relative to `internal/review/types.go`
 * because the frontend renders a unified diff view and never edits the YAML
 * tree directly. Anything write-path related (comment add / resolve toggle) is
 * Phase 1 step 2 and will land separate DTOs.
 */

/** A single inline comment on a review. */
@Serializable
data class CommentDto(
    val anchorId: String,
    /** "review" | "file" | "line" | "range" */
    val kind: String,
    val path: String,
    /** "head" | "base" (line / range anchors only). */
    val side: String? = null,
    val line: Int? = null,
    val lineStart: Int? = null,
    val lineEnd: Int? = null,
    /** "open" | "resolved" | "stale" */
    val state: String,
    val body: String,
)

/** A single line in a unified diff hunk. */
@Serializable
data class DiffLineDto(
    val baseLine: Int? = null,
    val headLine: Int? = null,
    /** "+", "-", " " */
    val prefix: String,
    val text: String,
)

/** A single hunk inside a file's unified diff. */
@Serializable
data class HunkDto(
    val baseStart: Int,
    val baseLines: Int,
    val headStart: Int,
    val headLines: Int,
    /** The raw `@@ ... @@` header line, used verbatim for display. */
    val header: String,
    val lines: List<DiffLineDto>,
)

/** A file-level entry that the sidebar lists and the main pane renders. */
@Serializable
data class FileDto(
    val path: String,
    /** "M" | "A" | "D" | "R" — single-letter git status. */
    val status: String,
    val renameFrom: String? = null,
    val renameTo: String? = null,
    val adds: Int,
    val dels: Int,
    val hunks: List<HunkDto>,
    /**
     * Abbreviated git blob SHA for the base side of the diff. Sourced from the
     * `index <blobBase>..<blobHead>` line in the unified-diff output.  Null when
     * the file is new (no base-side blob) or when the index line is absent.
     * The Frontend uses this value to populate [CreateCommentRequest.blob] when
     * [CreateCommentRequest.side] is "base".
     */
    val blobBase: String? = null,
    /**
     * Abbreviated git blob SHA for the head side of the diff. Null when the file
     * is deleted (no head-side blob).
     * The Frontend uses this value to populate [CreateCommentRequest.blob] when
     * [CreateCommentRequest.side] is "head".
     */
    val blobHead: String? = null,
)

/** The latest review document for the current branch (or null when none). */
@Serializable
data class ReviewDto(
    val id: String,
    val branch: String,
    val baseRef: String,
    val headRef: String,
    val reviewComment: String? = null,
    val comments: List<CommentDto>,
)

/** Top-level workspace payload returned by `GET /api/v1/workspace`. */
@Serializable
data class WorkspaceResponse(
    val projectSlug: String,
    val branch: String,
    val files: List<FileDto>,
    /** null when no review .md exists yet under this branch slug. */
    val review: ReviewDto? = null,
    /** Absolute path of the review .md file, or null. */
    val sourcePath: String? = null,
)

// ---------------------------------------------------------------------------
// Write-path DTOs (Phase 1 step 2)
// ---------------------------------------------------------------------------

/**
 * Request body for `POST /api/v1/comments`.
 *
 * [kind] must be one of "line" | "range" | "file" | "review".
 * [side] must be "head" (default) or "base".
 * [blob] is the git blob SHA that identifies the file version the comment
 * is anchored to. The Frontend should pass the BlobHead / BlobBase it
 * received from GET /workspace. Omit when kind == "review".
 */
@Serializable
data class CreateCommentRequest(
    val kind: String,
    val path: String? = null,
    val side: String = "head",
    val blob: String? = null,
    val line: Int? = null,
    val lineStart: Int? = null,
    val lineEnd: Int? = null,
    val body: String,
)

/**
 * Request body for `PATCH /api/v1/comments/{anchorId}/state`.
 *
 * [state] must be one of "open" | "resolved" | "stale".
 */
@Serializable
data class UpdateCommentStateRequest(val state: String)

/**
 * Request body for `PUT /api/v1/review-comment`.
 *
 * [text] is the new review-level comment. An empty string clears the field.
 */
@Serializable
data class UpdateReviewCommentRequest(val text: String)

/**
 * Response body for HTTP 412 Precondition Failed (ETag mismatch).
 *
 * [expected] is the ETag the client sent in `If-Match`.
 * [actual] is the current ETag of the review file on disk.
 */
@Serializable
data class EtagMismatchError(
    val error: String = "etag_mismatch",
    val expected: String,
    val actual: String,
)
