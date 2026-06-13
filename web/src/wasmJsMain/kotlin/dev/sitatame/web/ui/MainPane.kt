package dev.sitatame.web.ui

import androidx.compose.foundation.ExperimentalFoundationApi
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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
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
import dev.sitatame.web.api.DiffLineDto
import dev.sitatame.web.api.FileDto
import dev.sitatame.web.api.HunkDto

/**
 * Right-side main pane: file header (sticky), unified-diff hunks, and any
 * anchored comments displayed inline directly under the line they anchor to.
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun MainPane(
    file: FileDto?,
    comments: List<CommentDto>,
    modifier: Modifier = Modifier,
) {
    if (file == null) {
        EmptyState(modifier = modifier)
        return
    }
    val colors = LocalSitatameColors.current
    val fileComments = comments.filter { it.path == file.path }

    LazyColumn(
        modifier = modifier.background(MaterialTheme.colorScheme.background),
    ) {
        stickyHeader { FileHeader(file) }

        // File-anchored comments (no line) appear right under the file header.
        val fileLevelComments = fileComments.filter { it.kind == "file" }
        if (fileLevelComments.isNotEmpty()) {
            item(key = "file-comments-${file.path}") {
                Column(modifier = Modifier.padding(8.dp)) {
                    fileLevelComments.forEach { CommentCard(it) }
                }
            }
        }

        file.hunks.forEachIndexed { hunkIndex, hunk ->
            item(key = "hunk-${file.path}-$hunkIndex") {
                HunkHeaderRow(hunk)
            }
            hunk.lines.forEachIndexed { lineIndex, line ->
                item(key = "line-${file.path}-$hunkIndex-$lineIndex") {
                    DiffLineRow(line)
                    val anchored = fileComments.filter { matchesLine(it, line) }
                    if (anchored.isNotEmpty()) {
                        Column(modifier = Modifier.padding(start = 80.dp, end = 8.dp)) {
                            anchored.forEach { CommentCard(it) }
                        }
                    }
                }
            }
        }
        if (file.hunks.isEmpty()) {
            item(key = "empty-${file.path}") {
                Box(
                    modifier = Modifier.fillMaxWidth().padding(24.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        text = when (file.status) {
                            "A" -> "(added — no diff body parsed yet)"
                            "D" -> "(deleted — no diff body parsed yet)"
                            else -> "(no diff)"
                        },
                        color = colors.mutedText,
                    )
                }
            }
        }
    }
}

private fun matchesLine(c: CommentDto, line: DiffLineDto): Boolean {
    return when (c.kind) {
        "line" -> {
            val want = c.line ?: return false
            if (c.side == "base") line.baseLine == want
            else line.headLine == want
        }
        "range" -> {
            val start = c.lineStart ?: return false
            val end = c.lineEnd ?: return false
            val target = if (c.side == "base") line.baseLine else line.headLine
            target != null && target in start..end
        }
        else -> false
    }
}

@Composable
private fun FileHeader(file: FileDto) {
    val colors = LocalSitatameColors.current
    Surface(
        color = colors.hunkHeaderBg,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = file.status,
                    color = MaterialTheme.colorScheme.primary,
                    fontFamily = FontFamily.Monospace,
                    fontWeight = FontWeight.Bold,
                )
                Spacer(Modifier.width(8.dp))
                Text(
                    text = file.path,
                    color = MaterialTheme.colorScheme.onSurface,
                    fontFamily = FontFamily.Monospace,
                    fontWeight = FontWeight.SemiBold,
                )
                if (file.renameFrom != null && file.renameTo != null) {
                    Spacer(Modifier.width(8.dp))
                    Text(
                        text = "(renamed from ${file.renameFrom})",
                        color = colors.mutedText,
                        fontFamily = FontFamily.Monospace,
                        fontSize = 12.sp,
                    )
                }
                Spacer(Modifier.weight(1f))
                Text(
                    text = "+${file.adds}",
                    color = colors.addLineGutter,
                    fontFamily = FontFamily.Monospace,
                )
                Spacer(Modifier.width(8.dp))
                Text(
                    text = "-${file.dels}",
                    color = colors.delLineGutter,
                    fontFamily = FontFamily.Monospace,
                )
            }
            Box(modifier = Modifier.fillMaxWidth().background(colors.border))
        }
    }
}

@Composable
private fun HunkHeaderRow(hunk: HunkDto) {
    val colors = LocalSitatameColors.current
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(colors.hunkHeaderBg)
            .padding(horizontal = 12.dp, vertical = 4.dp),
    ) {
        SelectionContainer {
            Text(
                text = "@@ -${hunk.baseStart},${hunk.baseLines} +${hunk.headStart},${hunk.headLines} @@ ${hunk.header}",
                color = colors.mutedText,
                fontFamily = FontFamily.Monospace,
                fontSize = 12.sp,
            )
        }
    }
}

@Composable
fun DiffLineRow(row: DiffLineDto) {
    val colors = LocalSitatameColors.current
    val bg = when (row.prefix) {
        "+" -> colors.addLineBg
        "-" -> colors.delLineBg
        else -> colors.ctxLineBg
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(bg)
            .padding(horizontal = 0.dp, vertical = 0.dp),
        verticalAlignment = Alignment.Top,
    ) {
        GutterCell(row.baseLine)
        GutterCell(row.headLine)
        Text(
            text = row.prefix,
            modifier = Modifier.width(16.dp).padding(start = 4.dp),
            color = MaterialTheme.colorScheme.onBackground,
            fontFamily = FontFamily.Monospace,
            fontSize = 13.sp,
        )
        SelectionContainer(modifier = Modifier.weight(1f)) {
            Text(
                text = row.text,
                color = MaterialTheme.colorScheme.onBackground,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
            )
        }
    }
}

@Composable
private fun GutterCell(line: Int?) {
    val colors = LocalSitatameColors.current
    Text(
        text = line?.toString() ?: "",
        modifier = Modifier.width(48.dp).padding(horizontal = 4.dp),
        color = colors.mutedText,
        fontFamily = FontFamily.Monospace,
        fontSize = 12.sp,
    )
}

@Composable
fun CommentCard(c: CommentDto) {
    val colors = LocalSitatameColors.current
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surface,
            contentColor = MaterialTheme.colorScheme.onSurface,
        ),
    ) {
        Column(modifier = Modifier.padding(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StateBadge(c.state)
                Spacer(Modifier.width(8.dp))
                Text(
                    text = "anchor: ${c.kind}" + when {
                        c.line != null -> " · L${c.line}"
                        c.lineStart != null && c.lineEnd != null -> " · L${c.lineStart}-${c.lineEnd}"
                        else -> ""
                    },
                    color = colors.mutedText,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 12.sp,
                )
                Spacer(Modifier.weight(1f))
                // Write-path UI skeleton — Phase 1 step 2 wires the action.
                TextButton(onClick = { /* TODO: phase 1 step 2 — resolve toggle */ }) {
                    Text(if (c.state == "open") "Resolve" else "Reopen")
                }
            }
            Spacer(Modifier.height(4.dp))
            SelectionContainer {
                Text(
                    text = c.body,
                    color = MaterialTheme.colorScheme.onSurface,
                )
            }
        }
    }
}

@Composable
fun StateBadge(state: String) {
    val colors = LocalSitatameColors.current
    val (label, color) = when (state) {
        "open" -> "Open" to colors.openBadge
        "resolved" -> "Resolved" to colors.resolvedBadge
        "stale" -> "Stale" to colors.staleBadge
        else -> state to Color.Gray
    }
    Surface(
        color = color.copy(alpha = 0.2f),
        shape = RoundedCornerShape(12.dp),
    ) {
        Text(
            text = label,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
            color = color,
            fontFamily = FontFamily.Monospace,
            fontSize = 11.sp,
            fontWeight = FontWeight.SemiBold,
        )
    }
}

@Composable
private fun EmptyState(modifier: Modifier) {
    val colors = LocalSitatameColors.current
    Box(
        modifier = modifier.background(MaterialTheme.colorScheme.background),
        contentAlignment = Alignment.Center,
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.Center) {
            Text(
                text = "no file selected — pick one from the left",
                color = colors.mutedText,
                fontFamily = FontFamily.Monospace,
            )
        }
    }
}
