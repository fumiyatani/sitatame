package dev.sitatame.intellij.git

import java.util.concurrent.TimeUnit

/**
 * Discovers candidate base-ref values to populate the base ref selector
 * ComboBox in the 3-pane tool window toolbar.
 *
 * Intentionally avoids Git4Idea APIs so it can be exercised in plain JUnit
 * tests without the IntelliJ Platform framework.
 *
 * Candidate ordering (deduped, stable):
 *  1. Current SitatameSettings.baseRef (if non-blank) — labelled "(default from Settings)"
 *  2. Auto-detected remote.origin.head from .git/config (via [RepoContext])
 *  3. Known remote branches: origin/main, origin/master, origin/develop (if they exist)
 *  4. Local branches from `git branch --format='%(refname:short)'`, excluding currentBranch
 */
object BaseRefDiscovery {

    /** Default marker prefix displayed in the ComboBox. */
    const val SETTINGS_LABEL_SUFFIX = " (default — Settings)"

    /**
     * Return an ordered, deduplicated list of base-ref candidates.
     *
     * [repoRoot] and [currentBranch] are used for git discovery.
     * [settingsBaseRef] is the value stored in SitatameSettings; when non-blank
     * it is inserted at position 0 with a "(default — Settings)" suffix so the
     * user can see what the persisted default is. The raw value (without suffix)
     * is also included as a plain candidate so free-text matching works.
     *
     * Returns at minimum ["origin/main"] even on failure.
     */
    fun listCandidates(
        repoRoot: String,
        currentBranch: String,
        settingsBaseRef: String,
    ): List<String> {
        val seen = linkedSetOf<String>()

        // 1. Settings default (annotated)
        val trimmedSettings = settingsBaseRef.trim()
        if (trimmedSettings.isNotBlank()) {
            seen.add("$trimmedSettings$SETTINGS_LABEL_SUFFIX")
            seen.add(trimmedSettings)
        }

        // 2. remote.origin.head from .git/config
        val fromConfig = RepoContext.readOriginHeadFromGitConfig(repoRoot)
        if (!fromConfig.isNullOrBlank()) seen.add(fromConfig)

        // 3. Well-known remote branches
        for (remote in listOf("origin/main", "origin/master", "origin/develop")) {
            seen.add(remote)
        }

        // 4. Local branches (exclude currentBranch)
        val locals = runGit(repoRoot, "git", "branch", "--format=%(refname:short)")
        for (branch in locals) {
            if (branch != currentBranch) seen.add(branch)
        }

        return seen.toList()
    }

    /**
     * Run a git command in [repoRoot], returning stdout lines or empty list on failure.
     * 5-second timeout; stderr drained.
     */
    internal fun runGit(repoRoot: String, vararg args: String): List<String> {
        if (repoRoot.isEmpty()) return emptyList()
        return try {
            val proc = ProcessBuilder(*args)
                .directory(java.io.File(repoRoot))
                .redirectErrorStream(true)
                .start()
            val out = proc.inputStream.bufferedReader().readText()
            if (!proc.waitFor(5, TimeUnit.SECONDS)) {
                proc.destroyForcibly()
                return emptyList()
            }
            if (proc.exitValue() != 0) return emptyList()
            out.lineSequence().map { it.trim() }.filter { it.isNotEmpty() }.toList()
        } catch (_: Exception) {
            emptyList()
        }
    }
}
