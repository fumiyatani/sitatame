package dev.sitatame.web.server

import dev.sitatame.web.api.DiffLineDto
import dev.sitatame.web.api.FileDto
import dev.sitatame.web.api.HunkDto
import java.io.IOException
import java.nio.file.Path
import java.util.concurrent.TimeUnit

/**
 * Thin wrapper around the `git` CLI. The Web backend only needs three calls
 * for Phase 1 step 1:
 *
 *   git rev-parse --show-toplevel
 *   git symbolic-ref --quiet --short HEAD
 *   git diff <base>..HEAD --no-color --find-renames --find-copies
 *
 * The Go side (`internal/gitx`) is the source of truth for diff parsing in the
 * TUI; this Kotlin port is unified-diff only (Phase 1 step 1 scope) and does
 * not attempt to reproduce the raw / numstat join. Binary diffs are detected by
 * the patch parser when it sees the `Binary files ... differ` marker and emit
 * a file entry with an empty hunk list.
 */
class Git(private val workdir: Path) {

    /** Outcome of running git: stdout + stderr + exit code, regardless of success. */
    data class Result(val stdout: String, val stderr: String, val exitCode: Int)

    /**
     * Run `git <args>` in [workdir]; throws on non-zero exit.
     *
     * Callers that need to take a ref or other untrusted token as an argument
     * MUST place it after `--end-of-options` (git 2.24+) so a flag-shaped
     * value like `--upload-pack=...` cannot be misinterpreted. Mirrors the
     * Go-side discipline in `internal/gitx/repo.go`'s `RevParse`.
     */
    fun run(vararg args: String): String {
        val res = runRaw(*args)
        if (res.exitCode != 0) {
            throw IOException("git ${args.joinToString(" ")} exited ${res.exitCode}: ${res.stderr}")
        }
        return res.stdout
    }

    /**
     * Like [run] but returns the full [Result] without throwing on non-zero
     * exit. Used by [currentBranch] so the detached-HEAD case (exit 1 from
     * `symbolic-ref`) can be distinguished from real failures.
     */
    fun runRaw(vararg args: String): Result {
        val cmd = mutableListOf("git").apply { addAll(args) }
        val proc = ProcessBuilder(cmd)
            .directory(workdir.toFile())
            .redirectErrorStream(false)
            .start()
        val finished = proc.waitFor(30, TimeUnit.SECONDS)
        if (!finished) {
            proc.destroyForcibly()
            throw IOException("git ${args.joinToString(" ")} timed out")
        }
        val stdout = proc.inputStream.readAllBytes().toString(Charsets.UTF_8)
        val stderr = proc.errorStream.readAllBytes().toString(Charsets.UTF_8)
        return Result(stdout, stderr, proc.exitValue())
    }

    fun repoRoot(): Path {
        val out = run("rev-parse", "--show-toplevel").trim()
        return Path.of(out)
    }

    /**
     * Returns the symbolic name of HEAD (e.g. "feature/x"). Returns an empty
     * string when HEAD is detached.
     *
     * Mirrors `gitx.Repo.CurrentBranch`: `symbolic-ref --quiet --short HEAD`
     * exits 1 with empty stdout in detached state, which we explicitly map to
     * "". This is intentionally NOT `rev-parse --abbrev-ref HEAD`, because
     * that variant returns the literal string "HEAD" on detached, forcing the
     * caller to parity-check by string-comparison — fragile across git
     * versions and edge cases like submodule HEADs.
     */
    fun currentBranch(): String {
        val res = runRaw("symbolic-ref", "--quiet", "--short", "HEAD")
        return when (res.exitCode) {
            0 -> res.stdout.trim()
            // symbolic-ref exits 1 with empty stdout when HEAD is detached;
            // any other non-zero exit is a real failure we propagate.
            1 -> ""
            else -> throw IOException(
                "git symbolic-ref --quiet --short HEAD exited ${res.exitCode}: ${res.stderr}"
            )
        }
    }

    /**
     * Returns the SHA of HEAD as a hex string, or an empty string when HEAD
     * cannot be resolved (unborn / pathological repos). Used by
     * [normalizeBranch] to give detached HEAD sessions a per-SHA branch slug
     * matching the Go CLI behaviour in `cmd/root.go`.
     */
    fun headSHA(): String {
        return try {
            run("rev-parse", "HEAD").trim()
        } catch (_: Exception) {
            ""
        }
    }

    /**
     * Unified diff for `<base>..HEAD`. The result is the literal `git diff`
     * output; parse with [DiffParser.parse].
     *
     * `--end-of-options` (git 2.24+) ensures [base] cannot be misinterpreted
     * as a flag — important once per-repo configuration starts to feed
     * untrusted strings here (Phase 1 step 2 / #67). Mirrors Go's
     * `gitx.RevParse`.
     */
    fun unifiedDiff(base: String): String {
        return run(
            "diff",
            "--no-color",
            "--find-renames",
            "--find-copies",
            "--end-of-options",
            "$base..HEAD",
        )
    }
}

