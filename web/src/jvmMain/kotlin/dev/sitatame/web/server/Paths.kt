package dev.sitatame.web.server

import java.nio.file.Path
import java.nio.file.Paths as NioPaths

/**
 * Resolves the on-disk locations for review storage. Kotlin port of
 * `internal/review/paths.go` — keep the resolution order in sync with the Go
 * side or the Web UI will silently look at different directories than the
 * CLI / TUI.
 *
 *  1. $SITATAME_HOME (trimmed; empty after trim is treated as unset;
 *     leading `~/` expanded; relative paths absolutised with a stderr warning)
 *  2. <user home>/.sitatame
 *  3. <tmp>/sitatame  (best-effort fallback; stderr warning)
 *
 * Repo root canonicalisation also happens here because that affects
 * ProjectSlug stability across worktrees.
 */
data class SitatamePaths(
    val outputRoot: Path,
    val repoRoot: Path,
    val projectSlug: String,
    val branch: String,
    val branchSlug: String,
) {
    /** <outputRoot>/<projectSlug> */
    fun root(): Path = outputRoot.resolve(projectSlug)

    /** <outputRoot>/<projectSlug>/<branchSlug>/  — all review artifacts for this branch */
    fun branchDir(): Path = root().resolve(branchSlug)

    /** <outputRoot>/<projectSlug>/<branchSlug>/review.md */
    fun reviewFile(): Path = branchDir().resolve("review.md")

    /** <outputRoot>/<projectSlug>/<branchSlug>/review.md.bak */
    fun bakFile(): Path = branchDir().resolve("review.md.bak")

    companion object {
        const val ENV_OUTPUT_ROOT = "SITATAME_HOME"

        /**
         * Resolve paths for the current repo + branch using the same env /
         * home / tmp resolution order as the Go side.
         *
         * [envLookup] is injectable so tests can stub SITATAME_HOME without
         * touching the real process environment.
         */
        fun resolve(
            repoRoot: Path,
            branch: String,
            envLookup: (String) -> String? = System::getenv,
            homeDir: Path? = System.getProperty("user.home")?.let { NioPaths.get(it) },
        ): SitatamePaths {
            val outputRoot = resolveOutputRoot(envLookup, homeDir)
            val canonicalRepo = canonicaliseRepoRoot(repoRoot)
            return SitatamePaths(
                outputRoot = outputRoot,
                repoRoot = canonicalRepo,
                projectSlug = Slug.projectSlug(canonicalRepo.toString()),
                branch = branch,
                branchSlug = Slug.branchSlug(branch),
            )
        }

        /**
         * Kotlin port of `resolveOutputRoot` + `normaliseEnvOutputRoot` from
         * `internal/review/paths.go`. The validation pipeline must stay in
         * sync with Go:
         *
         *   - leading/trailing whitespace trimmed; all-whitespace is unset
         *     (so a stray `export SITATAME_HOME=" "` does not silently land
         *     everything under "  /<project-slug>/...").
         *   - a leading `~/` or bare `~` is expanded via [homeDir] so users
         *     can write `SITATAME_HOME=~/work` without relying on shell
         *     expansion.
         *   - relative paths are absolutised, with a one-line stderr warning
         *     so callers know which directory was actually picked.
         *
         * Exposed as `internal` for [resolveOutputRoot] tests.
         */
        internal fun resolveOutputRoot(envLookup: (String) -> String?, homeDir: Path?): Path {
            val raw = envLookup(ENV_OUTPUT_ROOT)?.trim().orEmpty()
            if (raw.isNotEmpty()) {
                return normaliseEnvOutputRoot(raw, homeDir)
            }
            if (homeDir != null) {
                return homeDir.resolve(".sitatame")
            }
            val tmp = System.getProperty("java.io.tmpdir") ?: "/tmp"
            val fallback = NioPaths.get(tmp, "sitatame")
            System.err.println("sitatame: could not resolve user home; falling back to $fallback")
            return fallback
        }

        private fun normaliseEnvOutputRoot(value: String, homeDir: Path?): Path {
            var v = value
            if (v == "~" || v.startsWith("~/")) {
                val home = homeDir?.toString()
                if (!home.isNullOrEmpty()) {
                    v = if (v == "~") home else home + v.substring(1)
                }
                // If homeDir is unknown, fall through with the literal value;
                // matches Go where UserHomeDir() failure leaves "~/..."
                // untouched and the next step (Abs) absolutises it.
            }
            val path = NioPaths.get(v)
            if (path.isAbsolute) {
                return path
            }
            // Relative: absolutise and warn so callers see which directory we
            // picked. Best-effort: if toAbsolutePath() throws or returns the
            // input unchanged we still emit the warning but use the literal
            // value, matching Go's filepath.Abs failure mode.
            val abs = try {
                path.toAbsolutePath().normalize()
            } catch (_: Exception) {
                path
            }
            System.err.println("sitatame: $ENV_OUTPUT_ROOT was relative; using $abs")
            return abs
        }

        private fun canonicaliseRepoRoot(repoRoot: Path): Path {
            // Best-effort: failures (broken symlink, non-existent path used in
            // tests) fall through to the input unchanged, matching Go semantics.
            return try {
                repoRoot.toRealPath()
            } catch (_: Exception) {
                try {
                    repoRoot.toAbsolutePath().normalize()
                } catch (_: Exception) {
                    repoRoot
                }
            }
        }
    }
}
