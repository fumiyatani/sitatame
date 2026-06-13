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

    private fun toInfo(repo: GitRepository): Info {
        val root = repo.root.path
        // currentBranchName is null when in detached HEAD. Use the SHA if
        // available; otherwise blank. The storage layer's BranchSlug("")
        // fallback ("branch__da39a3ee") will catch the blank case.
        val branch = repo.currentBranchName ?: repo.currentRevision ?: ""
        return Info(repoRoot = root, branch = branch)
    }
}
