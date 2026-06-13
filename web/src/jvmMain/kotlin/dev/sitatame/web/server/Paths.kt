package dev.sitatame.web.server

import java.nio.file.Path
import java.nio.file.Paths as NioPaths

/**
 * Resolves the on-disk locations for review storage. Kotlin port of
 * `internal/review/paths.go` — keep the resolution order in sync with the Go
 * side or the Web UI will silently look at different directories than the
 * CLI / TUI.
 *
 *  1. $SITATAME_HOME (trimmed; empty after trim is treated as unset)
 *  2. <user home>/.sitatame
 *  3. <tmp>/sitatame  (best-effort fallback; stderr warning)
 *
 * The Web backend only reads from this tree, so we skip the symlink-canonical
 * step for $SITATAME_HOME and the tilde expansion that production code on the
 * Go side does — those edge cases are rare for the PoC and can be ported in a
 * follow-up if a user hits them. Repo root canonicalisation still happens
 * because that affects ProjectSlug stability across worktrees.
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
    fun reviewsRoot(): Path = root().resolve("reviews")
    fun reviewsDir(): Path = reviewsRoot().resolve(branchSlug)

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

        private fun resolveOutputRoot(envLookup: (String) -> String?, homeDir: Path?): Path {
            val env = envLookup(ENV_OUTPUT_ROOT)?.trim().orEmpty()
            if (env.isNotEmpty()) {
                return NioPaths.get(env)
            }
            if (homeDir != null) {
                return homeDir.resolve(".sitatame")
            }
            val tmp = System.getProperty("java.io.tmpdir") ?: "/tmp"
            System.err.println("sitatame: could not resolve user home; falling back to $tmp/sitatame")
            return NioPaths.get(tmp, "sitatame")
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
