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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import dev.sitatame.web.api.WorkspaceResponse

/**
 * Root composable. Owns the load -> render state machine and the GitHub-style
 * 2-pane layout.
 *
 *  - top bar (branch + base/head refs)
 *  - row { sidebar | main pane }
 *  - status bar
 */
@Composable
fun SitatameApp() {
    SitatameTheme {
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = MaterialTheme.colorScheme.background,
        ) {
            var state by remember { mutableStateOf<WorkspaceState>(WorkspaceState.Loading) }

            LaunchedEffect(Unit) {
                state = try {
                    val response = fetchWorkspace()
                    WorkspaceState.Loaded(response)
                } catch (e: Throwable) {
                    WorkspaceState.Error(e.message ?: "unknown error")
                }
            }

            when (val s = state) {
                WorkspaceState.Loading -> LoadingView()
                is WorkspaceState.Error -> ErrorView(s.message)
                is WorkspaceState.Loaded -> LoadedView(s.workspace)
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
private fun LoadedView(workspace: WorkspaceResponse) {
    var selectedPath by remember(workspace.files) {
        mutableStateOf(workspace.files.firstOrNull()?.path)
    }
    val colors = LocalSitatameColors.current

    Column(modifier = Modifier.fillMaxSize()) {
        TopBar(workspace)
        Row(modifier = Modifier.fillMaxSize().weight(1f)) {
            Sidebar(
                files = workspace.files,
                comments = workspace.review?.comments.orEmpty(),
                selectedPath = selectedPath,
                onSelect = { selectedPath = it },
                modifier = Modifier
                    .width(320.dp)
                    .fillMaxSize()
                    .background(MaterialTheme.colorScheme.surface),
            )
            // Vertical divider.
            Box(
                modifier = Modifier
                    .width(1.dp)
                    .fillMaxSize()
                    .background(colors.border),
            )
            MainPane(
                file = workspace.files.firstOrNull { it.path == selectedPath },
                comments = workspace.review?.comments.orEmpty(),
                modifier = Modifier.weight(1f).fillMaxSize(),
            )
        }
        StatusBar(workspace)
    }
}

@Composable
private fun TopBar(workspace: WorkspaceResponse) {
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
    }
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(1.dp)
            .background(colors.border),
    )
}

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
