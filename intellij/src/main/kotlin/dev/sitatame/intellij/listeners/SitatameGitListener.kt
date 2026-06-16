package dev.sitatame.intellij.listeners

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.Logger
import dev.sitatame.intellij.storage.REVIEW_CHANGED_TOPIC
import dev.sitatame.intellij.storage.ReviewStore
import git4idea.repo.GitRepository
import git4idea.repo.GitRepositoryChangeListener

/**
 * Listens for Git repository state changes (branch switch, fetch, reset, …)
 * and triggers a tool-window refresh whenever the **current branch** changes.
 *
 * Implementation detail:
 *   Git4Idea fires [GitRepositoryChangeListener.repositoryChanged] on a pooled
 *   thread after any repository state mutation (HEAD move, index change, …).
 *   We track the last-known branch name per repository root and fire only when
 *   it actually changes, avoiding spurious refreshes on index-only events
 *   (staging, unstaging).
 *
 * Thread safety:
 *   [lastBranch] is accessed only from [repositoryChanged], which Git4Idea
 *   always calls on the same pooled thread per repository. No additional
 *   synchronisation is needed for single-repository projects; for multi-repo
 *   projects the map is written from different pooled threads but
 *   [HashMap] reads during `get` + `put` are safe here because
 *   `repositoryChanged` is not reentrant for the same repo root. A
 *   [java.util.concurrent.ConcurrentHashMap] would be safer; see the
 *   implementation note below.
 *
 * Registration:
 *   Done in [dev.sitatame.intellij.markers.SitatameProjectActivity] via
 *   `project.messageBus.connect(disposable).subscribe(GitRepository.GIT_REPO_CHANGE, …)`.
 */
class SitatameGitListener(
    /**
     * Injected for testability; production code passes lambdas backed by the
     * application-level services.
     */
    private val invalidateCache: () -> Unit,
    private val publishChanged: (repoRoot: String, branch: String) -> Unit,
) : GitRepositoryChangeListener {

    private val log = Logger.getInstance(SitatameGitListener::class.java)

    /**
     * Tracks the last branch name seen per repository root so we can detect
     * actual branch switches rather than reacting to every index/HEAD event.
     *
     * Key: repo root path (absolute, as returned by [GitRepository.root].path).
     * Value: branch name or null (detached HEAD without a known name).
     *
     * Use [java.util.concurrent.ConcurrentHashMap] for thread-safety across
     * multi-repo projects where different repos may fire simultaneously.
     */
    private val lastBranch = java.util.concurrent.ConcurrentHashMap<String, String?>()

    override fun repositoryChanged(repository: GitRepository) {
        val root = repository.root.path
        val newBranch = resolveBranch(repository)
        val old = lastBranch.put(root, newBranch)

        if (old == newBranch) return   // no branch change — skip

        log.debug("sitatame: branch changed in $root: $old → $newBranch")
        invalidateCache()
        if (newBranch != null) {
            publishChanged(root, newBranch)
        }
    }

    companion object {

        /**
         * Resolve the effective branch name from [repository] state.
         *
         * Decision table (mirrors [dev.sitatame.intellij.git.RepoContext.resolveBranch]):
         * | branchName | revision | result                      |
         * |------------|----------|-----------------------------|
         * | non-null   | any      | branchName                  |
         * | null       | non-null | "detached/<revision[:12]>"  |
         * | null       | null     | null (transient)            |
         *
         * Extracted as a pure function so tests can exercise it without the
         * IntelliJ Platform.
         */
        fun resolveBranch(repository: GitRepository): String? =
            resolveBranchRaw(repository.currentBranchName, repository.currentRevision)

        /**
         * Pure: resolve branch from raw strings without [GitRepository].
         * Testable without the IntelliJ Platform.
         */
        fun resolveBranchRaw(branchName: String?, revision: String?): String? {
            if (branchName == null && revision == null) return null
            return branchName ?: "detached/${revision!!.take(12)}"
        }
    }
}

/**
 * Factory that creates a [SitatameGitListener] wired to the application-level
 * services. Called from [SitatameProjectActivity].
 */
internal fun buildGitListener(): SitatameGitListener {
    return SitatameGitListener(
        invalidateCache = {
            runCatching {
                ApplicationManager.getApplication()
                    .getService(ReviewStore::class.java)
                    ?.invalidate()
            }
        },
        publishChanged = { repoRoot, branch ->
            runCatching {
                ApplicationManager.getApplication().messageBus
                    .syncPublisher(REVIEW_CHANGED_TOPIC)
                    .onChanged(repoRoot, branch)
            }
        },
    )
}
