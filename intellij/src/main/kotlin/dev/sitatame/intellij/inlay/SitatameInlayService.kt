package dev.sitatame.intellij.inlay

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.Inlay
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.vfs.VirtualFile
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore

/**
 * Attaches block inlays to an [Editor] for all sitatame [Comment]s whose
 * anchor path matches the file being viewed.
 *
 * **Lifecycle**: called from [SitatameEditorFactoryListener.editorCreated].
 * Inlays are automatically cleaned up when the editor is disposed; no
 * explicit teardown is needed.
 *
 * **Threading**: [attachInlays] schedules a background task to load comments
 * (I/O), then switches to EDT to place the inlays (InlayModel requires EDT).
 *
 * **Refresh**: on editor-open only. Re-attachment after mutation is deferred
 * to the REVIEW_CHANGED_TOPIC integration (tracked in #117 Phase 2 / #114).
 */
object SitatameInlayService {

    private val log = Logger.getInstance(SitatameInlayService::class.java)

    /**
     * Load comments from [ReviewStore] on a background thread, then attach
     * block inlays on EDT for the file shown in [editor].
     *
     * Safe to call from any thread; scheduling is handled internally.
     */
    fun attachInlays(editor: Editor) {
        // Guard: only operate on real file editors (not diff / console / etc.)
        val project = editor.project ?: return
        val file = fileFor(editor) ?: return

        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "sitatame: loading inlays", false) {
                override fun run(indicator: ProgressIndicator) {
                    val repo = RepoContext.forFile(project, file) ?: return
                    val store = ApplicationManager.getApplication()
                        .getService(ReviewStore::class.java)
                    val comments = try {
                        store.snapshotComments(repo.repoRoot, repo.branch)
                    } catch (e: Exception) {
                        log.warn("sitatame inlay: failed to load comments", e)
                        return
                    }

                    val relPath = relativise(file.path, repo.repoRoot)
                    val groups = groupByLine(comments, relPath)
                    if (groups.isEmpty()) return

                    ApplicationManager.getApplication().invokeLater {
                        placeInlays(editor, store, repo.repoRoot, repo.branch, groups)
                    }
                }
            }
        )
    }

    /**
     * Filter [comments] to those anchored to [relPath] and group them by
     * their effective anchor line number (1-based).
     *
     * - LINE anchors use `anchor.line`
     * - RANGE anchors use `anchor.lineStart`
     * - Other kinds are ignored (FILE / REVIEW level comments have no line)
     *
     * Groups are sorted by line ascending. This function is package-internal
     * and pure (no I/O), making it directly unit-testable without a platform
     * instance.
     */
    internal fun groupByLine(comments: List<Comment>, relPath: String): Map<Int, List<Comment>> {
        return comments
            .filter { c ->
                c.anchor.path == relPath &&
                    (c.anchor.kind == AnchorKind.LINE || c.anchor.kind == AnchorKind.RANGE)
            }
            .groupBy { c ->
                when (c.anchor.kind) {
                    AnchorKind.LINE -> c.anchor.line
                    AnchorKind.RANGE -> c.anchor.lineStart
                    else -> c.anchor.line  // unreachable after filter, kept for exhaustiveness
                }
            }
            .toSortedMap()
    }

    // -----------------------------------------------------------------------
    // EDT-only helpers
    // -----------------------------------------------------------------------

    private fun placeInlays(
        editor: Editor,
        store: ReviewStore,
        repoRoot: String,
        branch: String,
        groups: Map<Int, List<Comment>>,
    ) {
        val doc = editor.document
        val inlayModel = editor.inlayModel
        val placedInlays = mutableListOf<Inlay<SitatameCommentInlayRenderer>>()

        for ((line1based, commentGroup) in groups) {
            val lineIndex = line1based - 1  // convert to 0-based for Document API
            if (lineIndex < 0 || lineIndex >= doc.lineCount) {
                log.debug("sitatame inlay: line $line1based out of range (doc has ${doc.lineCount} lines)")
                continue
            }
            // Place the inlay at the end-of-line offset so it appears below the anchor line.
            val offset = doc.getLineEndOffset(lineIndex)

            val renderer = SitatameCommentInlayRenderer(
                comments = commentGroup,
                onToggle = { index -> handleToggle(editor, store, repoRoot, branch, commentGroup, index) },
            )

            val inlay: Inlay<SitatameCommentInlayRenderer>? = inlayModel.addBlockElement(
                offset,
                /*relatesToPrecedingText=*/ true,
                /*showAbove=*/ false,
                /*priority=*/ 0,
                renderer,
            )

            if (inlay != null) {
                placedInlays += inlay
            }
        }

        // Register a single mouse listener for all inlays in this editor.
        if (placedInlays.isNotEmpty()) {
            editor.addEditorMouseListener(SitatameInlayMouseListener(placedInlays))
        }
    }

    /**
     * Toggle the comment at [index] within [group] via [ReviewStore.toggleComment],
     * then repaint by invalidating the inlay. Runs a background task for I/O.
     */
    private fun handleToggle(
        editor: Editor,
        store: ReviewStore,
        repoRoot: String,
        branch: String,
        group: List<Comment>,
        index: Int,
    ) {
        if (index !in group.indices) return
        val target = group[index]
        val anchorId = target.anchor.anchorId
        val project = editor.project ?: return

        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, "sitatame: toggling comment", false) {
                override fun run(indicator: ProgressIndicator) {
                    try {
                        if (anchorId.isNotEmpty()) {
                            store.toggleComment(repoRoot, branch) { c -> c.anchor.anchorId == anchorId }
                        } else {
                            store.toggleComment(repoRoot, branch) { c ->
                                c.anchor.path == target.anchor.path &&
                                    c.anchor.line == target.anchor.line &&
                                    c.anchor.kind == target.anchor.kind
                            }
                        }
                    } catch (e: Exception) {
                        log.warn("sitatame inlay: toggleComment failed", e)
                    }
                    // Update the in-memory comment state so the renderer reflects
                    // the toggle immediately (without a full re-attach).
                    ApplicationManager.getApplication().invokeLater {
                        val newState = if (target.state == ReviewState.RESOLVED) ReviewState.OPEN else ReviewState.RESOLVED
                        target.state = newState
                        editor.contentComponent.repaint()
                    }
                }
            }
        )
    }

    private fun relativise(absPath: String, repoRoot: String): String {
        if (repoRoot.isEmpty()) return absPath
        val prefix = if (repoRoot.endsWith('/')) repoRoot else "$repoRoot/"
        return if (absPath.startsWith(prefix)) absPath.removePrefix(prefix) else absPath
    }

    private fun fileFor(editor: Editor): VirtualFile? =
        FileDocumentManager.getInstance().getFile(editor.document)
}
