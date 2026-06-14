package dev.sitatame.web.ui

import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import dev.sitatame.web.api.CommentDto
import dev.sitatame.web.api.CreateCommentRequest
import dev.sitatame.web.api.EtagMismatchError
import dev.sitatame.web.api.WorkspaceResponse

// ---------------------------------------------------------------------------
// Pending mutation types
// ---------------------------------------------------------------------------

/** A mutation that was in flight when the server responded with 412. */
sealed interface PendingMutation {
    data class AddComment(val request: CreateCommentRequest) : PendingMutation
    data class ToggleState(val anchorId: String, val newState: String) : PendingMutation
    data class UpdateReviewComment(val text: String) : PendingMutation
}

/** What the UI should show while a mutation is pending. */
sealed interface ConflictState {
    data class EtagConflict(
        val error: EtagMismatchError,
        val serverEtag: String,
        val pending: PendingMutation,
    ) : ConflictState
}

// ---------------------------------------------------------------------------
// Optimistic comment state for resolve toggle
// ---------------------------------------------------------------------------

/**
 * Holds a local override for a single comment's state, shown while the PATCH
 * call is in flight.  [original] is the last confirmed server state so we can
 * roll back on failure.
 */
data class OptimisticCommentState(
    val anchorId: String,
    val optimisticState: String,
    val original: String,
)

// ---------------------------------------------------------------------------
// Holder
// ---------------------------------------------------------------------------

/**
 * [Stable] holder for ETag + workspace snapshot + in-flight mutation state.
 *
 * Owned by [SitatameApp] and threaded down to composables that need to read
 * or trigger mutations.  All mutations go through this holder so the ETag is
 * always current.
 */
@Stable
class ReviewStateHolder {
    /** The current workspace snapshot (null while loading). */
    var snapshot by mutableStateOf<WorkspaceResponse?>(null)

    /** Latest known ETag for review.md (sentinel "empty" when no review yet). */
    var etag by mutableStateOf<String>("empty")

    /** Pending mutation waiting for conflict resolution. */
    var conflict by mutableStateOf<ConflictState?>(null)

    /** Per-comment optimistic state while PATCH is in flight. */
    var optimisticStates by mutableStateOf<Map<String, OptimisticCommentState>>(emptyMap())

    /** Toast messages to display briefly (error feedback). */
    var toastMessage by mutableStateOf<String?>(null)

    /**
     * Current thread filter selection.  Defaults to [StateFilter.All].
     * UI wiring (StateFilterControl composable) is Phase B.
     */
    var filterState by mutableStateOf(StateFilter.All)

    /** True while a resolve-toggle PATCH is in flight for a specific anchorId. */
    var pendingToggleIds by mutableStateOf<Set<String>>(emptySet())

    /**
     * Returns the effective state for a comment: the optimistic override if one
     * is in flight, otherwise the server-confirmed state from [snapshot].
     */
    fun effectiveState(comment: CommentDto): String =
        optimisticStates[comment.anchorId]?.optimisticState ?: comment.state

    /**
     * Returns the full comment list from [snapshot], with optimistic state
     * overrides applied.
     */
    fun effectiveComments(): List<CommentDto> =
        snapshot?.review?.comments.orEmpty().map { c ->
            optimisticStates[c.anchorId]?.let { opt ->
                c.copy(state = opt.optimisticState)
            } ?: c
        }

    /** Updates [snapshot] and [etag] from a successful GET /workspace response. */
    fun applyWorkspace(response: EtaggedResponse<WorkspaceResponse>) {
        snapshot = response.value
        etag = response.etag
        optimisticStates = emptyMap()
    }

    /** Clears a conflict after the user resolves it. */
    fun clearConflict() {
        conflict = null
    }

    /** Sets an optimistic state and records the original for rollback. */
    fun setOptimistic(anchorId: String, newState: String, original: String) {
        optimisticStates = optimisticStates + (
            anchorId to OptimisticCommentState(anchorId, newState, original)
        )
        pendingToggleIds = pendingToggleIds + anchorId
    }

    /** Confirms an optimistic update (removes override since server confirmed). */
    fun confirmOptimistic(anchorId: String, newEtag: String) {
        etag = newEtag
        // Capture the optimistic entry before removing it so the snapshot patch
        // below can still read the confirmed state value.
        val opt = optimisticStates[anchorId] ?: return
        optimisticStates = optimisticStates - anchorId
        pendingToggleIds = pendingToggleIds - anchorId
        snapshot = snapshot?.let { ws ->
            ws.copy(
                review = ws.review?.let { rev ->
                    rev.copy(
                        comments = rev.comments.map { c ->
                            if (c.anchorId == anchorId) c.copy(state = opt.optimisticState) else c
                        },
                    )
                },
            )
        }
    }

    /** Rolls back an optimistic update and shows a toast. */
    fun rollbackOptimistic(anchorId: String, message: String) {
        optimisticStates = optimisticStates - anchorId
        pendingToggleIds = pendingToggleIds - anchorId
        toastMessage = message
    }

    /** Applies a successfully created comment to the local snapshot. */
    fun applyNewComment(comment: CommentDto, newEtag: String) {
        etag = newEtag
        snapshot = snapshot?.let { ws ->
            ws.copy(
                review = ws.review?.let { rev ->
                    rev.copy(comments = rev.comments + comment)
                },
            )
        }
    }

    /** Applies an updated review comment text to the local snapshot. */
    fun applyReviewCommentUpdate(text: String, newEtag: String) {
        etag = newEtag
        snapshot = snapshot?.let { ws ->
            ws.copy(
                review = ws.review?.let { rev ->
                    rev.copy(reviewComment = text.ifBlank { null })
                },
            )
        }
    }

    fun dismissToast() {
        toastMessage = null
    }
}