/**
 * Minimal unified-diff parser.
 *
 * Scope:
 *  - Parses `diff --git a/<path> b/<path>` blocks.
 *  - Detects status M / A / D / R from the index / new-file / deleted-file /
 *    similarity / rename headers.
 *  - Parses `@@ -<bs>,<bn> +<hs>,<hn> @@ <header>` hunk markers and ' ', '+',
 *    '-' lines. The two `\ No newline at end of file` markers are skipped.
 *  - Counts adds / dels by tallying the prefix characters in hunks.
 *  - Binary diffs: the file is emitted with an empty hunk list. Phase 1
 *    step 1 does not render them differently in the UI yet.
 *
 * Out of scope (left for the Go-side parity port):
 *  - Mode-only changes without a hunk body.
 *  - Combined diffs (merge commits).
 *  - --textconv processed output.
 */
object DiffParser {

    private data class FileBuilder(
        var prePath: String? = null,
        var postPath: String? = null,
        var status: String = "M",
        var renameFrom: String? = null,
        var renameTo: String? = null,
        val hunks: MutableList<HunkDto> = mutableListOf(),
        var adds: Int = 0,
        var dels: Int = 0,
        /** Abbreviated blob SHA for base side; parsed from `index <a>..<b>` header. */
        var blobBase: String? = null,
        /** Abbreviated blob SHA for head side; parsed from `index <a>..<b>` header. */
        var blobHead: String? = null,
    )

    fun parse(diff: String): List<FileDto> {
        if (diff.isBlank()) return emptyList()
        val lines = diff.split('\n')

        val out = mutableListOf<FileDto>()
        var current: FileBuilder? = null

        var i = 0
        while (i < lines.size) {
            val line = lines[i]

            if (line.startsWith("diff --git ")) {
                current?.let { out += build(it) }
                current = FileBuilder()
                val paths = parseDiffGitHeader(line)
                current.prePath = paths.first
                current.postPath = paths.second
                i++
                continue
            }
            val cur = current
            if (cur == null) {
                i++
                continue
            }

            when {
                line.startsWith("new file mode") -> cur.status = "A"
                line.startsWith("deleted file mode") -> cur.status = "D"
                line.startsWith("rename from ") -> {
                    cur.status = "R"
                    cur.renameFrom = line.removePrefix("rename from ")
                }
                line.startsWith("rename to ") -> {
                    cur.status = "R"
                    cur.renameTo = line.removePrefix("rename to ")
                }
                line.startsWith("index ") -> {
                    // `index <blobBase>..<blobHead>[ <mode>]`
                    // Example: `index caab94d..f02d7d6 100644`
                    // Deleted files have `0000000` as blobHead; new files have
                    // `0000000` as blobBase. We store them as-is so the Frontend
                    // can choose the correct SHA for the anchor side.
                    parseBlobIndex(line)?.let { (base, head) ->
                        cur.blobBase = base
                        cur.blobHead = head
                    }
                }
                line.startsWith("Binary files ") -> {
                    // Leave hunks empty; the FE will fall back to a "binary"
                    // hint when it sees zero hunks but a non-context status.
                }
                line.startsWith("@@") -> {
                    val (header, advance) = parseHunk(lines, i)
                    if (header != null) {
                        cur.hunks += header
                        cur.adds += header.lines.count { it.prefix == "+" }
                        cur.dels += header.lines.count { it.prefix == "-" }
                    }
                    i += advance
                    continue
                }
                // Skip --- a/path, +++ b/path, index lines etc.
            }
            i++
        }
        current?.let { out += build(it) }
        return out
    }

    private fun build(b: FileBuilder): FileDto {
        // Prefer postPath for non-delete entries, prePath for deletes. This
        // matches what the TUI sidebar shows.
        val displayPath = when (b.status) {
            "D" -> b.prePath ?: b.postPath ?: "(unknown)"
            else -> b.postPath ?: b.prePath ?: "(unknown)"
        }
        return FileDto(
            path = displayPath,
            status = b.status,
            renameFrom = b.renameFrom,
            renameTo = b.renameTo,
            adds = b.adds,
            dels = b.dels,
            hunks = b.hunks.toList(),
            blobBase = b.blobBase,
            blobHead = b.blobHead,
        )
    }

