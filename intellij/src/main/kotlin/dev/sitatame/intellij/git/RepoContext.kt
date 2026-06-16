package dev.sitatame.intellij.git

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VirtualFile
import dev.sitatame.intellij.settings.SitatameSettings
import git4idea.repo.GitRepository
import git4idea.repo.GitRepositoryManager
import java.io.File

/**
 * Resolves the (repo root, branch, base ref) triple for any file inside the
 * project.
 *
 * Phase 1 keeps this tiny on purpose: the action layer only needs an absolute
 * repo root (for [ProjectSlug] derivation) and a branch name (for
 * [BranchSlug]). Multi-repo projects (a monorepo with several `.git` dirs)
 * pick the repository that owns the file; Git4Idea's `GitRepositoryManager`
 * already does that lookup.
 *
 * Base ref resolution priority (added in issue #115):
 *  1. Explicit override in [SitatameSettings.baseRef] (non-blank)
 *  2. `remote.origin.head` from the repo's git config (auto-detect)
 *  3. Fallback literal `"origin/main"`
 */
object RepoContext {

    /**
     * Repo root + branch + base ref as the storage / diff layer needs them.
     * [repoRoot] and [branch] may be empty only in degenerate cases.
     */
    data class Info(
        val repoRoot: String,
        val branch: String,
        /** Resolved base ref. Never blank; falls back to "origin/main". */
        val baseRef: String = "origin/main",
    ) {
        fun isComplete(): Boolean = repoRoot.isNotEmpty()
    }

    /**
     * Resolve repo info for the file being edited. Returns null when the
     * file is not inside any registered Git repository (e.g. a non-VCS
     * project, or before Git4Idea has finished scanning).
     */
    fun forFile(project: Project, file: VirtualFile?): Info? {
        if (file == null) return null
        val repo = GitRepositoryManager.getInstance(project).getRepositoryForFile(file)
            ?: return null
        return toInfo(repo)
    }

    /**
     * Resolve repo info for the *project* — used by the tool window which
     * doesn't have a single file context. Picks the first repository.
     */
    fun forProject(project: Project): Info? {
        val manager = GitRepositoryManager.getInstance(project)
        val repo = manager.repositories.firstOrNull() ?: return null
        return toInfo(repo)
    }

    /**
     * Resolve the effective base ref for [repoRoot]:
     *  1. Explicit setting from [SitatameSettings] (non-blank)
     *  2. `remote.origin.head` in `.git/config` (auto-detect via git4idea's
     *     in-memory config; falls back to reading `.git/config` directly)
     *  3. Literal `"origin/main"`
     *
     * Exposed as `internal` so [RepoContextBaseRefTest] can call it directly
     * without spinning up the IntelliJ platform.
     */
    internal fun resolveBaseRef(repoRoot: String, settingsBaseRef: String): String {
        // Priority 1: explicit override
        val explicit = settingsBaseRef.trim()
        if (explicit.isNotBlank()) return explicit

        // Priority 2: remote.origin.head from git config
        val fromConfig = readOriginHeadFromGitConfig(repoRoot)
        if (!fromConfig.isNullOrBlank()) return fromConfig!!

        // Priority 3: fallback
        return "origin/main"
    }

    /**
     * Read `remote.origin.head` from `.git/config` of [repoRoot].
     *
     * Looks for a HEAD entry under the `[remote "origin"]` section, e.g. the
     * line written by `git remote set-head origin -a`. Returns the value with
     * the `refs/remotes/` prefix stripped (so "refs/remotes/origin/main"
     * becomes "origin/main").
     *
     * Returns null on any parse or IO failure; caller falls back to "origin/main".
     * Exposed as `internal` for testing without the IntelliJ platform.
     */
    internal fun readOriginHeadFromGitConfig(repoRoot: String): String? {
        return try {
            readOriginHeadFromGitConfigInternal(repoRoot)
        } catch (_: Exception) {
            null
        }
    }

