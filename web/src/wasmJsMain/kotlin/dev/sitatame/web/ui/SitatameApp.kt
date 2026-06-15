package dev.sitatame.web.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.sitatame.web.api.CommentDto
import dev.sitatame.web.api.CreateCommentRequest
import dev.sitatame.web.api.UpdateCommentStateRequest
import dev.sitatame.web.api.UpdateReviewCommentRequest
import dev.sitatame.web.api.WorkspaceResponse
import kotlinx.coroutines.launch

/**
 * Root composable. Owns the load -> render state machine and the GitHub-style
 * 2-pane layout.
 *
 *  - top bar (branch + base/head refs + "Add overall comment" button)
 *  - ReviewSummaryPanel (review-level feedback + edit button)
 *  - row { sidebar | main pane }
 *  - status bar
 *  - ConflictModal (shown on 412)
 *  - Toast (shown on mutation error)
 */
@Composable
fun SitatameApp() {
    SitatameTheme {
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = MaterialTheme.colorScheme.background,
        ) {
            val holder = remember { ReviewStateHolder() }
            var loadError by remember { mutableStateOf<String?>(null) }

            LaunchedEffect(Unit) {
                try {
                    val resp = fetchWorkspace()
                    holder.applyWorkspace(resp)
                } catch (e: Throwable) {
                    loadError = e.message ?: "unknown error"
                }
            }

            val snap = holder.snapshot
            when {
                loadError != null -> ErrorView(loadError!!)
                snap == null -> LoadingView()
                else -> LoadedView(workspace = snap, holder = holder)
            }
        }
    }
}

@Stable
sealed interface WorkspaceState {
    data object Loading : WorkspaceState
    data class Error(val message: String) : WorkspaceState
    data class Loaded(val workspace: WorkspaceResponse) : WorkspaceState
}

@Composable
private fun LoadingView() {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.height(12.dp))
            Text("loading workspace…", color = MaterialTheme.colorScheme.onBackground)
        }
    }
}

@Composable
private fun ErrorView(message: String) {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(
                "failed to load /api/v1/workspace",
                color = MaterialTheme.colorScheme.error,
                fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.height(8.dp))
            Text(
                message,
                color = MaterialTheme.colorScheme.onBackground,
                fontFamily = FontFamily.Monospace,
            )
        }
    }
}

