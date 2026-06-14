package dev.sitatame.web.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
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

/**
 * A segmented control (or dropdown fallback for narrow width) that lets the
 * user choose which thread states to display.
 *
 * - At 320 dp and above: four inline segments `[All | Open | Done | Stale]`.
 * - Below 320 dp: a dropdown `ExposedDropdownMenuBox`-style button to avoid
 *   horizontal overflow.
 *
 * [selected] is the currently active filter; [onSelect] is called when the
 * user taps a different filter.
 */
@Composable
fun StateFilterControl(
    selected: StateFilter,
    onSelect: (StateFilter) -> Unit,
    modifier: Modifier = Modifier,
) {
    BoxWithConstraints(modifier = modifier.fillMaxWidth()) {
        if (maxWidth >= 320.dp) {
            SegmentedFilterRow(selected = selected, onSelect = onSelect)
        } else {
            DropdownFilter(selected = selected, onSelect = onSelect)
        }
    }
}

// ---------------------------------------------------------------------------
// Segmented row — shown when the sidebar has enough horizontal room.
// ---------------------------------------------------------------------------

private val filterLabels = listOf(
    StateFilter.All to "All",
    StateFilter.Open to "Open",
    StateFilter.Resolved to "Done",
    StateFilter.Stale to "Stale",
)

@Composable
private fun SegmentedFilterRow(
    selected: StateFilter,
    onSelect: (StateFilter) -> Unit,
) {
    val colors = LocalSitatameColors.current
    val shape = RoundedCornerShape(6.dp)

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 8.dp, vertical = 6.dp),
        horizontalArrangement = Arrangement.spacedBy(0.dp),
    ) {
        filterLabels.forEachIndexed { idx, (filter, label) ->
            val isSelected = filter == selected
            val cornerRadius = 6.dp
            val segmentShape = when {
                filterLabels.size == 1 -> RoundedCornerShape(cornerRadius)
                idx == 0 -> RoundedCornerShape(
                    topStart = cornerRadius,
                    bottomStart = cornerRadius,
                )
                idx == filterLabels.size - 1 -> RoundedCornerShape(
                    topEnd = cornerRadius,
                    bottomEnd = cornerRadius,
                )
                else -> RoundedCornerShape(0.dp)
            }
            Box(
                modifier = Modifier
                    .weight(1f)
                    .background(
                        color = if (isSelected) MaterialTheme.colorScheme.primary.copy(alpha = 0.20f)
                        else Color.Transparent,
                        shape = segmentShape,
                    )
                    .border(
                        width = 1.dp,
                        color = if (isSelected) MaterialTheme.colorScheme.primary
                        else colors.border,
                        shape = segmentShape,
                    )
                    .clickable { onSelect(filter) }
                    .padding(horizontal = 4.dp, vertical = 5.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    text = label,
                    color = if (isSelected) MaterialTheme.colorScheme.primary
                    else MaterialTheme.colorScheme.onSurface,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 11.sp,
                    fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal,
                    maxLines = 1,
                )
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Dropdown fallback — shown when width < 320dp.
// ---------------------------------------------------------------------------

@Composable
private fun DropdownFilter(
    selected: StateFilter,
    onSelect: (StateFilter) -> Unit,
) {
    val colors = LocalSitatameColors.current
    var expanded by remember { mutableStateOf(false) }
    val selectedLabel = filterLabels.first { it.first == selected }.second

    Box(
        modifier = Modifier
            .padding(horizontal = 8.dp, vertical = 6.dp),
    ) {
        Box(
            modifier = Modifier
                .width(120.dp)
                .background(
                    color = MaterialTheme.colorScheme.surface,
                    shape = RoundedCornerShape(6.dp),
                )
                .border(
                    width = 1.dp,
                    color = colors.border,
                    shape = RoundedCornerShape(6.dp),
                )
                .clickable { expanded = true }
                .padding(horizontal = 10.dp, vertical = 5.dp),
        ) {
            Text(
                text = "Filter: $selectedLabel",
                color = MaterialTheme.colorScheme.onSurface,
                fontFamily = FontFamily.Monospace,
                fontSize = 11.sp,
            )
        }
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false },
        ) {
            filterLabels.forEach { (filter, label) ->
                DropdownMenuItem(
                    text = {
                        Text(
                            text = label,
                            color = if (filter == selected) MaterialTheme.colorScheme.primary
                            else MaterialTheme.colorScheme.onSurface,
                            fontFamily = FontFamily.Monospace,
                            fontSize = 12.sp,
                        )
                    },
                    onClick = {
                        onSelect(filter)
                        expanded = false
                    },
                )
            }
        }
    }
}
