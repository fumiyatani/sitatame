package dev.sitatame.web.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.sitatame.web.api.CommentDto

/**
 * Renders a single comment thread — a grouped set of comments sharing the same
 * [Thread.key] — with collapsible display.
 *
 * - Resolved threads start collapsed ([defaultCollapsed] returns true).
 * - Open / Stale threads start expanded.
 * - Tapping the header row toggles collapsed state.
 * - When expanded: each comment is rendered via [CommentRow], followed by a
 *   "Reply to this thread" button that invokes [onReply].
 *
 * Background colour and badge colour reflect [Thread.state]:
 *   Open     → default background, blue badge
 *   Resolved → dim grey background, grey badge
 *   Stale    → pale yellow background, yellow badge
 *
 * [pendingToggleIds] is the set of anchorIds whose PATCH call is in-flight;
 * those comments show a spinner instead of the Resolve/Reopen button.
 */
@Composable
fun CommentThread(
    thread: Thread,
    onReply: (CommentDto) -> Unit,
    onToggleState: (CommentDto) -> Unit,
    pendingToggleIds: Set<String> = emptySet(),
    modifier: Modifier = Modifier,
) {
    var collapsed by remember(thread.key) { mutableStateOf(defaultCollapsed(thread)) }
    val colors = LocalSitatameColors.current

    // Background and badge colours derived from thread state.
    val (threadBg, badgeColor) = when (thread.state) {
        ThreadState.Open -> MaterialTheme.colorScheme.background to colors.openBadge
        ThreadState.Resolved -> Color(0xFF1C2128) to colors.resolvedBadge   // dark muted grey
        ThreadState.Stale -> Color(0xFF2D2208) to colors.staleBadge          // dark amber tint
    }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(threadBg),
    ) {
        // Header row — always visible, clickable to toggle.
        ThreadHeader(
            thread = thread,
            collapsed = collapsed,
            badgeColor = badgeColor,
            onClick = { collapsed = !collapsed },
        )

        // Body — shown only when expanded.
        if (!collapsed) {
            thread.comments.forEach { comment ->
                CommentRow(
                    comment = comment,
                    pending = comment.anchorId in pendingToggleIds,
                    onToggle = { onToggleState(comment) },
                )
            }
            // Reply button — opens CommentModal in reply mode via parent callback.
            TextButton(
                onClick = { onReply(thread.comments.first()) },
                modifier = Modifier
                    .padding(start = 36.dp, top = 2.dp, bottom = 2.dp),
            ) {
                Text(
                    text = "Reply to this thread",
                    fontSize = 11.sp,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
            Spacer(Modifier.height(4.dp))
        }
    }
}

// ---------------------------------------------------------------------------
// ThreadHeader
// ---------------------------------------------------------------------------

@Composable
private fun ThreadHeader(
    thread: Thread,
    collapsed: Boolean,
    badgeColor: Color,
    onClick: () -> Unit,
) {
    val colors = LocalSitatameColors.current
    val chevron = if (collapsed) "▶" else "▼"
    val commentCount = thread.comments.size

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { onClick() }
            .padding(start = 24.dp, end = 8.dp, top = 4.dp, bottom = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = chevron,
            color = colors.mutedText,
            fontFamily = FontFamily.Monospace,
            fontSize = 10.sp,
        )
        Spacer(Modifier.width(6.dp))
        Text(
            text = thread.key.label(),
            modifier = Modifier.weight(1f),
            color = MaterialTheme.colorScheme.onSurface,
            fontFamily = FontFamily.Monospace,
            fontSize = 11.sp,
            maxLines = 1,
        )
        Spacer(Modifier.width(6.dp))
        // Thread state badge
        Surface(
            color = badgeColor.copy(alpha = 0.20f),
            shape = RoundedCornerShape(10.dp),
        ) {
            Text(
                text = thread.state.shortLabel(),
                modifier = Modifier.padding(horizontal = 6.dp, vertical = 1.dp),
                color = badgeColor,
                fontFamily = FontFamily.Monospace,
                fontSize = 10.sp,
                fontWeight = FontWeight.SemiBold,
            )
        }
        if (commentCount > 1) {
            Spacer(Modifier.width(4.dp))
            Text(
                text = "($commentCount)",
                color = colors.mutedText,
                fontFamily = FontFamily.Monospace,
                fontSize = 10.sp,
            )
        }
    }
}

// ---------------------------------------------------------------------------
// CommentRow — one comment inside a thread.
// ---------------------------------------------------------------------------

/**
 * A single comment line inside an expanded [CommentThread].
 *
 * Mirrors [SidebarCommentRow] in the old Sidebar but is now decoupled from
 * the file-list layout so it can live inside a thread column.
 */
@Composable
fun CommentRow(
    comment: CommentDto,
    pending: Boolean,
    onToggle: () -> Unit,
) {
    val colors = LocalSitatameColors.current
    val textColor = if (comment.state == "resolved") Color.Gray
    else MaterialTheme.colorScheme.onSurface

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = 36.dp, end = 8.dp, top = 2.dp, bottom = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        StateBadge(comment.state)
        Spacer(Modifier.width(6.dp))
        Text(
            text = buildString {
                append(comment.body.take(40))
                if (comment.body.length > 40) append("…")
            },
            modifier = Modifier.weight(1f),
            color = textColor,
            fontFamily = FontFamily.Monospace,
            fontSize = 11.sp,
            maxLines = 1,
        )
        if (pending) {
            CircularProgressIndicator(
                modifier = Modifier.size(16.dp),
                strokeWidth = 2.dp,
                color = MaterialTheme.colorScheme.primary,
            )
        } else {
            TextButton(
                onClick = onToggle,
                modifier = Modifier.padding(horizontal = 0.dp),
            ) {
                Text(
                    text = if (comment.state == "open") "Resolve" else "Reopen",
                    fontSize = 10.sp,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Extension helpers for UI display
// ---------------------------------------------------------------------------

/** Short label to display in the thread state badge. */
private fun ThreadState.shortLabel(): String = when (this) {
    ThreadState.Open -> "Open"
    ThreadState.Resolved -> "Done"
    ThreadState.Stale -> "Stale"
}

/**
 * Human-readable location label for a [ThreadKey], used in the thread header.
 *
 * Format:
 *   Line  → `<path>:<line>` (base side appends " (base)")
 *   Range → `<path>:<lineStart>-<lineEnd>` (base side appends " (base)")
 *   File  → `<path> (file)`
 */
private fun ThreadKey.label(): String = when (this) {
    is ThreadKey.Line -> buildString {
        append(path.substringAfterLast('/'))
        append(':')
        append(line)
        if (side == "base") append(" (base)")
    }
    is ThreadKey.Range -> buildString {
        append(path.substringAfterLast('/'))
        append(':')
        append(lineStart)
        append('-')
        append(lineEnd)
        if (side == "base") append(" (base)")
    }
    is ThreadKey.File -> "${path.substringAfterLast('/')} (file)"
}
