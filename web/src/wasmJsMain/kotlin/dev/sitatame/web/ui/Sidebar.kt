package dev.sitatame.web.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.sitatame.web.api.CommentDto
import dev.sitatame.web.api.FileDto

/**
 * Left sidebar (~320dp). Mirrors the TUI file picker: a flat list of all
 * changed files with status indicator, +adds/-dels, and an "open comment"
 * badge.
 *
 * [onToggleState] is called when the user clicks the Resolve/Reopen button on
 * an inline comment.  [pendingToggleIds] is the set of anchorIds whose PATCH
 * call is still in-flight — those show a spinner instead of a button.
 */
@Composable
fun Sidebar(
    files: List<FileDto>,
    comments: List<CommentDto>,
    selectedPath: String?,
    onSelect: (String) -> Unit,
    onToggleState: (CommentDto) -> Unit = {},
    pendingToggleIds: Set<String> = emptySet(),
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier) {
        SidebarHeader(fileCount = files.size)
        LazyColumn(modifier = Modifier.fillMaxWidth()) {
            items(files, key = { it.path }) { file ->
                val fileComments = comments.filter { it.path == file.path }
                val openCount = fileComments.count { it.state == "open" }
                SidebarRow(
                    file = file,
                    openCount = openCount,
                    selected = file.path == selectedPath,
                    onClick = { onSelect(file.path) },
                )
                // Inline comment list under each selected file row
                if (file.path == selectedPath && fileComments.isNotEmpty()) {
                    fileComments.forEach { comment ->
                        SidebarCommentRow(
                            comment = comment,
                            pending = comment.anchorId in pendingToggleIds,
                            onToggle = { onToggleState(comment) },
                        )
                    }
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// SidebarHeader
// ---------------------------------------------------------------------------

@Composable
private fun SidebarHeader(fileCount: Int) {
    val colors = LocalSitatameColors.current
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = "Files",
            color = MaterialTheme.colorScheme.onSurface,
            fontWeight = FontWeight.SemiBold,
        )
        Spacer(Modifier.width(8.dp))
        Text(
            text = "($fileCount)",
            color = colors.mutedText,
            fontFamily = FontFamily.Monospace,
        )
    }
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .background(colors.border)
            .padding(horizontal = 0.dp, vertical = 0.dp),
    )
}

// ---------------------------------------------------------------------------
// SidebarRow
// ---------------------------------------------------------------------------

@Composable
private fun SidebarRow(
    file: FileDto,
    openCount: Int,
    selected: Boolean,
    onClick: () -> Unit,
) {
    val colors = LocalSitatameColors.current
    val bg = if (selected) colors.sidebarHighlight else Color.Transparent
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(bg)
            .clickable { onClick() }
            .padding(horizontal = 12.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        StatusGlyph(file.status)
        Spacer(Modifier.width(8.dp))
        Text(
            text = file.path,
            modifier = Modifier.weight(1f),
            color = MaterialTheme.colorScheme.onSurface,
            fontFamily = FontFamily.Monospace,
            fontSize = 13.sp,
            maxLines = 1,
        )
        Spacer(Modifier.width(8.dp))
        Text(
            text = "+${file.adds}",
            color = colors.addLineGutter,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
        )
        Spacer(Modifier.width(4.dp))
        Text(
            text = "-${file.dels}",
            color = colors.delLineGutter,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
        )
        if (openCount > 0) {
            Spacer(Modifier.width(8.dp))
            OpenBadge(openCount)
        }
    }
}

// ---------------------------------------------------------------------------
// SidebarCommentRow — resolve toggle (B6)
// ---------------------------------------------------------------------------

/**
 * A compact comment row shown under the selected file's entry in the sidebar.
 *
 * The Resolve/Reopen button triggers an optimistic state update via
 * [onToggle].  While [pending] is true, a spinner replaces the button.
 */
@Composable
private fun SidebarCommentRow(
    comment: CommentDto,
    pending: Boolean,
    onToggle: () -> Unit,
) {
    val colors = LocalSitatameColors.current
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.background)
            .padding(start = 24.dp, end = 8.dp, top = 2.dp, bottom = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        StateBadge(comment.state)
        Spacer(Modifier.width(6.dp))
        Text(
            text = buildString {
                when (comment.kind) {
                    "line" -> append("L${comment.line}")
                    "range" -> append("L${comment.lineStart}-${comment.lineEnd}")
                    "file" -> append("file")
                    "review" -> append("review")
                    else -> append(comment.kind)
                }
                append(": ")
                append(comment.body.take(30))
                if (comment.body.length > 30) append("…")
            },
            modifier = Modifier.weight(1f),
            color = colors.mutedText,
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
                    fontSize = 11.sp,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
        }
    }
}

// ---------------------------------------------------------------------------
// StatusGlyph / OpenBadge
// ---------------------------------------------------------------------------

@Composable
private fun StatusGlyph(status: String) {
    val colors = LocalSitatameColors.current
    val color = when (status) {
        "A" -> colors.addLineGutter
        "D" -> colors.delLineGutter
        "R" -> MaterialTheme.colorScheme.primary
        else -> colors.mutedText
    }
    Surface(
        color = color.copy(alpha = 0.15f),
        shape = RoundedCornerShape(4.dp),
    ) {
        Text(
            text = status,
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 1.dp),
            color = color,
            fontFamily = FontFamily.Monospace,
            fontWeight = FontWeight.Bold,
            fontSize = 12.sp,
        )
    }
}

@Composable
private fun OpenBadge(count: Int) {
    val colors = LocalSitatameColors.current
    Surface(
        color = colors.openBadge.copy(alpha = 0.2f),
        shape = RoundedCornerShape(10.dp),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 1.dp),
            horizontalArrangement = Arrangement.Center,
        ) {
            Text(
                text = count.toString(),
                color = colors.openBadge,
                fontFamily = FontFamily.Monospace,
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold,
            )
        }
    }
}
