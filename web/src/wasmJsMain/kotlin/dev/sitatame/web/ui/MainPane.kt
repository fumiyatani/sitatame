package dev.sitatame.web.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
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
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.sitatame.web.api.CommentDto
import dev.sitatame.web.api.DiffLineDto
import dev.sitatame.web.api.FileDto
import dev.sitatame.web.api.HunkDto

// ---------------------------------------------------------------------------
// Range selection state
// ---------------------------------------------------------------------------

/** Identifies a selected line within a specific hunk. */
data class HunkLine(
    val hunkIndex: Int,
    val lineIndex: Int,
    val line: Int,        // head or base line number
    val side: String,     // "head" | "base"
    val hunkStart: Int,   // hunk.headStart / baseStart — used for hunk identity check
)

@Stable
class RangeSelectionState {
    var start by mutableStateOf<HunkLine?>(null)
    var bannerMessage by mutableStateOf<String?>(null)

    val isActive: Boolean get() = start != null || bannerMessage != null

    fun reset() {
        start = null
        bannerMessage = null
    }

    fun beginAt(hl: HunkLine) {
        start = hl
        bannerMessage = null
    }
}

// ---------------------------------------------------------------------------
// Main composable
// ---------------------------------------------------------------------------

/**
 * Right-side main pane: file header (sticky), range mode banner, unified-diff
 * hunks, and any anchored comments displayed inline.
 *
 * [onAddComment] is called with a [CommentTarget] whenever the user initiates
 * a comment:
 *   - single tap on a diff line → [CommentTarget.Line]
 *   - range selection complete → [CommentTarget.Range]
 *   - "Add file comment" button → [CommentTarget.File]
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun MainPane(
    file: FileDto?,
    comments: List<CommentDto>,
    onAddComment: (CommentTarget) -> Unit,
    modifier: Modifier = Modifier,
) {
    if (file == null) {
        EmptyState(modifier = modifier)
        return
    }
    val colors = LocalSitatameColors.current
    val fileComments = comments.filter { it.path == file.path }
    val rangeState = remember { RangeSelectionState() }

    Box(modifier = modifier) {
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .background(MaterialTheme.colorScheme.background),
        ) {
            stickyHeader {
                Column {
                    FileHeader(
                        file = file,
                        onAddFileComment = { onAddComment(CommentTarget.File(file.path)) },
                    )
                    // Range mode banner sits just below the file header.
                    RangeModeBanner(rangeState)
                }
            }

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
                        val side = sideForLine(line)
                        val lineNo = if (side == "base") line.baseLine else line.headLine

                        // Range highlight: if a range start is selected and this
                        // line is within the same hunk, dim lines outside the
                        // partial selection range.
                        val isRangeHighlighted = rangeState.start?.let { s ->
                            s.hunkIndex == hunkIndex && lineNo != null &&
                                lineNo >= minOf(s.line, lineNo) &&
                                lineNo <= maxOf(s.line, lineNo)
                        } ?: false

                        DiffLineRow(
                            row = line,
                            highlighted = isRangeHighlighted,
                            onTap = {
                                if (lineNo == null) return@DiffLineRow
                                val start = rangeState.start
                                if (start == null) {
                                    // Normal single-line tap
                                    onAddComment(CommentTarget.Line(file.path, lineNo, side))
                                } else {
                                    // Range selection: second click
                                    if (start.hunkIndex != hunkIndex) {
                                        rangeState.bannerMessage =
                                            "Select end line in the same hunk (started in hunk ${start.hunkIndex + 1})"
                                        return@DiffLineRow
                                    }
                                    val lo = minOf(start.line, lineNo)
                                    val hi = maxOf(start.line, lineNo)
                                    rangeState.reset()
                                    onAddComment(
                                        CommentTarget.Range(file.path, lo, hi, side),
                                    )
                                }
                            },
                            onShiftTap = {
                                // Shift+tap → start range selection
                                if (lineNo == null) return@DiffLineRow
                                val hunkStart =
                                    if (side == "base") hunk.baseStart else hunk.headStart
                                rangeState.beginAt(
                                    HunkLine(hunkIndex, lineIndex, lineNo, side, hunkStart),
                                )
                            },
                        )

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
}

// ---------------------------------------------------------------------------
// Side determination
// ---------------------------------------------------------------------------

/**
 * Determines the `side` value for a diff line following the Go TUI convention
 * (`internal/tui/modal.go:lineSideForRow`):
 *  - deletion rows (`-`) → `"base"` (the line exists only on the base side)
 *  - all other rows (additions `+` and context ` `) → `"head"`
 */
