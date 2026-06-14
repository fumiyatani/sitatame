package dev.sitatame.intellij.storage

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * JSON payload written to the rescue file when [Codec.encode] fails.
 *
 * Mirrors Go's `rescuePayload` struct in `internal/review/store.go`:
 *
 * ```go
 * type rescuePayload struct {
 *     Schema              string  `json:"schema"`
 *     SavedAt             string  `json:"saved_at"`
 *     Reason              string  `json:"reason"`
 *     OriginalEncodeError string  `json:"original_encode_error"`
 *     Review              *Review `json:"review"`
 * }
 * ```
 *
 * Key names and structure are intentionally kept identical to Go so that
 * a Go-side parser can read Kotlin-generated rescue files and vice versa.
 *
 * The nested [ReviewDto] is a flat projection of [Review]: mutable models
 * that contain snakeyaml [org.snakeyaml.engine.v2.nodes.Node] references
 * cannot be serialized directly.
 */
@Serializable
data class RescuePayload(
    val schema: String,
    @SerialName("saved_at") val savedAt: String,
    val reason: String,
    @SerialName("original_encode_error") val originalEncodeError: String,
    val review: ReviewDto,
)

/**
 * Serializable projection of [Review]. Omits the snakeyaml [Node] extras
 * maps which are not JSON-serializable.
 *
 * Field names use snake_case via [SerialName] to match the Go JSON schema.
 */
@Serializable
data class ReviewDto(
    val schema: Int,
    val id: String,
    @SerialName("created_at") val createdAt: String,
    val branch: String,
    val base: RefDto,
    val head: RefDto,
    val files: List<FileMetaDto>,
    @SerialName("review_comment") val reviewComment: String,
    val comments: List<CommentDto>,
)

@Serializable
data class RefDto(
    val ref: String,
    val sha: String,
)

@Serializable
data class FileMetaDto(
    val path: String,
    @SerialName("blob_base") val blobBase: String,
    @SerialName("blob_head") val blobHead: String,
    val status: String,
    @SerialName("rename_from") val renameFrom: String,
    @SerialName("rename_to") val renameTo: String,
    val similarity: Int,
)

@Serializable
data class AnchorDto(
    @SerialName("anchor_id") val anchorId: String,
    val kind: String,
    val path: String,
    val side: String,
    val blob: String,
    val line: Int,
    @SerialName("line_start") val lineStart: Int,
    @SerialName("line_end") val lineEnd: Int,
    @SerialName("rename_from") val renameFrom: String,
    @SerialName("rename_to") val renameTo: String,
    val similarity: Int,
)

@Serializable
data class CommentDto(
    val anchor: AnchorDto,
    val state: String,
    val body: String,
)

// -- Conversion helpers ------------------------------------------------------

internal fun Review.toDto(): ReviewDto = ReviewDto(
    schema = schema,
    id = id,
    createdAt = createdAt,
    branch = branch,
    base = RefDto(ref = base.ref, sha = base.sha),
    head = RefDto(ref = head.ref, sha = head.sha),
    files = files.map { f ->
        FileMetaDto(
            path = f.path,
            blobBase = f.blobBase,
            blobHead = f.blobHead,
            status = f.status,
            renameFrom = f.renameFrom,
            renameTo = f.renameTo,
            similarity = f.similarity,
        )
    },
    reviewComment = reviewComment,
    comments = comments.map { c ->
        CommentDto(
            anchor = AnchorDto(
                anchorId = c.anchor.anchorId,
                kind = c.anchor.kind,
                path = c.anchor.path,
                side = c.anchor.side,
                blob = c.anchor.blob,
                line = c.anchor.line,
                lineStart = c.anchor.lineStart,
                lineEnd = c.anchor.lineEnd,
                renameFrom = c.anchor.renameFrom,
                renameTo = c.anchor.renameTo,
                similarity = c.anchor.similarity,
            ),
            state = c.state,
            body = c.body,
        )
    },
)

/** JSON instance shared by rescue serialization. Pretty-printed. */
internal val rescueJson = Json { prettyPrint = true }
