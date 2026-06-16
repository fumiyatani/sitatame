package dev.sitatame.intellij.git

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File
import java.nio.file.Files

/**
 * Unit tests for [BaseRefDiscovery.listCandidates].
 *
 * Pure function tests — no IntelliJ Platform required.
 * Uses a temp directory to write fake .git/config for remote.origin.head detection.
 */
class BaseRefDiscoveryTest {

    private lateinit var tmpRepo: File

    @Before
    fun setUp() {
        tmpRepo = Files.createTempDirectory("sitatame-baseref-test-").toFile()
    }

    @After
    fun tearDown() {
        tmpRepo.deleteRecursively()
    }

    // -----------------------------------------------------------------------
    // Settings annotation
    // -----------------------------------------------------------------------

    @Test
    fun settingsRef_appearsFirstWithAnnotation() {
        val candidates = BaseRefDiscovery.listCandidates(
            repoRoot = tmpRepo.path,
            currentBranch = "feature/x",
            settingsBaseRef = "origin/develop",
        )
        assertTrue(
            "First candidate should be the annotated settings entry",
            candidates[0].startsWith("origin/develop")
        )
        assertTrue(
            "Annotated entry should contain suffix",
            candidates[0].contains(BaseRefDiscovery.SETTINGS_LABEL_SUFFIX)
        )
    }

    @Test
    fun settingsRef_rawValueAlsoIncluded() {
        val candidates = BaseRefDiscovery.listCandidates(
            repoRoot = tmpRepo.path,
            currentBranch = "feature/x",
            settingsBaseRef = "origin/develop",
        )
        assertTrue("Raw ref should appear in candidates", candidates.contains("origin/develop"))
    }

    @Test
    fun blankSettingsRef_noAnnotationEntry() {
        val candidates = BaseRefDiscovery.listCandidates(
            repoRoot = tmpRepo.path,
            currentBranch = "main",
            settingsBaseRef = "",
        )
        assertFalse(
            "Blank settings → no annotated entry",
            candidates.any { it.contains(BaseRefDiscovery.SETTINGS_LABEL_SUFFIX) }
        )
    }

    // -----------------------------------------------------------------------
    // Well-known remote branches always present
    // -----------------------------------------------------------------------

    @Test
    fun wellKnownRemotes_alwaysPresent() {
        val candidates = BaseRefDiscovery.listCandidates(
            repoRoot = tmpRepo.path,
            currentBranch = "feature/x",
            settingsBaseRef = "",
        )
        assertTrue("origin/main should always be in candidates", candidates.contains("origin/main"))
        assertTrue("origin/master should always be in candidates", candidates.contains("origin/master"))
        assertTrue("origin/develop should always be in candidates", candidates.contains("origin/develop"))
    }

    // -----------------------------------------------------------------------
    // Deduplication
    // -----------------------------------------------------------------------

    @Test
    fun noDuplicates_whenSettingsRefMatchesWellKnown() {
        val candidates = BaseRefDiscovery.listCandidates(
            repoRoot = tmpRepo.path,
            currentBranch = "feature/x",
            settingsBaseRef = "origin/main",
        )
        // "origin/main" appears as raw value AND as a well-known entry → should be deduped to 1
        val count = candidates.count { it == "origin/main" }
        assertEquals("origin/main should appear exactly once (deduped)", 1, count)
    }

    // -----------------------------------------------------------------------
    // remote.origin.head detection
    // -----------------------------------------------------------------------

    @Test
    fun originHeadFromGitConfig_includedInCandidates() {
        // Write a fake .git/config with remote.origin.head = refs/remotes/origin/trunk
        val gitDir = File(tmpRepo, ".git").also { it.mkdirs() }
        File(gitDir, "config").writeText(
            """
            [core]
                repositoryformatversion = 0
            [remote "origin"]
                url = https://github.com/example/repo.git
                HEAD = refs/remotes/origin/trunk
            """.trimIndent()
        )

        val candidates = BaseRefDiscovery.listCandidates(
            repoRoot = tmpRepo.path,
            currentBranch = "feature/x",
            settingsBaseRef = "",
        )

        assertTrue(
            "origin/trunk from git config HEAD should appear in candidates",
            candidates.contains("origin/trunk")
        )
    }

    // -----------------------------------------------------------------------
    // Order: settings entry first
    // -----------------------------------------------------------------------

    @Test
    fun settingsEntry_comesBeforeWellKnownRemotes() {
        val candidates = BaseRefDiscovery.listCandidates(
            repoRoot = tmpRepo.path,
            currentBranch = "feature/x",
            settingsBaseRef = "origin/develop",
        )
        val annotatedIdx = candidates.indexOfFirst { it.contains(BaseRefDiscovery.SETTINGS_LABEL_SUFFIX) }
        val originMainIdx = candidates.indexOf("origin/main")
        assertTrue("annotated settings entry should come before origin/main", annotatedIdx < originMainIdx)
    }
}
