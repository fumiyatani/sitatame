package dev.sitatame.web.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * GitHub Dimmed Dark-style Material3 colour scheme + diff-row colours that
 * Material3 doesn't model. The diff colours are exposed via a CompositionLocal
 * because Material3 has no "addLine" slot to override.
 */
@Immutable
data class SitatameColors(
    val addLineBg: Color,
    val addLineGutter: Color,
    val delLineBg: Color,
    val delLineGutter: Color,
    val ctxLineBg: Color,
    val hunkHeaderBg: Color,
    val sidebarHighlight: Color,
    val openBadge: Color,
    val resolvedBadge: Color,
    val staleBadge: Color,
    val mutedText: Color,
    val border: Color,
)

val LocalSitatameColors = compositionLocalOf {
    sitatameDarkColors()
}

private fun sitatameDarkColors() = SitatameColors(
    // Soft greens — GitHub success-emphasis muted so the foreground text
    // stays readable without an explicit override.
    addLineBg = Color(0xFF033A16),
    addLineGutter = Color(0xFF1F6F3E),
    delLineBg = Color(0xFF67060C),
    delLineGutter = Color(0xFF8B2A30),
    ctxLineBg = Color(0xFF0D1117),
    hunkHeaderBg = Color(0xFF161B22),
    sidebarHighlight = Color(0xFF21262D),
    openBadge = Color(0xFFF78166),
    resolvedBadge = Color(0xFF7EE787),
    staleBadge = Color(0xFFD29922),
    mutedText = Color(0xFF8B949E),
    border = Color(0xFF30363D),
)

private val DarkColorScheme = darkColorScheme(
    background = Color(0xFF0D1117),    // canvas-default
    surface = Color(0xFF161B22),       // canvas-subtle (sidebar / cards)
    surfaceVariant = Color(0xFF21262D),// canvas-inset
    primary = Color(0xFF58A6FF),       // accent-fg
    secondary = Color(0xFF7EE787),     // success-fg (for indirect highlights)
    onBackground = Color(0xFFC9D1D9),  // fg-default
    onSurface = Color(0xFFC9D1D9),
    onSurfaceVariant = Color(0xFFC9D1D9),
    onPrimary = Color(0xFF0D1117),
    outline = Color(0xFF30363D),       // borders
    error = Color(0xFFF85149),
    onError = Color(0xFF0D1117),
)

@Composable
fun SitatameTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = DarkColorScheme,
        content = content,
    )
}
