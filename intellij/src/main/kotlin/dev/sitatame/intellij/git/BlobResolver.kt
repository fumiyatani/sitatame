package dev.sitatame.intellij.git

/**
 * Resolves the git blob SHA for a file at the current HEAD (index) via
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
     * Return the abbreviated (7-char) blob SHA of [relPath] at HEAD in
     * [repoRoot], or an empty string if the file is not tracked, git is
     * unavailable, or any error occurs.
     *
     * Uses `git ls-files -s -- <relPath>` which outputs:
     * `<mode> <40-char-sha> <stage>\t<path>`
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
                .redirectErrorStream(false)
                .start()
            val out = proc.inputStream.bufferedReader().readText().trim()
            proc.waitFor()
            // Output format: "100644 <sha40> 0\t<path>"
            // Field 1 (0-indexed) is the full SHA. Take first 7 chars.
            val sha40 = out.split(" ").getOrNull(1) ?: return ""
            if (sha40.length < 7) return ""
            sha40.substring(0, 7)
        } catch (_: Exception) {
            ""
        }
    }
}
