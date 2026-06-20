package dev.sitatame.intellij.git

/**
 * Discovers the branches that can be used as the *base* of the
 * `git diff <base>..HEAD` comparison driving the 3-pane tool window.
 *
 * The base ref selector lists the repository's real remote and local
 * branches, grouped under "Remote Branches" / "Local Branches" headers, so the
 * user can pick what their current branch (HEAD) is compared against — the same
 * mental model as GitHub's "base ← compare" pull-request view.
 *
 * Intentionally avoids Git4Idea APIs so [assemble] (the ordering / grouping
 * logic) can be exercised in plain JUnit tests without the IntelliJ Platform.
 * Only [listEntries] shells out to git.
 */
object BaseRefDiscovery {

    /** Well-known base branches surfaced first within each group. */
    private val PRIORITY = listOf("main", "master", "develop")

    /**
     * One row in the base ref selector. Either a non-selectable group [Header]
     * or a selectable [Branch].
     */
    sealed interface Entry {
        /** A non-selectable section header such as "Remote Branches". */
        data class Header(val title: String) : Entry

        /** A selectable branch ref. [remote] drives the group and icon. */
        data class Branch(val ref: String, val remote: Boolean) : Entry
    }

    /**
     * Shell out to git for the repository's branches and return the grouped,
     * ordered selector entries. Returns an empty list on any git failure; the
     * caller's [resolveSelection] then still surfaces the configured base ref so
     * the diff target never silently changes.
     *
     * MUST be called off the EDT — it spawns git subprocesses.
     */
    fun listEntries(repoRoot: String, currentBranch: String): List<Entry> {
        val remotes = GitProcess.run(repoRoot, "git", "for-each-ref", "--format=%(refname:short)", "refs/remotes")
        val locals = GitProcess.run(repoRoot, "git", "for-each-ref", "--format=%(refname:short)", "refs/heads")
        return assemble(remotes, locals, currentBranch)
    }

    /** The combo entries to display and which branch (if any) to select. */
    data class Selection(val entries: List<Entry>, val selected: Entry.Branch?)

    /**
     * Decide which entry to select for a given [effectiveRef] (the resolved base
     * the diff will run against). Pure so it can be unit-tested without Swing.
     *
     * - Exact ref match wins.
     * - Otherwise a remote/local counterpart is accepted (e.g. `origin/main`
     *   matches a configured `main`, and vice versa).
     * - If nothing matches and [effectiveRef] is non-blank, a synthetic branch
     *   entry for it is prepended and selected, so the displayed selection always
     *   equals the ref the diff uses — never a silent fallback to an unrelated
     *   first branch.
     * - Only when [effectiveRef] is blank do we fall back to the first branch.
     */
    fun resolveSelection(entries: List<Entry>, effectiveRef: String): Selection {
        val branches = entries.filterIsInstance<Entry.Branch>()
        // Remote names actually present (e.g. "origin"), so the remote/local
        // counterpart match only strips a real remote prefix — never an
        // arbitrary first path segment (which would let "feature/main" match a
        // local "main"). Assumes single-segment remote names (the norm); a
        // slash-containing remote like "foo/bar" would be under-stripped, at
        // worst forcing a synthetic entry the user can still pick from the list.
        val knownRemotes = branches.asSequence()
            .filter { it.remote }
            .map { it.ref.substringBefore('/') }
            .filter { it.isNotEmpty() }
            .toSet()

        val match = branches.firstOrNull { it.ref == effectiveRef }
            ?: branches.firstOrNull { b ->
                // configured a bare name → a remote-qualified branch ("main" → "origin/main")
                (b.remote && knownRemotes.any { r -> b.ref == "$r/$effectiveRef" }) ||
                    // configured a remote ref → its local counterpart ("origin/main" → "main")
                    (!b.remote && knownRemotes.any { r -> effectiveRef == "$r/${b.ref}" })
            }
        if (match != null) return Selection(entries, match)

        if (effectiveRef.isNotBlank()) {
            // Best-effort remote flag (cosmetic — drives the icon only). Precise
            // when remotes are known; falls back to structure when discovery
            // returned nothing.
            val firstSegment = effectiveRef.substringBefore('/')
            val remote = knownRemotes.contains(firstSegment) ||
                (knownRemotes.isEmpty() && effectiveRef.contains("/"))
            val synthetic = Entry.Branch(effectiveRef, remote = remote)
            return Selection(listOf<Entry>(synthetic) + entries, synthetic)
        }
        return Selection(entries, branches.firstOrNull())
    }

    /**
     * Pure grouping / ordering of raw git output into selector entries.
     *
     * - Remote branches: the per-remote symbolic `HEAD` pointers (e.g.
     *   `origin/HEAD`) are dropped — they are not real branches and would shadow
     *   the branch they point at.
     * - Local branches: [currentBranch] is excluded; diffing a branch against
     *   itself yields nothing and is never a useful base.
     * - Within each group, [PRIORITY] branches (main/master/develop) come first,
     *   then the rest alphabetically.
     * - A group whose branch list is empty contributes no header.
     */
    internal fun assemble(
        remoteRefs: List<String>,
        localRefs: List<String>,
        currentBranch: String,
    ): List<Entry> {
        val remotes = ordered(
            remoteRefs
                .map { it.trim() }
                .filter { it.isNotEmpty() && !it.endsWith("/HEAD") }
                .distinct()
        )
        val locals = ordered(
            localRefs
                .map { it.trim() }
                .filter { it.isNotEmpty() && it != currentBranch.trim() }
                .distinct()
        )

        val entries = mutableListOf<Entry>()
        if (remotes.isNotEmpty()) {
            entries.add(Entry.Header("Remote Branches"))
            remotes.forEach { entries.add(Entry.Branch(it, remote = true)) }
        }
        if (locals.isNotEmpty()) {
            entries.add(Entry.Header("Local Branches"))
            locals.forEach { entries.add(Entry.Branch(it, remote = false)) }
        }
        return entries
    }

    /** PRIORITY branches first (in PRIORITY order), then the rest alphabetically. */
    private fun ordered(refs: List<String>): List<String> {
        fun priorityIndex(ref: String): Int =
            PRIORITY.indexOfFirst { ref == it || ref.endsWith("/$it") }

        val (priority, rest) = refs.partition { priorityIndex(it) >= 0 }
        return priority.sortedBy { priorityIndex(it) } + rest.sorted()
    }
}
