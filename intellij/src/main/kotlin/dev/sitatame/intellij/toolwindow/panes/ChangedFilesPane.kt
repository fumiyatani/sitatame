package dev.sitatame.intellij.toolwindow.panes

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileEditor.OpenFileDescriptor
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.ui.JBColor
import com.intellij.ui.components.JBList
import com.intellij.ui.components.JBScrollPane
import dev.sitatame.intellij.git.ChangedFilesProvider
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.Comment
import java.awt.BorderLayout
import java.awt.Component
import java.awt.event.MouseAdapter
import java.awt.event.MouseEvent
import javax.swing.DefaultListCellRenderer
import javax.swing.DefaultListModel
import javax.swing.JLabel
import javax.swing.JList
import javax.swing.JPanel
import javax.swing.ListSelectionModel

/**
 * Left pane of the 3-pane layout: shows files changed between baseRef and HEAD.
 *
 * Each row shows: [relPath]  [(open / total)] where counts are derived from the
 * current comment snapshot. Files with comments are shown in bold; files without
 * comments are shown in a dimmed colour.
 *
 * A special "All files" entry at position 0 deselects the file filter.
 *
 * Double-click opens the file in the editor.
 * Single-click notifies the middle pane via [onFileSelected].
 */
class ChangedFilesPane(
    private val project: Project,
    private val onFileSelected: (FileSelection) -> Unit,
) {

    private val listModel = DefaultListModel<FileEntry>()
    private val list = JBList(listModel).apply {
        selectionMode = ListSelectionModel.SINGLE_SELECTION
        cellRenderer = FileCellRenderer()
    }

    val component: JPanel = build()

    init {
        list.addListSelectionListener {
            if (!it.valueIsAdjusting) {
                val sel = when (val v = list.selectedValue) {
                    null, FileEntry.ALL_FILES -> FileSelection.All
                    else -> FileSelection.One(v.relPath)
                }
                onFileSelected(sel)
            }
        }
        list.addMouseListener(object : MouseAdapter() {
            override fun mouseClicked(e: MouseEvent) {
                if (e.clickCount == 2) jumpToSelectedFile()
            }
        })
    }

    private fun build(): JPanel {
        val panel = JPanel(BorderLayout())
        panel.add(JBScrollPane(list), BorderLayout.CENTER)
        return panel
    }

    // -----------------------------------------------------------------------
    // Public API
    // -----------------------------------------------------------------------

    /**
     * Refresh the file list for the given [baseRef]. Runs git diff on a
     * background thread, then returns to EDT to update the list model.
     * [comments] are used to compute per-file counts.
     *
     * [isCurrent] is evaluated on the EDT just before the model is replaced; if
     * it returns false the result is dropped. The caller passes its refresh
     * generation so a slow diff for an old base can't overwrite the file list
     * after a newer base has already been selected.
     */
    fun refresh(baseRef: String, comments: List<Comment>, isCurrent: () -> Boolean = { true }) {
        ApplicationManager.getApplication().executeOnPooledThread {
            val repo = RepoContext.forProject(project)
            val changedFiles = if (repo != null) {
                ChangedFilesProvider.listChangedFiles(repo.repoRoot, baseRef)
            } else {
                emptyList()
            }
            ApplicationManager.getApplication().invokeLater {
                if (!isCurrent()) return@invokeLater
                updateModel(changedFiles, comments)
            }
        }
    }

    /** Empty the file list (used when there is no repository context). EDT only. */
    fun clear() {
        updateModel(emptyList(), emptyList())
    }

    /**
     * Update comment counts without re-running git diff. Called when a comment
     * is added/resolved/deleted so counts stay current without a full refresh.
     */
    fun updateCounts(comments: List<Comment>) {
        val existingPaths = (0 until listModel.size())
            .map { listModel.getElementAt(it) }
            .filter { it != FileEntry.ALL_FILES }
            .map { it.relPath }
        updateModel(existingPaths, comments)
    }

    // -----------------------------------------------------------------------
    // Private helpers
    // -----------------------------------------------------------------------

    private fun updateModel(changedFiles: List<String>, comments: List<Comment>) {
        // Compute open and total counts per file.
        val totalByFile = comments.groupingBy { it.anchor.path }.eachCount()
        val openByFile = comments.filter { it.state != "resolved" }
            .groupingBy { it.anchor.path }.eachCount()

        // Preserve previous selection by relPath.
        val previousRelPath = (list.selectedValue as? FileEntry)?.relPath

        listModel.clear()
        listModel.addElement(FileEntry.ALL_FILES)
        for (path in changedFiles) {
            val total = totalByFile[path] ?: 0
            val open = openByFile[path] ?: 0
            listModel.addElement(FileEntry(path, open, total))
        }

        // Restore selection.
        if (previousRelPath != null) {
            val idx = (0 until listModel.size()).firstOrNull {
                listModel.getElementAt(it).relPath == previousRelPath
            }
            if (idx != null) list.selectedIndex = idx
        } else {
            list.selectedIndex = 0 // "All files"
        }
    }

    private fun jumpToSelectedFile() {
        val entry = list.selectedValue ?: return
        if (entry == FileEntry.ALL_FILES) return
        val repo = RepoContext.forProject(project) ?: return
        val absPath = "${repo.repoRoot}/${entry.relPath}"
        val vFile = LocalFileSystem.getInstance().findFileByPath(absPath) ?: return
        OpenFileDescriptor(project, vFile).navigate(true)
    }

    // -----------------------------------------------------------------------
    // Data
    // -----------------------------------------------------------------------

    /** A row in the changed-files list. The sentinel ALL_FILES has an empty relPath. */
    data class FileEntry(val relPath: String, val openCount: Int, val totalCount: Int) {
        val hasComments: Boolean get() = totalCount > 0
        val isAllFiles: Boolean get() = relPath.isEmpty()

        companion object {
            val ALL_FILES = FileEntry("", 0, 0)
        }
    }

    // -----------------------------------------------------------------------
    // Cell renderer
    // -----------------------------------------------------------------------

    private class FileCellRenderer : DefaultListCellRenderer() {
        private val dimColor = JBColor(0x888888, 0x777777)

        override fun getListCellRendererComponent(
            list: JList<*>?,
            value: Any?,
            index: Int,
            isSelected: Boolean,
            cellHasFocus: Boolean,
        ): Component {
            val label = super.getListCellRendererComponent(
                list, value, index, isSelected, cellHasFocus
            ) as JLabel

            val entry = value as? FileEntry
            if (entry == null || entry.isAllFiles) {
                label.text = "All files"
                return label
            }

            val fileName = entry.relPath.substringAfterLast("/").ifEmpty { entry.relPath }
            val countText = if (entry.hasComments) " [${entry.openCount}/${entry.totalCount}]" else ""
            label.text = if (entry.hasComments) {
                "<html><b>$fileName</b><span>$countText</span></html>"
            } else {
                "<html><span>$fileName</span></html>"
            }
            label.toolTipText = entry.relPath + countText

            if (!isSelected && !entry.hasComments) {
                label.foreground = dimColor
            }

            return label
        }
    }
}