private fun sideForLine(line: DiffLineDto): String =
    if (line.prefix == "-") "base" else "head"

// ---------------------------------------------------------------------------
// Range mode banner
// ---------------------------------------------------------------------------

@Composable
private fun RangeModeBanner(state: RangeSelectionState) {
    if (!state.isActive) return
    val colors = LocalSitatameColors.current
    val message = when {
        state.bannerMessage != null -> state.bannerMessage!!
        state.start != null -> "Select end line (start = ${state.start!!.line}) — Shift+click to start over"
        else -> "Select start line (Shift+click)"
    }
    Surface(
        color = MaterialTheme.colorScheme.primary.copy(alpha = 0.15f),
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(max = 40.dp),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = message,
                color = MaterialTheme.colorScheme.primary,
                fontFamily = FontFamily.Monospace,
                fontSize = 12.sp,
                modifier = Modifier.weight(1f),
            )
            TextButton(
                onClick = { state.reset() },
            ) {
                Text(
                    "Cancel",
                    color = colors.mutedText,
                    fontSize = 11.sp,
                )
            }
        }
    }
}

// ---------------------------------------------------------------------------
// matchesLine
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// FileHeader
// ---------------------------------------------------------------------------

@Composable
private fun FileHeader(file: FileDto, onAddFileComment: () -> Unit) {
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
                Spacer(Modifier.width(12.dp))
                TextButton(onClick = onAddFileComment) {
                    Text(
                        "Add file comment",
                        fontSize = 11.sp,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
            }
            Box(modifier = Modifier.fillMaxWidth().background(colors.border))
        }
    }
}

// ---------------------------------------------------------------------------
// HunkHeaderRow
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// DiffLineRow
// ---------------------------------------------------------------------------

/**
 * A single diff line row.
 *
 * [onTap] is called on a normal click; [onShiftTap] on Shift+click (for
 * range mode initiation).
 *
 * Text selection and tap detection coexist by keeping the `SelectionContainer`
 * on the text only and wrapping the whole row with `pointerInput`.  The tap
 * gesture fires on `onTap`; text dragging (selection) is handled by the inner
 * `SelectionContainer` independently because `detectTapGestures` only
 * intercepts tap, not drag.
 *
 * Note: distinguishing Shift vs plain click via `pointerInput` in Compose for
 * Web (Wasm) is limited — `PointerKeyboardModifiers` carries `.isShiftPressed`
 * on `PointerEvent`, but `detectTapGestures` doesn't expose it yet (as of CMP
 * 1.7.x).  We use `onLongPress` as the "shift tap equivalent" for now; the JS
 * layer can wire `Shift+click` in Phase C when a direct DOM event bridge is
 * available.  The range button in the top bar remains the canonical UX path.
 */
@Composable
fun DiffLineRow(
    row: DiffLineDto,
    highlighted: Boolean = false,
    onTap: (() -> Unit)? = null,
    onShiftTap: (() -> Unit)? = null,
) {
    val colors = LocalSitatameColors.current
    val baseBg = when (row.prefix) {
        "+" -> colors.addLineBg
        "-" -> colors.delLineBg
        else -> colors.ctxLineBg
    }
    val bg = if (highlighted) baseBg.copy(alpha = 0.5f) else baseBg

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(bg)
            .then(
                if (onTap != null) {
                    Modifier.pointerInput(onTap, onShiftTap) {
                        detectTapGestures(
                            onTap = { onTap() },
                            onLongPress = { onShiftTap?.invoke() },
                        )
                    }
                } else {
                    Modifier
                },
            )
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

// ---------------------------------------------------------------------------
// GutterCell
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// CommentCard / StateBadge (shared, also used from SitatameApp)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// EmptyState
// ---------------------------------------------------------------------------

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
