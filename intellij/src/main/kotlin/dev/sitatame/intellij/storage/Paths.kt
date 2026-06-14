package dev.sitatame.intellij.storage

import java.io.File
import java.nio.file.Path
import java.nio.file.Paths as NioPaths

/**
 * Port of `internal/review/paths.go`.
 *
 * As of issue #76 all review artifacts for a branch live under a single
 * per-branch directory:
 *   `<OutputRoot>/<ProjectSlug>/<BranchSlug>/`
 *
 * with a single canonical review file (`review.md`), an automatic backup
 * (`review.md.bak`), and rescue JSON files on encode failure.
 *
 * The OutputRoot resolution order — `SITATAME_HOME` env → user home →
 * tempdir fallback — must match the Go side exactly so the CLI, Web UI and
 * IntelliJ plugin all read and write the same files for the same project.
 */
data class SitatamePaths(
    val outputRoot: String,
    val repoRoot: String,
    val projectSlug: String,
    val branch: String,
    val slug: String,
) {
    fun root(): String = joinPath(outputRoot, projectSlug)

    /** Per-branch directory: `<OutputRoot>/<ProjectSlug>/<BranchSlug>/` */
    fun branchDir(): String = joinPath(root(), slug)

    /** Canonical review file: `<branchDir>/review.md` */
    fun reviewFile(): String = joinPath(branchDir(), "review.md")

    /** Backup review file: `<branchDir>/review.md.bak` */
    fun bakFile(): String = joinPath(branchDir(), "review.md.bak")

    /**
     * Glob pattern matching rescue JSON files:
     * `<branchDir>/review.md.rescue.*.json`
     */
    fun rescueFilePattern(): String = joinPath(branchDir(), "review.md.rescue.*.json")

    /**
     * Pre-#38 in-repo storage path (<repo>/.sitatame). Kept so the UI can warn
     * the user if old data still exists; never read or written.
     */
    fun legacyRoot(): String =
        if (repoRoot.isEmpty()) "" else joinPath(repoRoot, PathsFactory.ROOT_DIR)
}

object PathsFactory {

    const val ENV_OUTPUT_ROOT = "SITATAME_HOME"
    const val ROOT_DIR = ".sitatame"

    /**
     * Build [SitatamePaths] using the default OutputRoot resolution. Override
     * via [overrideHome] when the IntelliJ Settings panel sets a custom
     * SITATAME_HOME; matches Go's `os.Getenv(EnvOutputRoot)` semantics.
     */
    fun newPaths(repoRoot: String, branch: String, overrideHome: String? = null): SitatamePaths =
        newPathsWithRoot(resolveOutputRoot(overrideHome), repoRoot, branch)

    /** Test-friendly: provide an explicit OutputRoot. Mirrors Go's `NewPathsWithRoot`. */
    fun newPathsWithRoot(outputRoot: String, repoRoot: String, branch: String): SitatamePaths {
        val canonical = canonicaliseRepoRoot(repoRoot)
        return SitatamePaths(
            outputRoot = outputRoot,
            repoRoot = canonical,
            projectSlug = Slug.projectSlug(canonical),
            branch = branch,
            slug = Slug.branchSlug(branch),
        )
    }

    /**
     * Canonicalise the repo root: resolve symlinks, then absolutise. Errors
     * are swallowed because tests use synthetic paths like "/repo" that don't
     * exist on disk — Go does the same.
     */
    fun canonicaliseRepoRoot(repoRoot: String): String {
        if (repoRoot.isEmpty()) return repoRoot
        var canonical = repoRoot
        try {
            val real = NioPaths.get(repoRoot).toRealPath()
            canonical = real.toString()
        } catch (_: Exception) {
            // Best effort; keep input.
        }
        try {
            canonical = File(canonical).absoluteFile.path
        } catch (_: Exception) {
            // Best effort.
        }
        return canonical
    }

    /**
     * Default resolution order: env override → SITATAME_HOME env → user home
     * (`~/.sitatame`) → temp dir fallback. [overrideHome] (Settings panel)
     * takes precedence over the env so the IDE can isolate test workflows
     * without touching the user's shell.
     */
    fun resolveOutputRoot(overrideHome: String? = null): String {
        val explicit = overrideHome?.trim()?.takeIf { it.isNotEmpty() }
        if (explicit != null) return normaliseOutputRoot(explicit)

        val env = System.getenv(ENV_OUTPUT_ROOT)?.trim().orEmpty()
        if (env.isNotEmpty()) return normaliseOutputRoot(env)

        val home = System.getProperty("user.home").orEmpty()
        if (home.isNotEmpty()) return joinPath(home, ROOT_DIR)

        val fallback = joinPath(System.getProperty("java.io.tmpdir") ?: "/tmp", "sitatame")
        // Match the Go one-line warning. Use System.err so it appears in idea.log.
        System.err.println("sitatame: could not resolve user home; falling back to $fallback")
        return fallback
    }

    private fun normaliseOutputRoot(v: String): String {
        var resolved = v
        if (resolved == "~" || resolved.startsWith("~/")) {
            val home = System.getProperty("user.home").orEmpty()
            if (home.isNotEmpty()) {
                resolved = if (resolved == "~") home else joinPath(home, resolved.substring(2))
            }
        }
        val f = File(resolved)
        if (f.isAbsolute) return f.path
        return try {
            val abs = f.absoluteFile.path
            System.err.println("sitatame: $ENV_OUTPUT_ROOT was relative; using $abs")
            abs
        } catch (_: Exception) {
            resolved
        }
    }
}

/** Mirror of Go's `filepath.Join` for the two-arg case we actually use. */
internal fun joinPath(a: String, b: String): String {
    if (a.isEmpty()) return b
    if (b.isEmpty()) return a
    val sep = File.separatorChar
    val needsSep = !a.endsWith(sep) && !b.startsWith(sep)
    return if (needsSep) a + sep + b else if (a.endsWith(sep) && b.startsWith(sep)) a + b.substring(1) else a + b
}

internal fun toPath(s: String): Path = NioPaths.get(s)
