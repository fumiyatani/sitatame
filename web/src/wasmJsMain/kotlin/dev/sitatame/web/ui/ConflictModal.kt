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
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * Shown when the server returns HTTP 412 (ETag mismatch).
 *
 * Presents a 2-choice dialog:
 *  1. Reload latest and retry the pending mutation with the new ETag.
 *  2. Discard the pending mutation and close the modal.
 *
 * Force-overwrite ("Retry as-is") is intentionally absent — that semantic is
 * deferred to a future PR with an explicit force flag and dedicated confirm UI
 * (see architecture-advisor/20260614T145800-web-ui-write-v3-FINAL.md §落とした候補).
 */
@Composable
fun ConflictModal(
    conflict: ConflictState.EtagConflict,
    onReloadAndRetry: () -> Unit,
    onDiscard: () -> Unit,
) {
    // Dim backdrop
    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.65f)),
        contentAlignment = Alignment.Center,
    ) {
        Surface(
            modifier = Modifier
                .padding(horizontal = 64.dp, vertical = 48.dp)
                .shadow(8.dp, RoundedCornerShape(8.dp)),
            color = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(8.dp),
        ) {
            Column(modifier = Modifier.padding(24.dp)) {
                // Warning header
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Surface(
                        color = MaterialTheme.colorScheme.error.copy(alpha = 0.15f),
                        shape = RoundedCornerShape(4.dp),
                    ) {
                        Text(
                            text = "Conflict",
                            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
                            color = MaterialTheme.colorScheme.error,
                            fontFamily = FontFamily.Monospace,
                            fontWeight = FontWeight.SemiBold,
                            fontSize = 12.sp,
                        )
                    }
                    Spacer(Modifier.width(8.dp))
                    Text(
                        text = "review.md was updated by another client",
                        color = MaterialTheme.colorScheme.onSurface,
                        fontWeight = FontWeight.SemiBold,
                        fontSize = 14.sp,
                    )
                }

                Spacer(Modifier.height(12.dp))

                // Pending mutation description
                val pendingLabel = when (val p = conflict.pending) {
                    is PendingMutation.AddComment ->
                        "Add ${p.request.kind} comment: \"${p.request.body.take(60)}${if (p.request.body.length > 60) "…" else ""}\""
                    is PendingMutation.ToggleState ->
                        "Set state → ${p.newState} (anchor: ${p.anchorId.take(8)}…)"
                    is PendingMutation.UpdateReviewComment ->
                        "Update review comment: \"${p.text.take(60)}${if (p.text.length > 60) "…" else ""}\""
                }

                Text(
                    text = "Your pending edit:",
                    color = LocalSitatameColors.current.mutedText,
                    fontSize = 12.sp,
                    fontFamily = FontFamily.Monospace,
                )
                Spacer(Modifier.height(4.dp))
                Surface(
                    color = MaterialTheme.colorScheme.background,
                    shape = RoundedCornerShape(4.dp),
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(
                        text = pendingLabel,
                        modifier = Modifier.padding(horizontal = 10.dp, vertical = 8.dp),
                        color = MaterialTheme.colorScheme.onBackground,
                        fontFamily = FontFamily.Monospace,
                        fontSize = 12.sp,
                    )
                }

                Spacer(Modifier.height(20.dp))

                // Action buttons
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    TextButton(onClick = onDiscard) {
                        Text(
                            "Discard pending edit",
                            color = MaterialTheme.colorScheme.onSurface,
                            fontSize = 13.sp,
                        )
                    }
                    Spacer(Modifier.width(8.dp))
                    Button(
                        onClick = onReloadAndRetry,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = MaterialTheme.colorScheme.primary,
                            contentColor = MaterialTheme.colorScheme.onPrimary,
                        ),
                    ) {
                        Text("Reload latest and retry pending edit", fontSize = 13.sp)
                    }
                }
            }
        }
    }
}
