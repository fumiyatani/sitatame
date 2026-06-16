package dev.sitatame.intellij.toolwindow.panes

/**
 * Represents the file selection state shared across all three panes.
 *
 * [All] — no file is selected; the comment list shows all comments for the branch.
 * [One] — a specific file is selected; the comment list filters to that file only.
 *
 * Sealed so exhaustive `when` is enforced throughout the pane layer.
 */
sealed class FileSelection {
    /** No file selected — show all comments. */
    object All : FileSelection()

    /** A specific file is selected. [relPath] is the repo-relative path. */
    data class One(val relPath: String) : FileSelection()
}
