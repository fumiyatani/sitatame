package dev.sitatame.intellij.git

import java.util.concurrent.TimeUnit

/**
 * Resolves the git blob SHA for a file in the **git index** via
 * `git ls-files -s`. This is the cross-surface stale-detection key: when
 * the file is modified after a comment was authored, the blob SHA changes
 * and validate.go marks the comment stale.
 *
 * We intentionally avoid Git4Idea APIs here so this class can be exercised
 * in plain JUnit tests without the IntelliJ Platform test framework.
 * ProcessBuilder shelling out to `git` mirrors the approach taken in other
 * lightweight CLI-backed resolver utilities in sitatame.
 *
 * Thread safety: [headBlobSha] is safe to call from any thread. The action
 * layer calls it on a background thread (Task.Backgroundable) to avoid
 * blocking the EDT.
 */
object BlobResolver {

    /**
     * Return the abbreviated (7-char) blob SHA of [relPath] from the
     * **git index** (not HEAD commit, not working tree) in [repoRoot], or an
     * empty string if the file is not tracked, git is unavailable, or any
     * error occurs.
     *
     * Uses `git ls-files -s -- <relPath>` which outputs one line per index
     * stage in the format `<mode> <40-char-sha> <stage>\t<path>`.
     *
     * **Index vs HEAD vs working tree**: `git ls-files -s` reads the index,
     * not the HEAD commit and not the working tree. A file that has been
     * edited but not yet staged (`git add`) returns the previously-staged
     * blob SHA. This is intentional: stale detection compares against the
     * blob that was current when the comment was authored (which was also
     * sourced from the index at save time), so using the index consistently
     * avoids false-positive stale marks for in-flight edits.
     *
     * **Conflicted index**: during a merge conflict `git ls-files -s` emits
     * three lines (stages 1/2/3). We prefer stage 0 (normal) or stage 2
     * (ours / HEAD side of the merge). If neither is present we fall back to
     * the first available line.
     *
     * The abbreviated form (first 7 chars) matches the blob SHAs stored in
     * `FileMeta.blobHead` / `FileMeta.blobBase` by the Go CLI, keeping the
     * stale-detection comparison consistent.
     */
    /**
     * Return true if [relPath] has been deleted from the git index (i.e. the
     * file is no longer tracked at HEAD or in the staging area). Uses
     * `git ls-files -s -- <relPath>`: an empty result means the file is absent
     * from the index, which is the reliable proxy for "deleted" in the context
     * of file-scope FILE comments.
     *
     * This mirrors Go's `fileScopeSide`: deleted files (Status == StatusDeleted)
     * have no head blob, so the FILE anchor must use SideBase to avoid an
     * immediately-stale anchor (Go's validateAnchor stales anchors whose blob
     * no longer matches). See modal.go:136-141.
     *
     * Returns false when [repoRoot] or [relPath] is blank, or on any error.
     */
    fun isDeletedFromIndex(repoRoot: String, relPath: String): Boolean {
        if (repoRoot.isEmpty() || relPath.isEmpty()) return false
        return try {
            val proc = ProcessBuilder("git", "ls-files", "-s", "--", relPath)
                .directory(java.io.File(repoRoot))
                .redirectErrorStream(true)
                .start()
            val out = proc.inputStream.bufferedReader().readText()
            if (!proc.waitFor(5, TimeUnit.SECONDS)) {
                proc.destroyForcibly()
                return false
            }
            if (proc.exitValue() != 0) return false
            // Empty output → file absent from index → deleted.
            out.isBlank()
        } catch (_: Exception) {
            false
        }
    }

    /**
     * Return the abbreviated (7-char) blob SHA of [relPath] from the
     * **git index** (not HEAD commit, not working tree) in [repoRoot], or an
     * empty string if the file is not tracked, git is unavailable, or any
     * error occurs.
     *
     * Uses `git ls-files -s -- <relPath>` which outputs one line per index
     * stage in the format `<mode> <40-char-sha> <stage>\t<path>`.
     *
     * **Index vs HEAD vs working tree**: `git ls-files -s` reads the index,
     * not the HEAD commit and not the working tree. A file that has been
     * edited but not yet staged (`git add`) returns the previously-staged
     * blob SHA. This is intentional: stale detection compares against the
     * blob that was current when the comment was authored (which was also
     * sourced from the index at save time), so using the index consistently
     * avoids false-positive stale marks for in-flight edits.
     *
     * **Conflicted index**: during a merge conflict `git ls-files -s` emits
     * three lines (stages 1/2/3). We prefer stage 0 (normal) or stage 2
     * (ours / HEAD side of the merge). If neither is present we fall back to
     * the first available line.
     *
     * The abbreviated form (first 7 chars) matches the blob SHAs stored in
     * `FileMeta.blobHead` / `FileMeta.blobBase` by the Go CLI, keeping the
     * stale-detection comparison consistent.
     */
    fun headBlobSha(repoRoot: String, relPath: String): String {
        if (repoRoot.isEmpty() || relPath.isEmpty()) return ""
        return try {
            val proc = ProcessBuilder("git", "ls-files", "-s", "--", relPath)
                .directory(java.io.File(repoRoot))
                .redirectErrorStream(true)   // drain stderr into stdout to prevent pipe-buffer hang
                .start()
            val out = proc.inputStream.bufferedReader().readText()
            if (!proc.waitFor(5, TimeUnit.SECONDS)) {
                proc.destroyForcibly()
                return ""
            }
            if (proc.exitValue() != 0) return ""

            // Output format: "<mode> <sha40> <stage>\t<path>"
            // Prefer stage 0 (normal) or stage 2 (ours/HEAD during merge conflict).
            val lines = out.lineSequence().filter { it.isNotBlank() }.toList()
            if (lines.isEmpty()) return ""

            val candidate = lines.firstOrNull { line ->
                val parts = line.split(Regex("\\s+"), limit = 4)
                parts.getOrNull(2) == "0" || parts.getOrNull(2) == "2"
            } ?: lines.first()

            val tokens = candidate.split(Regex("\\s+"), limit = 4)
            if (tokens.size < 4) return ""
            val sha40 = tokens[1]
            if (!sha40.matches(Regex("[0-9a-fA-F]{7,40}"))) return ""
            sha40.substring(0, 7)
        } catch (_: Exception) {
            ""
        }
    }
}
