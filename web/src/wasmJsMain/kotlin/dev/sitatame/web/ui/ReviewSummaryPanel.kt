package dev.sitatame.web.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.sitatame.web.api.CommentDto

/**
 * Renders the review-level feedback that is otherwise dropped by the
 * file-anchored MainPane: the overall `review.reviewComment` (Shift+R in the
 * TUI) and any anchor-less `kind == "review"` comments.
 *
 * Sits above the sidebar/main split so the user sees the top-level summary
 * before drilling into per-file diffs. Renders nothing when both inputs are
 * empty so the layout doesn't grow a phantom strip on review-less branches.
 */
@Composable
fun ReviewSummaryPanel(
    reviewComment: String?,
    reviewLevelComments: List<CommentDto>,
    modifier: Modifier = Modifier,
) {
    val hasComment = !reviewComment.isNullOrBlank()
    val hasReviewLevel = reviewLevelComments.isNotEmpty()
    if (!hasComment && !hasReviewLevel) return

    val colors = LocalSitatameColors.current
    Surface(
        color = MaterialTheme.colorScheme.surface,
        modifier = modifier.fillMaxWidth(),
    ) {
        Column(modifier = Modifier.padding(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = "Review-level feedback",
                    color = MaterialTheme.colorScheme.onSurface,
                    fontWeight = FontWeight.SemiBold,
                )
                Spacer(Modifier.width(8.dp))
                val parts = buildList {
                    if (hasComment) add("summary")
                    if (hasReviewLevel) add("${reviewLevelComments.size} review comments")
                }
                Text(
                    text = "(" + parts.joinToString(" · ") + ")",
                    color = colors.mutedText,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 12.sp,
                )
            }
            if (hasComment) {
                Spacer(Modifier.height(8.dp))
                ReviewCommentCard(reviewComment!!)
            }
            if (hasReviewLevel) {
                Spacer(Modifier.height(8.dp))
                reviewLevelComments.forEach { CommentCard(it) }
            }
        }
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(1.dp)
                .background(colors.border),
        )
    }
}

/**
 * Markdown rendering is out of scope for Phase 1 step 1; until step 2 wires a
 * Markdown component we display the raw body in a monospace card so users at
 * least see the unrendered text rather than losing it entirely.
 */
@Composable
private fun ReviewCommentCard(body: String) {
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.background,
            contentColor = MaterialTheme.colorScheme.onBackground,
        ),
        shape = RoundedCornerShape(6.dp),
    ) {
        SelectionContainer(modifier = Modifier.padding(12.dp)) {
            Text(
                text = body,
                color = MaterialTheme.colorScheme.onBackground,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
            )
        }
    }
}