@Composable
private fun LoadedView(workspace: WorkspaceResponse, holder: ReviewStateHolder) {
    var selectedPath by remember(workspace.files) {
        mutableStateOf(workspace.files.firstOrNull()?.path)
    }
    val colors = LocalSitatameColors.current
    val scope = rememberCoroutineScope()

    // Active CommentModal target (null = modal closed).
    var activeModalTarget by remember { mutableStateOf<CommentTarget?>(null) }
    // Optional title override for reply mode; null means use target.label() default.
    var activeModalTitle by remember { mutableStateOf<String?>(null) }

    val effectiveComments = holder.effectiveComments()
    val reviewLevelComments = effectiveComments.filter { it.kind == "review" }

    // Gather blob info for the selected file to pass to CommentTarget
    val selectedFile = workspace.files.firstOrNull { it.path == selectedPath }

    // --- mutation handlers ---

    fun submitComment(target: CommentTarget, body: String) {
        val req = buildCreateCommentRequest(target, body, workspace)
        scope.launch {
            when (val result = postComment(req, holder.etag)) {
                is MutationResult.Success -> {
                    val newEtag = result.response.etag
                    val anchorId = (result.response.value["anchor_id"] ?: "")
                    // Build a minimal CommentDto from the request so the UI
                    // updates without a full GET /workspace.
                    val newComment = CommentDto(
                        anchorId = anchorId,
                        kind = req.kind,
                        path = req.path ?: "",
                        side = req.side,
                        line = req.line,
                        lineStart = req.lineStart,
                        lineEnd = req.lineEnd,
                        state = "open",
                        body = req.body,
                    )
                    holder.applyNewComment(newComment, newEtag)
                }
                is MutationResult.EtagMismatch -> {
                    holder.conflict = ConflictState.EtagConflict(
                        error = result.error,
                        serverEtag = result.currentEtag,
                        pending = PendingMutation.AddComment(req),
                    )
                }
                is MutationResult.ValidationError -> {
                    holder.toastMessage = "Validation error: ${result.errors.take(120)}"
                }
                is MutationResult.Unexpected -> {
                    holder.toastMessage = "Unexpected error (HTTP ${result.status})"
                }
            }
        }
    }

    fun toggleCommentState(comment: CommentDto) {
        val newState = if (comment.state == "open") "resolved" else "open"
        holder.setOptimistic(comment.anchorId, newState, comment.state)
        scope.launch {
            when (val result = patchCommentState(
                comment.anchorId,
                UpdateCommentStateRequest(newState),
                holder.etag,
            )) {
                is MutationResult.Success -> {
                    holder.confirmOptimistic(comment.anchorId, result.response.etag)
                }
                is MutationResult.EtagMismatch -> {
                    holder.rollbackOptimistic(comment.anchorId, "Conflict: review was updated by another client")
                    holder.conflict = ConflictState.EtagConflict(
                        error = result.error,
                        serverEtag = result.currentEtag,
                        pending = PendingMutation.ToggleState(comment.anchorId, newState),
                    )
                }
                is MutationResult.ValidationError -> {
                    holder.rollbackOptimistic(comment.anchorId, "Validation error: ${result.errors.take(80)}")
                }
                is MutationResult.Unexpected -> {
                    holder.rollbackOptimistic(comment.anchorId, "Server error (HTTP ${result.status})")
                }
            }
        }
    }

    fun submitReviewComment(text: String) {
        scope.launch {
            when (val result = putReviewComment(UpdateReviewCommentRequest(text), holder.etag)) {
                is MutationResult.Success -> {
                    holder.applyReviewCommentUpdate(text, result.response.etag)
                }
                is MutationResult.EtagMismatch -> {
                    holder.conflict = ConflictState.EtagConflict(
                        error = result.error,
                        serverEtag = result.currentEtag,
                        pending = PendingMutation.UpdateReviewComment(text),
                    )
                }
                is MutationResult.ValidationError -> {
                    holder.toastMessage = "Validation error: ${result.errors.take(120)}"
                }
                is MutationResult.Unexpected -> {
                    holder.toastMessage = "Server error (HTTP ${result.status})"
                }
            }
        }
    }

    // Conflict resolution handlers
    fun reloadAndRetry() {
        val pending = (holder.conflict as? ConflictState.EtagConflict)?.pending ?: run {
            holder.clearConflict()
            return
        }
        holder.clearConflict()
        scope.launch {
            try {
                val resp = fetchWorkspace()
                holder.applyWorkspace(resp)
                // Retry the pending mutation with the new ETag.
                when (pending) {
                    is PendingMutation.AddComment -> {
                        when (val result = postComment(pending.request, holder.etag)) {
                            is MutationResult.Success -> {
                                val comment = CommentDto(
                                    anchorId = result.response.value["anchor_id"] ?: "",
                                    kind = pending.request.kind,
                                    path = pending.request.path ?: "",
                                    side = pending.request.side,
                                    line = pending.request.line,
                                    lineStart = pending.request.lineStart,
                                    lineEnd = pending.request.lineEnd,
                                    state = "open",
                                    body = pending.request.body,
                                )
                                holder.applyNewComment(comment, result.response.etag)
                            }
                            else -> holder.toastMessage = "Retry failed after reload"
                        }
                    }
                    is PendingMutation.ToggleState -> {
                        when (val result = patchCommentState(
                            pending.anchorId,
                            UpdateCommentStateRequest(pending.newState),
                            holder.etag,
                        )) {
                            is MutationResult.Success ->
                                holder.confirmOptimistic(pending.anchorId, result.response.etag)
                            else -> holder.toastMessage = "Retry failed after reload"
                        }
                    }
                    is PendingMutation.UpdateReviewComment -> {
                        when (val result = putReviewComment(
                            UpdateReviewCommentRequest(pending.text),
                            holder.etag,
                        )) {
                            is MutationResult.Success ->
                                holder.applyReviewCommentUpdate(pending.text, result.response.etag)
                            else -> holder.toastMessage = "Retry failed after reload"
                        }
                    }
                }
            } catch (e: Throwable) {
                holder.toastMessage = "Reload failed: ${e.message}"
            }
        }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        Column(modifier = Modifier.fillMaxSize()) {
            TopBar(
                workspace = workspace,
                onAddOverallComment = {
                    activeModalTarget = CommentTarget.Review
                    activeModalTitle = null
                },
            )
            ReviewSummaryPanel(
                reviewComment = holder.snapshot?.review?.reviewComment,
                reviewLevelComments = reviewLevelComments,
                onEditReviewComment = { text -> submitReviewComment(text) },
            )
            Row(modifier = Modifier.fillMaxSize().weight(1f)) {
                Sidebar(
                    files = workspace.files,
                    comments = effectiveComments,
                    selectedPath = selectedPath,
                    filterState = holder.filterState,
                    onSelect = { selectedPath = it },
                    onFilterSelect = { holder.filterState = it },
                    onToggleState = { comment -> toggleCommentState(comment) },
                    onReply = { sourceComment ->
                        val target = sourceComment.toCommentTarget()
                        activeModalTarget = target
                        activeModalTitle = "Reply to: ${target.label()}"
                    },
                    pendingToggleIds = holder.pendingToggleIds,
                    modifier = Modifier
                        .width(320.dp)
                        .fillMaxSize()
                        .background(MaterialTheme.colorScheme.surface),
                )
                Box(
                    modifier = Modifier
                        .width(1.dp)
                        .fillMaxSize()
                        .background(colors.border),
                )
                MainPane(
                    file = selectedFile,
                    comments = effectiveComments,
                    onAddComment = { target ->
                        activeModalTarget = target
                        activeModalTitle = null
                    },
                    modifier = Modifier.weight(1f).fillMaxSize(),
                )
            }
            StatusBar(holder.snapshot ?: workspace)
        }

        // CommentModal overlay
        activeModalTarget?.let { target ->
            CommentModal(
                target = target,
                onSubmit = { body ->
                    submitComment(target, body)
                    activeModalTarget = null
                    activeModalTitle = null
                },
                onCancel = {
                    activeModalTarget = null
                    activeModalTitle = null
                },
                title = activeModalTitle ?: target.label(),
            )
        }

        // ConflictModal overlay
        (holder.conflict as? ConflictState.EtagConflict)?.let { conflict ->
            ConflictModal(
                conflict = conflict,
                onReloadAndRetry = { reloadAndRetry() },
                onDiscard = { holder.clearConflict() },
            )
        }

        // Toast
        holder.toastMessage?.let { msg ->
            Toast(
                message = msg,
                onDismiss = { holder.dismissToast() },
            )
        }
    }
}

