package dev.sitatame.intellij.git

/**
 * Resolves the list of files changed between [baseRef] and HEAD via
 * `git diff --name-only <baseRef>..HEAD`.
 *
 * Intentionally avoids Git4Idea APIs so it can be exercised in plain JUnit
 * tests without the IntelliJ Platform framework. The subprocess is run through
 * [GitProcess] so its 5-second timeout holds even if git wedges.
 *
 * Thread safety: [listChangedFiles] is safe to call from any thread. Call
 * from a background thread (Task.Backgroundable) to avoid blocking the EDT.
 */
object ChangedFilesProvider {

    /**
     * Return the list of repo-relative file paths changed between [baseRef]
     * and HEAD in [repoRoot], or an empty list on any error.
     *
     * Uses `git diff --name-only <baseRef>..HEAD` which lists only files
     * that differ between the merge-base of the two refs and HEAD, regardless
     * of any working-tree changes. This is consistent with how `sitatame diff`
     * works on the CLI side.
     *
     * A 5-second timeout guards against hanging in large repos or broken git
     * configs. stderr is drained into stdout to prevent pipe-buffer deadlocks,
     * matching the [BlobResolver] pattern.
     *
     * @param repoRoot Absolute path to the git repository root.
     * @param baseRef  The base reference to diff against (e.g. "origin/main").
     * @return Sorted list of repo-relative paths, empty on failure.
     */
    fun listChangedFiles(repoRoot: String, baseRef: String): List<String> {
        if (repoRoot.isEmpty() || baseRef.isEmpty()) return emptyList()
        return GitProcess.run(repoRoot, "git", "diff", "--name-only", "$baseRef..HEAD").sorted()
    }
}
