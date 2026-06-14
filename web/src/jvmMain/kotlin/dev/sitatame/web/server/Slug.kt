package dev.sitatame.web.server

import java.security.MessageDigest

/**
 * Kotlin port of `internal/review/slug.go`.
 *
 * MUST stay bit-for-bit equivalent to the Go side because the Web UI and the
 * TUI read/write the same on-disk directory structure
 * (~/.sitatame/<project-slug>/<branch-slug>/review.md). Any divergence
 * here means the two clients silently look at different review files for the
 * same branch.
 *
 * The matching tests live in SlugTest.kt.
 */
object Slug {

    private const val BRANCH_PREFIX_MAX = 32
    private const val BRANCH_HASH_LEN = 8

    /**
     * Returns "<safe-basename>__<sha1-8>" for an absolute repository path. See
     * Go's review.ProjectSlug for the exact semantics — in particular the
     * empty / "." / "/" fallback to "project" and the safePrefix == "branch"
     * remap to "project".
     */
    fun projectSlug(repoAbsPath: String): String {
        var base = basename(repoAbsPath)
        if (base.isEmpty() || base == "." || base == "/") {
            base = "project"
        }
        var prefix = safePrefix(base)
        if (prefix == "branch") {
            // safePrefix falls back to "branch" when nothing safe survives;
            // for a project slug "project" reads better. Matches Go.
            prefix = "project"
        }
        val hash = sha1Hex(repoAbsPath.toByteArray(Charsets.UTF_8)).substring(0, BRANCH_HASH_LEN)
        return prefix + "__" + hash
    }

    /**
     * Returns "<safe-prefix>__<sha1-8>" per the PRD branch slug rules. Matches
     * Go's review.BranchSlug exactly, including the empty-branch case
     * (BranchSlug("") == "branch__da39a3ee").
     */
    fun branchSlug(branch: String): String {
        val prefix = safePrefix(branch)
        val hash = sha1Hex(branch.toByteArray(Charsets.UTF_8)).substring(0, BRANCH_HASH_LEN)
        return prefix + "__" + hash
    }

    private fun safePrefix(branch: String): String {
        if (branch.isEmpty()) return "branch"
        val head = if (branch.length > BRANCH_PREFIX_MAX) branch.substring(0, BRANCH_PREFIX_MAX) else branch
        val buf = StringBuilder(head.length)
        var hasSafe = false
        // Go iterates byte-by-byte. The PRD branch names are ASCII (git ref
        // names) so byte / char iteration agrees here; if a non-ASCII branch
        // ever shows up, multi-byte chars would have to be replaced byte-wise,
        // which Kotlin can't do without re-encoding. We keep the char loop
        // because the Go side's safePrefix is also intended for the ASCII
        // subset of ref names.
        for (i in head.indices) {
            val c = head[i]
            if (isSafeChar(c)) {
                buf.append(c)
                hasSafe = true
            } else {
                buf.append('_')
            }
        }
        return if (!hasSafe) "branch" else buf.toString()
    }

    private fun isSafeChar(c: Char): Boolean {
        return when (c) {
            in 'a'..'z' -> true
            in 'A'..'Z' -> true
            in '0'..'9' -> true
            '.', '_', '-' -> true
            else -> false
        }
    }

    /**
     * Compute basename the way Go's filepath.Base does for POSIX paths. The
     * server only runs under JVM where File.separator is platform-dependent,
     * but the on-disk repo path is always given to us as a slash-separated
     * absolute path from `git rev-parse --show-toplevel`. Anything Windowsy
     * is out of scope for the PoC.
     */
    private fun basename(path: String): String {
        if (path.isEmpty()) return ""
        val trimmed = path.trimEnd('/')
        if (trimmed.isEmpty()) return "/"
        val idx = trimmed.lastIndexOf('/')
        return if (idx < 0) trimmed else trimmed.substring(idx + 1)
    }

    private fun sha1Hex(bytes: ByteArray): String {
        val md = MessageDigest.getInstance("SHA-1")
        val digest = md.digest(bytes)
        return buildString(digest.size * 2) {
            for (b in digest) {
                val v = b.toInt() and 0xFF
                append(HEX[v ushr 4])
                append(HEX[v and 0x0F])
            }
        }
    }

    private val HEX = "0123456789abcdef".toCharArray()
}