// ---------------------------------------------------------------------------
// Helper: build CreateCommentRequest from CommentTarget
// ---------------------------------------------------------------------------

private fun buildCreateCommentRequest(
    target: CommentTarget,
    body: String,
    workspace: WorkspaceResponse,
): CreateCommentRequest = when (target) {
    is CommentTarget.Line -> {
        val file = workspace.files.firstOrNull { it.path == target.path }
        // Use the blob SHA from the diff index header. Deletion lines have
        // side="base" and use blobBase; all other lines use blobHead.
        val blob = when (target.side) {
            "base" -> file?.blobBase
            else -> file?.blobHead
        }
        CreateCommentRequest(
            kind = "line",
            path = target.path,
            side = target.side,
            blob = blob,
            line = target.line,
            body = body,
        )
    }
    is CommentTarget.Range -> {
        val file = workspace.files.firstOrNull { it.path == target.path }
        val blob = when (target.side) {
            "base" -> file?.blobBase
            else -> file?.blobHead
        }
        CreateCommentRequest(
            kind = "range",
            path = target.path,
            side = target.side,
            blob = blob,
            lineStart = target.lineStart,
            lineEnd = target.lineEnd,
            body = body,
        )
    }
    is CommentTarget.File -> {
        CreateCommentRequest(
            kind = "file",
            path = target.path,
            body = body,
        )
    }
    CommentTarget.Review -> {
        CreateCommentRequest(
            kind = "review",
            body = body,
        )
    }
}

