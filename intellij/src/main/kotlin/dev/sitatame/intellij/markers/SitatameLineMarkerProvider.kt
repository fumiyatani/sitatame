package dev.sitatame.intellij.markers

import com.intellij.codeInsight.daemon.LineMarkerInfo
import com.intellij.codeInsight.daemon.LineMarkerProvider
import com.intellij.icons.AllIcons
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.editor.markup.GutterIconRenderer
import com.intellij.psi.PsiElement
import com.intellij.psi.PsiFile
import dev.sitatame.intellij.git.RepoContext
import dev.sitatame.intellij.storage.AnchorKind
import dev.sitatame.intellij.storage.Comment
import dev.sitatame.intellij.storage.ReviewState
import dev.sitatame.intellij.storage.ReviewStore
import javax.swing.Icon

/**
 * Draws a gutter icon for each editor line that has a sitatame review comment.
 *
 * Registered in plugin.xml as:
 *   `<lineMarkerProvider language="any" implementationClass="..."/>`
 *
 * Implementation notes:
 * - [getLineMarkerInfo] is called on the EDT by the IntelliJ highlighting
 *   pipeline for every leaf PsiElement. We only act on the first leaf token
 *   of a line to avoid multiple markers per line. File I/O is avoided by
 *   reading from the ReviewStore's in-memory cache (snapshotComments), which
 *   is O(n) and lock-free on the read path.
 * - Stale detection: an anchor whose `blob` field is non-empty but does not
 *   match the current head blob would be stale. Because blob population is
 *   tracked in a separate issue (#103), we take the safe fallback: if `blob`
 *   is empty the comment is NOT shown as stale (we cannot tell). If blob is
 *   non-empty, staleness check is still skipped until #103 lands.
 * - AnchorKind.FILE and AnchorKind.REVIEW are not shown in the gutter.
 */
class SitatameLineMarkerProvider : LineMarkerProvider {

    override fun getLineMarkerInfo(element: PsiElement): LineMarkerInfo<*>? {
        val file = element.containingFile ?: return null

        // Only act on the first leaf element of a line (PsiFile children are
        // leaves via getFirstChild chains; a faster heuristic: skip if the
        // previous sibling occupies the same line).
        if (!isFirstLeafOnLine(element)) return null

        val project = element.project
        val virtualFile = file.virtualFile ?: return null

        val repo = RepoContext.forFile(project, virtualFile) ?: return null
        if (repo.repoRoot.isEmpty()) return null

        val store = ApplicationManager.getApplication().getService(ReviewStore::class.java)
            ?: return null

        val relPath = relativise(virtualFile.path, repo.repoRoot)

        // 1-based line number of this element.
        val doc = com.intellij.openapi.fileEditor.FileDocumentManager.getInstance()
            .getDocument(virtualFile) ?: return null
        val elementLine = doc.getLineNumber(element.textOffset) + 1  // 1-based

        val comments = store.snapshotComments(repo.repoRoot, repo.branch)
        val matches = commentsForLine(comments, relPath, elementLine)
        if (matches.isEmpty()) return null

        // Aggregate: if any is open → open icon; if all resolved → resolved;
        // stale takes priority over open when blob check is available (#103).
        val icon = aggregateIcon(matches)

        val tooltip = buildTooltip(matches)

        return LineMarkerInfo(
            element,
            element.textRange,
            icon,
            { _: PsiElement -> tooltip },
            null,
            GutterIconRenderer.Alignment.LEFT,
            { tooltip },
        )
    }

    // -- Helpers exposed as internal for unit tests --------------------------

    /**
     * Return all comments whose anchor covers [lineNumber] in [relPath].
     * Handles LINE (exact match) and RANGE (start ≤ line ≤ end) anchors.
     * FILE and REVIEW anchors are excluded per the spec.
     */
    internal fun commentsForLine(
        comments: List<Comment>,
        relPath: String,
        lineNumber: Int,
    ): List<Comment> = comments.filter { c ->
        if (c.anchor.path != relPath) return@filter false
        when (c.anchor.kind) {
            AnchorKind.LINE -> c.anchor.line == lineNumber
            AnchorKind.RANGE -> lineNumber in c.anchor.lineStart..c.anchor.lineEnd
            else -> false  // FILE / REVIEW: not shown in gutter
        }
    }

    /**
     * Choose the summary icon for a non-empty list of comments on one line.
     *
     * Priority: stale (currently: comment with non-empty blob, #103 pending)
     *   > open > resolved.
     */
    internal fun aggregateIcon(comments: List<Comment>): Icon {
        // Stale: blob non-empty indicates we *could* check staleness.
        // Until #103 lands the blob is always empty, so this path is dormant
        // but structurally correct.
        if (comments.any { it.state == ReviewState.STALE }) {
            return AllIcons.General.Warning
        }
        // All resolved → resolved icon.
        if (comments.all { it.state == ReviewState.RESOLVED }) {
            return AllIcons.Actions.Checked
        }
        // At least one open.
        return AllIcons.General.Note
    }

    private fun buildTooltip(comments: List<Comment>): String {
        if (comments.size == 1) {
            val c = comments[0]
            val stateLabel = c.state.replaceFirstChar { it.uppercase() }
            val preview = c.body.lineSequence().firstOrNull().orEmpty().take(80)
            return "sitatame [$stateLabel]: $preview"
        }
        val openCount = comments.count { it.state == ReviewState.OPEN }
        val resolvedCount = comments.count { it.state == ReviewState.RESOLVED }
        return buildString {
            append("sitatame: ${comments.size} comments")
            if (openCount > 0) append(" ($openCount open")
            if (resolvedCount > 0) {
                if (openCount > 0) append(", $resolvedCount resolved)")
                else append(" ($resolvedCount resolved)")
            } else if (openCount > 0) {
                append(")")
            }
        }
    }

    // -- Private helpers -------------------------------------------------------

    /**
     * Returns true when [element] is the first PSI leaf on its line.
     *
     * Strategy: walk the PSI tree backwards (prevLeaf) and check whether any
     * predecessor occupies the same line. This avoids emitting multiple markers
     * for a single line when the line has several leaf tokens.
     */
    private fun isFirstLeafOnLine(element: PsiElement): Boolean {
        // PsiFile itself is never a leaf marker target.
        if (element is PsiFile) return false
        // Only consider actual leaf nodes (no children).
        if (element.firstChild != null) return false

        val doc = element.containingFile?.virtualFile?.let {
            com.intellij.openapi.fileEditor.FileDocumentManager.getInstance().getDocument(it)
        } ?: return true  // can't tell → allow

        val elementLine = doc.getLineNumber(element.textOffset)
        var prev = com.intellij.psi.util.PsiTreeUtil.prevLeaf(element)
        while (prev != null) {
            val prevLine = doc.getLineNumber(prev.textOffset)
            if (prevLine < elementLine) break   // previous leaf is on an earlier line → we are first
            if (prevLine == elementLine) return false  // same line → not first
            prev = com.intellij.psi.util.PsiTreeUtil.prevLeaf(prev)
        }
        return true
    }

    private fun relativise(absPath: String, repoRoot: String): String {
        if (repoRoot.isEmpty()) return absPath
        val prefix = if (repoRoot.endsWith('/')) repoRoot else "$repoRoot/"
        return if (absPath.startsWith(prefix)) absPath.removePrefix(prefix) else absPath
    }
}
