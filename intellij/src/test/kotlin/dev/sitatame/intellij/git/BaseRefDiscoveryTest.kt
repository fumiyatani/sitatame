package dev.sitatame.intellij.git

import dev.sitatame.intellij.git.BaseRefDiscovery.Entry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for [BaseRefDiscovery.assemble] — the pure grouping / ordering of
 * raw git branch output into selector entries. No IntelliJ Platform or real git
 * required.
 */
class BaseRefDiscoveryTest {

    private fun refs(entries: List<Entry>): List<String> =
        entries.filterIsInstance<Entry.Branch>().map { it.ref }

    private fun headers(entries: List<Entry>): List<String> =
        entries.filterIsInstance<Entry.Header>().map { it.title }

    // -----------------------------------------------------------------------
    // Grouping
    // -----------------------------------------------------------------------

    @Test
    fun groups_remotesAndLocalsUnderHeaders() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/main", "origin/feature-a"),
            localRefs = listOf("main", "feature-b"),
            currentBranch = "feature-current",
        )

        assertEquals(listOf("Remote Branches", "Local Branches"), headers(entries))

        // Remote group precedes local group.
        val remoteHeaderIdx = entries.indexOfFirst { it is Entry.Header && it.title == "Remote Branches" }
        val localHeaderIdx = entries.indexOfFirst { it is Entry.Header && it.title == "Local Branches" }
        assertTrue("Remote header before local header", remoteHeaderIdx < localHeaderIdx)

        val originMainEntry = entries.filterIsInstance<Entry.Branch>().first { it.ref == "origin/main" }
        val localMainEntry = entries.filterIsInstance<Entry.Branch>().first { it.ref == "main" }
        assertTrue("origin/main is remote", originMainEntry.remote)
        assertFalse("main is local", localMainEntry.remote)
    }

    @Test
    fun emptyGroup_producesNoHeader() {
        val onlyRemotes = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/main"),
            localRefs = emptyList(),
            currentBranch = "x",
        )
        assertEquals(listOf("Remote Branches"), headers(onlyRemotes))

        val onlyLocals = BaseRefDiscovery.assemble(
            remoteRefs = emptyList(),
            localRefs = listOf("main"),
            currentBranch = "x",
        )
        assertEquals(listOf("Local Branches"), headers(onlyLocals))

        val nothing = BaseRefDiscovery.assemble(emptyList(), emptyList(), "x")
        assertTrue("No branches → no entries at all", nothing.isEmpty())
    }

    // -----------------------------------------------------------------------
    // Filtering
    // -----------------------------------------------------------------------

    @Test
    fun remoteHeadPointer_isDropped() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/HEAD", "origin/main"),
            localRefs = emptyList(),
            currentBranch = "x",
        )
        assertFalse("origin/HEAD pointer must be excluded", refs(entries).contains("origin/HEAD"))
        assertTrue("origin/main kept", refs(entries).contains("origin/main"))
    }

    @Test
    fun currentBranch_excludedFromLocals() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = emptyList(),
            localRefs = listOf("main", "feature-current"),
            currentBranch = "feature-current",
        )
        assertFalse("current branch is never a useful base", refs(entries).contains("feature-current"))
        assertTrue("other locals kept", refs(entries).contains("main"))
    }

    @Test
    fun currentBranch_stillListedAsRemote() {
        // The current branch's pushed counterpart (origin/feature-current) is a
        // valid base even though the local branch itself is excluded.
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/feature-current"),
            localRefs = listOf("feature-current"),
            currentBranch = "feature-current",
        )
        assertTrue(refs(entries).contains("origin/feature-current"))
        assertFalse(refs(entries).contains("feature-current"))
    }

    @Test
    fun blankAndDuplicateRefs_areCleaned() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/main", " origin/main ", "", "  "),
            localRefs = listOf("dev", "dev"),
            currentBranch = "x",
        )
        assertEquals("origin/main deduped to one", 1, refs(entries).count { it == "origin/main" })
        assertEquals("dev deduped to one", 1, refs(entries).count { it == "dev" })
    }

    // -----------------------------------------------------------------------
    // Ordering: main/master/develop first within each group
    // -----------------------------------------------------------------------

    @Test
    fun priorityBranches_comeFirstThenAlphabetical() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/zebra", "origin/develop", "origin/apple", "origin/main"),
            localRefs = emptyList(),
            currentBranch = "x",
        )
        assertEquals(
            listOf("origin/main", "origin/develop", "origin/apple", "origin/zebra"),
            refs(entries),
        )
    }

    @Test
    fun localPriority_ordering() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = emptyList(),
            localRefs = listOf("topic-z", "master", "topic-a"),
            currentBranch = "current",
        )
        assertEquals(listOf("master", "topic-a", "topic-z"), refs(entries))
    }

    // -----------------------------------------------------------------------
    // resolveSelection: which entry the combo selects for a given base ref
    // -----------------------------------------------------------------------

    @Test
    fun resolveSelection_exactMatch_selectsItAndKeepsEntries() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/main", "origin/dev"),
            localRefs = emptyList(),
            currentBranch = "x",
        )
        val sel = BaseRefDiscovery.resolveSelection(entries, "origin/dev")
        assertEquals("origin/dev", sel.selected?.ref)
        assertEquals("entries are not modified on a match", entries, sel.entries)
    }

    @Test
    fun resolveSelection_fuzzyMatchesRemoteCounterpart() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/main"),
            localRefs = emptyList(),
            currentBranch = "x",
        )
        // Configured base "main" should resolve to the remote "origin/main".
        val sel = BaseRefDiscovery.resolveSelection(entries, "main")
        assertEquals("origin/main", sel.selected?.ref)
    }

    @Test
    fun resolveSelection_doesNotFuzzyMatchAcrossUnrelatedSegments() {
        // Only a local "main" exists; a configured "feature/main" must NOT match
        // it (that would diff against the wrong branch). It should synthesize.
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = emptyList(),
            localRefs = listOf("main"),
            currentBranch = "x",
        )
        val sel = BaseRefDiscovery.resolveSelection(entries, "feature/main")
        assertEquals("configured ref is selected, not local main", "feature/main", sel.selected?.ref)
        assertEquals(
            "configured ref is synthesized at the front, not matched to local main",
            "feature/main",
            (sel.entries.firstOrNull() as? Entry.Branch)?.ref,
        )
    }

    @Test
    fun resolveSelection_noMatch_synthesizesConfiguredRef() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/feature-a"),
            localRefs = listOf("topic"),
            currentBranch = "x",
        )
        val sel = BaseRefDiscovery.resolveSelection(entries, "origin/release-9")
        // The diff target must equal the configured ref, never a silent fallback
        // to an unrelated first branch.
        assertEquals("origin/release-9", sel.selected?.ref)
        assertTrue("synthetic ref is treated as remote", sel.selected?.remote == true)
        assertTrue(
            "synthetic entry is prepended so display == diff",
            sel.entries.firstOrNull() == Entry.Branch("origin/release-9", remote = true),
        )
    }

    @Test
    fun resolveSelection_emptyEntries_stillSurfacesConfiguredRef() {
        val sel = BaseRefDiscovery.resolveSelection(emptyList(), "origin/main")
        assertEquals("origin/main", sel.selected?.ref)
        assertEquals(listOf<Entry>(Entry.Branch("origin/main", remote = true)), sel.entries)
    }

    @Test
    fun resolveSelection_blankRef_fallsBackToFirstBranch() {
        val entries = BaseRefDiscovery.assemble(
            remoteRefs = listOf("origin/main"),
            localRefs = emptyList(),
            currentBranch = "x",
        )
        val sel = BaseRefDiscovery.resolveSelection(entries, "")
        assertEquals("origin/main", sel.selected?.ref)
        assertEquals("no synthetic entry for a blank ref", entries, sel.entries)
    }
}
