package dev.sitatame.intellij.storage

import java.io.File
import java.security.MessageDigest

/**
 * Port of `internal/review/slug.go`.
 *
 * The slug functions are the cross-route contract: the CLI, Web UI and
 * IntelliJ plugin must hash the same inputs into the same on-disk directory
 * names. Any drift here partitions storage and breaks cross-tool review
 * sharing, so this file is intentionally a straight transliteration of the
 * Go side rather than an "idiomatic Kotlin" rewrite.
 */
object Slug {

    private const val BRANCH_PREFIX_MAX = 32
    private const val BRANCH_HASH_LEN = 8

    /**
     * Returns `"<safe-basename>__<sha1-8>"` for a repository absolute path.
     *
     * Matches Go's `ProjectSlug` byte-for-byte:
     *  - basename via `filepath.Base` semantics (we use [File.getName] which
     *    matches for non-empty paths; empty/root/dot fall back to "project")
     *  - safePrefix replaces unsafe bytes with `_`, falls back to "project"
     *    (Go falls back to "branch" for [BranchSlug], but [ProjectSlug]
     *    overrides that to "project" for readability — keep that override).
     *  - sha1 of the UTF-8 bytes of the full path, hex-encoded, first 8 chars.
     */
    fun projectSlug(repoAbsPath: String): String {
        val base = baseName(repoAbsPath).let {
            if (it.isEmpty() || it == "." || it == File.separator) "project" else it
        }
        var prefix = safePrefix(base)
        if (prefix == "branch") {
            // Go: ProjectSlug overrides "branch" → "project" so the directory
            // reads as project__xxxx rather than branch__xxxx.
            prefix = "project"
        }
        return prefix + "__" + sha1Hex(repoAbsPath.toByteArray(Charsets.UTF_8)).substring(0, BRANCH_HASH_LEN)
    }

    /**
     * Returns `"<safe-prefix>__<sha1-8>"` per the PRD branch slug rules.
     * Matches Go's `BranchSlug` byte-for-byte, including
     *  - empty branch → `"branch__da39a3ee"` (sha1 of empty string)
     *  - all-unsafe branch → `"branch__<sha1-8>"` fallback
     *  - truncation to 32-char prefix.
     */
    fun branchSlug(branch: String): String {
        val prefix = safePrefix(branch)
        return prefix + "__" + sha1Hex(branch.toByteArray(Charsets.UTF_8)).substring(0, BRANCH_HASH_LEN)
    }

    /**
     * `filepath.Base` semantics: returns the last path element. For an empty
     * string Go returns ".", but the [projectSlug] caller normalises that, so
     * we just return empty here and let the caller decide.
     */
    private fun baseName(path: String): String {
        if (path.isEmpty()) return ""
        var p = path
        while (p.length > 1 && p.endsWith(File.separator)) p = p.dropLast(1)
        val idx = p.lastIndexOf(File.separatorChar)
        if (idx < 0) return p
        return p.substring(idx + 1)
    }

    private fun safePrefix(branch: String): String {
        if (branch.isEmpty()) return "branch"
        // Go iterates the branch as bytes and slices `head[:32]` — that is a
        // byte slice. To match byte-for-byte for non-ASCII branches we have
        // to mirror that here: take the UTF-8 bytes, truncate at 32 bytes,
        // then walk one byte at a time. Multi-byte UTF-8 sequences fail
        // isSafeByte (> 0x7F) and become underscores on both sides, so the
        // outputs agree exactly.
        val fullBytes = branch.toByteArray(Charsets.UTF_8)
        val byteLen = minOf(fullBytes.size, BRANCH_PREFIX_MAX)
        val buf = StringBuilder(byteLen)
        var hasSafe = false
        for (i in 0 until byteLen) {
            val c = fullBytes[i].toInt() and 0xFF
            if (isSafeByte(c)) {
                buf.append(c.toChar())
                hasSafe = true
            } else {
                buf.append('_')
            }
        }
        return if (!hasSafe) "branch" else buf.toString()
    }

    private fun isSafeByte(c: Int): Boolean =
        (c in 'a'.code..'z'.code) ||
        (c in 'A'.code..'Z'.code) ||
        (c in '0'.code..'9'.code) ||
        c == '.'.code || c == '_'.code || c == '-'.code

    private fun sha1Hex(bytes: ByteArray): String {
        val md = MessageDigest.getInstance("SHA-1")
        val sum = md.digest(bytes)
        val sb = StringBuilder(sum.size * 2)
        for (b in sum) {
            val v = b.toInt() and 0xFF
            sb.append(HEX[v ushr 4])
            sb.append(HEX[v and 0x0F])
        }
        return sb.toString()
    }

    private val HEX = "0123456789abcdef".toCharArray()
}
