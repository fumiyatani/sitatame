package dev.sitatame.intellij.git

import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VirtualFile
import git4idea.repo.GitRepository
import git4idea.repo.GitRepositoryManager

/**
 * Resolves the (repo root, branch) pair for any file inside the project.
 *
 * Phase 1 keeps this tiny on purpose: the action layer only needs an absolute
 * repo root (for [ProjectSlug] derivation) and a branch name (for
 * [BranchSlug]). Multi-repo projects (a monorepo with several `.git` dirs)
 * pick the repository that owns the file; Git4Idea's `GitRepositoryManager`
 * already does that lookup.
 */
object RepoContext {

    /** Repo root + branch as the storage layer needs them. Either may be empty. */
    data class Info(val repoRoot: String, val branch: String) {
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
     * Returns null when the repository is in a transient state where neither a
     * branch name nor a commit SHA is available (e.g. mid-rebase or mid-reset).
     * In that state any slug would be `"branch__da39a3ee"` (SHA-1 of the empty
     * string), causing silent comment mixing across unrelated states.
     *
     * Callers already guard on a null return via `?: return` / `?: run { … }`,
     * so refusing to produce an Info here is the correct signal. Callers that
     * wish to surface a UX message should check for the transient case by
     * calling [isTransientState] before [forFile] / [forProject].
     *
     * Branch presence / detached-HEAD with a known SHA are both fine: the SHA
     * is a valid, deterministic slug input and there is no collision risk.
     */
    private fun toInfo(repo: GitRepository): Info? {
        val root = repo.root.path
        val branch = resolveBranch(repo.currentBranchName, repo.currentRevision) ?: return null
        return Info(repoRoot = root, branch = branch)
    }

    /**
     * Pure function that resolves the branch slug input from raw Git4Idea state.
     * Extracted as `internal` so unit tests can exercise the decision table
     * without spinning up the IntelliJ test platform or mocking [GitRepository].
     *
     * Decision table:
     * | branchName | revision | result                       |
     * |------------|----------|------------------------------|
     * | non-null   | any      | branchName (normal HEAD)     |
     * | null       | non-null | revision (detached HEAD)     |
     * | null       | null     | null (transient — refuse)    |
     */
    internal fun resolveBranch(branchName: String?, revision: String?): String? {
        if (branchName == null && revision == null) return null
        return branchName ?: revision
    }

    /**
     * Returns true when the repository is in a transient Git state (mid-rebase,
     * mid-reset, etc.) where no branch name or SHA is available.
     * Use this to show a user-facing notification rather than silently doing
     * nothing.
     */
    fun isTransientState(project: Project): Boolean {
        val manager = GitRepositoryManager.getInstance(project)
        val repo = manager.repositories.firstOrNull() ?: return false
        return repo.currentBranchName == null && repo.currentRevision == null
    }
}