// ---------------------------------------------------------------------------
// Top bar
// ---------------------------------------------------------------------------

@Composable
private fun TopBar(workspace: WorkspaceResponse, onAddOverallComment: () -> Unit) {
    val colors = LocalSitatameColors.current
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surface)
            .padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text("sitatame", fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onSurface)
        Spacer(Modifier.width(16.dp))
        Text(
            text = buildString {
                append("branch: ").append(workspace.branch)
                workspace.review?.let { rev ->
                    append("  |  ").append(rev.baseRef).append(" → ").append(rev.headRef)
                }
            },
            color = colors.mutedText,
            fontFamily = FontFamily.Monospace,
        )
        Spacer(Modifier.weight(1f))
        Button(
            onClick = onAddOverallComment,
            colors = ButtonDefaults.buttonColors(
                containerColor = MaterialTheme.colorScheme.primary,
                contentColor = MaterialTheme.colorScheme.onPrimary,
            ),
        ) {
            Text("Add overall comment", fontSize = 12.sp)
        }
    }
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(1.dp)
            .background(colors.border),
    )
}

// ---------------------------------------------------------------------------
// Status bar
// ---------------------------------------------------------------------------

@Composable
private fun StatusBar(workspace: WorkspaceResponse) {
    val colors = LocalSitatameColors.current
    val comments = workspace.review?.comments.orEmpty()
    val open = comments.count { it.state == "open" }
    val resolved = comments.count { it.state == "resolved" }
    val stale = comments.count { it.state == "stale" }
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(1.dp)
            .background(colors.border),
    )
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surface)
            .padding(horizontal = 16.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(
            text = "${workspace.files.size} files · ${comments.size} comments " +
                "($open open, $resolved resolved, $stale stale)",
            color = colors.mutedText,
            fontFamily = FontFamily.Monospace,
        )
        Text(
            text = workspace.sourcePath?.let { "review: $it" } ?: "no review yet",
            color = colors.mutedText,
            fontFamily = FontFamily.Monospace,
        )
    }
}

// ---------------------------------------------------------------------------
// Toast
// ---------------------------------------------------------------------------

@Composable
private fun Toast(message: String, onDismiss: () -> Unit) {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.BottomCenter) {
        Surface(
            modifier = Modifier.padding(16.dp),
            color = MaterialTheme.colorScheme.error,
            shape = RoundedCornerShape(8.dp),
        ) {
            Row(
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = message,
                    color = MaterialTheme.colorScheme.onError,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 13.sp,
                )
                Spacer(Modifier.width(16.dp))
                Button(
                    onClick = onDismiss,
                    colors = ButtonDefaults.buttonColors(
                        containerColor = MaterialTheme.colorScheme.onError,
                        contentColor = MaterialTheme.colorScheme.error,
                    ),
                ) {
                    Text("Dismiss", fontSize = 11.sp)
                }
            }
        }
    }
    // Auto-dismiss via LaunchedEffect is not wired here (would need kotlinx-coroutines
    // delay); the user can dismiss manually. Phase C can add auto-dismiss.
}
