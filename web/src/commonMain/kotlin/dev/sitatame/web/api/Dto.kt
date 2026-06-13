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
