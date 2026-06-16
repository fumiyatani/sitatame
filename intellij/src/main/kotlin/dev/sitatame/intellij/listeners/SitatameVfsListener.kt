package dev.sitatame.intellij.listeners

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.vfs.AsyncFileListener
import com.intellij.openapi.vfs.newvfs.events.VFileEvent
import dev.sitatame.intellij.storage.REVIEW_CHANGED_TOPIC
import dev.sitatame.intellij.storage.ReviewStore
import dev.sitatame.intellij.storage.PathsFactory

/**
 * Detects external edits to any `<OutputRoot>/<project-slug>/<branch-slug>/review.md`
 * and propagates them into the plugin as if the action layer had made the change.
 *
 * The canonical path pattern for a review file is:
 *   `<something>/.sitatame/<project-slug>/<branch-slug>/review.md`
 *
 * We do **not** need to know the exact slug values here — the VFS event path
 * is enough to decide relevance. The actual repoRoot/branch is decoded from the
 * path segments so the correct topic payload can be published.
 *
 * Threading:
 *   `prepareChange` is called on a pooled thread. `afterVfsChange` is also
 *   called on a pooled thread (see AsyncFileListener javadoc). Both are safe
 *   because ReviewStore.invalidate() is thread-safe (ConcurrentHashMap.clear)
 *   and ApplicationManager.messageBus.syncPublisher is thread-safe.
 *
 * Registration:
 *   Done in [dev.sitatame.intellij.markers.SitatameProjectActivity] via
 *   VirtualFileManager.getInstance().addAsyncFileListener, bound to the
 *   project's coroutine-scope disposable so it is removed when the project
 *   closes.
 */
class SitatameVfsListener(
    /**
     * Injected for testability. Production code passes the application-level
     * ReviewStore singleton; tests pass a lightweight fake.
     */
    private val invalidateCache: () -> Unit,
    private val publishChanged: (repoRoot: String, branch: String) -> Unit,
) : AsyncFileListener {

    override fun prepareChange(events: List<VFileEvent>): AsyncFileListener.ChangeApplier? {
        val relevant = events.filter { isRelevant(it.path) }
        if (relevant.isEmpty()) return null

        return object : AsyncFileListener.ChangeApplier {
            override fun afterVfsChange() {
                invalidateCache()
                for (event in relevant) {
                    val (repoRoot, branch) = extractContext(event.path) ?: continue
                    publishChanged(repoRoot, branch)
                }
            }
        }
    }

    companion object {

        /**
         * Returns true when [path] looks like a sitatame review.md file, i.e.:
         *   - contains `/.sitatame/` (segment boundary)
         *   - ends with `/review.md`
         *
         * This is a pure path-string predicate so it can be tested without the
         * IntelliJ Platform.
         */
        fun isRelevant(path: String): Boolean {
            if (!path.endsWith("/review.md")) return false
            // Accept both Unix and Windows separators.
            return path.contains("/.sitatame/") || path.contains("\\.sitatame\\")
        }

        /**
         * Extract a (repoRoot, branchSlug) pair from a review.md absolute path.
         *
         * Expected layout:
         *   `<outputRoot>/<projectSlug>/<branchSlug>/review.md`
         *
         * where `<outputRoot>` typically ends with `/.sitatame` (the default
         * SITATAME_HOME).  We only need the branchSlug to publish the topic;
         * for the repoRoot we pass the projectSlug because the tool window
         * matches on repoRoot+branch to filter events.
         *
         * Since we publish to REVIEW_CHANGED_TOPIC which the tool window
         * already uses to filter by (repoRoot, branch), we reconstruct:
         *   - `repoRoot` = the outputRoot (everything up to and including `.sitatame`)
         *   - `branch`   = the branch slug (third-from-last segment)
         *
         * This lets the tool window refresh correctly when it has the same
         * context, without us having to reverse-slug or hit the filesystem.
         *
         * Returns null if the path does not have enough segments.
         */
        fun extractContext(path: String): Pair<String, String>? {
            // Normalise to forward slashes for uniform parsing.
            val normalised = path.replace('\\', '/')
            // Must end with /<projectSlug>/<branchSlug>/review.md
            // Strip the trailing "/review.md"
            val withoutFile = normalised.removeSuffix("/review.md")
            val lastSlash = withoutFile.lastIndexOf('/')
            if (lastSlash < 0) return null
            val branchSlug = withoutFile.substring(lastSlash + 1)
            val withoutBranch = withoutFile.substring(0, lastSlash)
            val prevSlash = withoutBranch.lastIndexOf('/')
            if (prevSlash < 0) return null
            // projectSlug is withoutBranch[prevSlash+1..]  (not used directly)
            val outputRoot = withoutBranch.substring(0, prevSlash)
            // We use the outputRoot as a stand-in for repoRoot so the tool window
            // can match. The tool window calls RepoContext.forProject() independently
            // to get the real repoRoot on the next refresh cycle anyway.
            return Pair(outputRoot, branchSlug)
        }
    }
}

/**
 * Factory that creates a [SitatameVfsListener] wired to the application-level
 * services. Called from [SitatameProjectActivity].
 */
internal fun buildVfsListener(): SitatameVfsListener {
    return SitatameVfsListener(
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