    /**
     * Parses the `index <blobBase>..<blobHead>[ <mode>]` line.
     *
     * Returns `(blobBase, blobHead)` on success, or null when the line does not
     * match the expected format.  The all-zeros SHA (`0000000` / `0000000000...`)
     * is returned as-is: the caller (Frontend) can discard it when needed.
     */
    private fun parseBlobIndex(line: String): Pair<String, String>? {
        // Strip the "index " prefix and optional trailing " <mode>".
        val rest = line.removePrefix("index ").trim()
        val dotDot = rest.indexOf("..")
        if (dotDot < 0) return null
        val base = rest.substring(0, dotDot)
        val afterDot = rest.substring(dotDot + 2)
        // The head SHA is followed by an optional space + mode; take everything
        // up to the first space (or end of string).
        val head = afterDot.substringBefore(' ')
        if (base.isEmpty() || head.isEmpty()) return null
        return base to head
    }

    /**
     * Extracts `(prePath, postPath)` from `diff --git a/<x> b/<y>`. Quoted
     * paths (paths with spaces, escapes) are out of scope; the CLI typically
     * runs without `core.quotepath=true` in repos used by sitatame so this
     * covers the PoC.
     */
    private fun parseDiffGitHeader(line: String): Pair<String?, String?> {
        val rest = line.removePrefix("diff --git ").trim()
        // Find the `b/` segment. We can't just `split(' ')` because paths
        // could contain spaces in principle; for PoC we accept the simple
        // case and fall back to null when ambiguous.
        val aIdx = rest.indexOf("a/")
        val bIdx = rest.indexOf(" b/", startIndex = aIdx)
        if (aIdx < 0 || bIdx < 0) return null to null
        val a = rest.substring(aIdx + 2, bIdx)
        val b = rest.substring(bIdx + 3)
        return a to b
    }

    /** Parse one hunk starting at [start]; returns (hunk, lines consumed). */
    private fun parseHunk(lines: List<String>, start: Int): Pair<HunkDto?, Int> {
        val header = lines[start]
        val (counts, headerTail) = splitHunkHeader(header) ?: return null to 1
        var i = start + 1
        var baseLine = counts.baseStart
        var headLine = counts.headStart
        val body = mutableListOf<DiffLineDto>()
        while (i < lines.size) {
            val l = lines[i]
            if (l.startsWith("@@") || l.startsWith("diff --git ")) break
            if (l.startsWith("\\")) {
                // "\ No newline at end of file" — skip silently.
                i++
                continue
            }
            if (l.isEmpty() && i == lines.size - 1) {
                // Trailing empty element from split('\n'); ignore.
                i++
                continue
            }
            val prefix = if (l.isEmpty()) " " else l.substring(0, 1)
            val text = if (l.isEmpty()) "" else l.substring(1)
            when (prefix) {
                "+" -> {
                    body += DiffLineDto(baseLine = null, headLine = headLine, prefix = "+", text = text)
                    headLine++
                }
                "-" -> {
                    body += DiffLineDto(baseLine = baseLine, headLine = null, prefix = "-", text = text)
                    baseLine++
                }
                " " -> {
                    body += DiffLineDto(baseLine = baseLine, headLine = headLine, prefix = " ", text = text)
                    baseLine++
                    headLine++
                }
                else -> {
                    // Anything else (e.g. start of next "diff --git") would
                    // have been caught above; defensive break.
                    break
                }
            }
            i++
        }
        val hunk = HunkDto(
            baseStart = counts.baseStart,
            baseLines = counts.baseLines,
            headStart = counts.headStart,
            headLines = counts.headLines,
            header = headerTail,
            lines = body.toList(),
        )
        return hunk to (i - start)
    }

    private data class HunkCounts(
        val baseStart: Int,
        val baseLines: Int,
        val headStart: Int,
        val headLines: Int,
    )

    /**
     * Parse `@@ -<bs>,<bn> +<hs>,<hn> @@ <header-tail>` into (counts,
     * header-tail). When `,<n>` is omitted git treats it as 1, matching the
     * unified-diff spec.
     */
    private fun splitHunkHeader(line: String): Pair<HunkCounts, String>? {
        val secondAtAt = line.indexOf("@@", startIndex = 2)
        if (secondAtAt < 0) return null
        val meta = line.substring(2, secondAtAt).trim()
        val tail = line.substring(secondAtAt + 2).removePrefix(" ")
        val parts = meta.split(' ')
        if (parts.size < 2) return null
        val base = parsePair(parts[0]) ?: return null
        val head = parsePair(parts[1]) ?: return null
        return HunkCounts(base.first, base.second, head.first, head.second) to tail
    }

    private fun parsePair(token: String): Pair<Int, Int>? {
        val sign = token.firstOrNull() ?: return null
        if (sign != '-' && sign != '+') return null
        val body = token.substring(1)
        val (a, b) = if (',' in body) {
            val s = body.split(',', limit = 2)
            (s[0].toIntOrNull() ?: return null) to (s[1].toIntOrNull() ?: return null)
        } else {
            (body.toIntOrNull() ?: return null) to 1
        }
        return a to b
    }
}