    private fun readOriginHeadFromGitConfigInternal(repoRoot: String): String? {
        // Resolve the actual .git directory: could be a worktree pointer file.
        val gitFile = File(repoRoot, ".git")
        val configFile: File = when {
            gitFile.isDirectory -> File(gitFile, "config")
            gitFile.isFile -> {
                // Worktree or submodule: "gitdir: <path>" pointer
                val pointer = gitFile.readText().trim()
                val gitdirPrefix = "gitdir:"
                if (!pointer.startsWith(gitdirPrefix)) return null
                val linkedGit = File(pointer.removePrefix(gitdirPrefix).trim())
                // For a worktree the commondir is two levels up from the
                // linked .git/worktrees/<name> directory.
                val worktreesDir = linkedGit.parentFile ?: return null
                val commonGit = worktreesDir.parentFile ?: return null
                File(commonGit, "config")
            }
            else -> return null
        }
        if (!configFile.isFile) return null

        // Parse config manually: look for [remote "origin"] section, then
        // a `head =` or `HEAD =` key within it.
        var inOriginRemote = false
        for (raw in configFile.readLines()) {
            val line = raw.trim()
            when {
                line.startsWith("[") -> {
                    // e.g. [remote "origin"]
                    inOriginRemote = line.equals("[remote \"origin\"]", ignoreCase = true)
                }
                inOriginRemote && line.startsWith("head", ignoreCase = true) -> {
                    // head = refs/remotes/origin/main  (or similar)
                    val value = line.substringAfter("=", "").trim()
                    if (value.isBlank()) continue
                    // refs/remotes/origin/<branch> → origin/<branch>
                    val refsRemotesPrefix = "refs/remotes/"
                    return if (value.startsWith(refsRemotesPrefix)) {
                        value.removePrefix(refsRemotesPrefix)
                    } else {
                        value
                    }
                }
            }
        }
        return null
    }

    /**
     * Returns null when the repository is in a transient state where neither a
     * branch name nor a commit SHA is available (e.g. mid-rebase or mid-reset).
     * In that state any slug would be `"branch__da39a3ee"` (SHA-1 of the empty
     * string), causing silent comment mixing across unrelated states.
     *
     * Callers already guard on a null return via `?: return` / `?: run { … }`,
     * so refusing to produce an Info here is the correct signal. Callers that
     * wish to surface a UX message should check for the transient case by
     * calling [hasNoResolvableRef] before [forFile] / [forProject].
     *
     * Branch presence / detached-HEAD with a known SHA are both fine: the SHA
     * is a valid, deterministic slug input and there is no collision risk.
     */
    private fun toInfo(repo: GitRepository): Info? {
        val root = repo.root.path
        val branch = resolveBranch(repo.currentBranchName, repo.currentRevision) ?: return null
        val settingsBaseRef = runCatching {
            ApplicationManager.getApplication()
                .getService(SitatameSettings::class.java)?.state?.baseRef ?: ""
        }.getOrDefault("")
        val baseRef = resolveBaseRef(root, settingsBaseRef)
        return Info(repoRoot = root, branch = branch, baseRef = baseRef)
    }

    /**
     * Pure function that resolves the branch slug input from raw Git4Idea state.
     * Extracted as `internal` so unit tests can exercise the decision table
     * without spinning up the IntelliJ test platform or mocking [GitRepository].
     *
     * Decision table:
     * | branchName | revision | result                              |
     * |------------|----------|-------------------------------------|
     * | non-null   | any      | branchName (normal HEAD)            |
     * | null       | non-null | "detached/<revision[:12]>"          |
     * | null       | null     | null (transient — refuse)           |
     *
     * The "detached/<sha12>" form matches the TUI's normalisation in
     * `cmd/root.go` (`branch = "detached/" + headSHA[:12]`), ensuring
     * IntelliJ and TUI write to the same `~/.sitatame/<project>/<branch>/`
     * directory for the same detached HEAD commit.
     */
    internal fun resolveBranch(branchName: String?, revision: String?): String? {
        if (branchName == null && revision == null) return null
        return branchName ?: "detached/${revision!!.take(12)}"
    }

    /**
     * Returns true when both [GitRepository.currentBranchName] and
     * [GitRepository.currentRevision] are null — i.e. the repository has no
     * resolvable ref at all.
     *
     * Note: this does **not** detect merge / cherry-pick / rebase / bisect
     * in-progress states; those operations still have a valid revision.
     * Use this to show a user-facing notification rather than silently doing
     * nothing.
     */
    fun hasNoResolvableRef(project: Project): Boolean {
        val manager = GitRepositoryManager.getInstance(project)
        val repo = manager.repositories.firstOrNull() ?: return false
        return repo.currentBranchName == null && repo.currentRevision == null
    }
}
