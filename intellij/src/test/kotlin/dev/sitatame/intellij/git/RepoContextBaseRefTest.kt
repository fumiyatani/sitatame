package dev.sitatame.intellij.git

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import java.io.File
import java.nio.file.Files

/**
 * Unit tests for [RepoContext.resolveBaseRef] and
 * [RepoContext.readOriginHeadFromGitConfig].
 *
 * These run without the IntelliJ Platform (no ApplicationManager, no
 * GitRepositoryManager) because they exercise pure functions that operate only
 * on strings and the filesystem.
 */
class RepoContextBaseRefTest {

    private lateinit var tmpRepo: File

    @Before
    fun setUp() {
        tmpRepo = Files.createTempDirectory("sitatame-repoctx-test-").toFile()
    }

    @After
    fun tearDown() {
        tmpRepo.deleteRecursively()
    }

    // -----------------------------------------------------------------------
    // resolveBaseRef — priority chain
    // -----------------------------------------------------------------------

    @Test
    fun resolveBaseRef_explicitSettingTakesPriority() {
        // Even if .git/config would yield something, the explicit setting wins.
        writeGitConfig(originHead = "refs/remotes/origin/develop")
        val result = RepoContext.resolveBaseRef(tmpRepo.path, "origin/custom-branch")
        assertEquals("origin/custom-branch", result)
    }

    @Test
    fun resolveBaseRef_autoDetectsOriginHead() {
        writeGitConfig(originHead = "refs/remotes/origin/develop")
        val result = RepoContext.resolveBaseRef(tmpRepo.path, "")
        assertEquals("origin/develop", result)
    }

    @Test
    fun resolveBaseRef_fallsBackToOriginMainWhenNoGitConfig() {
        // No .git directory at all.
        val result = RepoContext.resolveBaseRef(tmpRepo.path, "")
        assertEquals("origin/main", result)
    }

    @Test
    fun resolveBaseRef_fallsBackToOriginMainWhenNoHeadInConfig() {
        writeGitConfig(originHead = null)
        val result = RepoContext.resolveBaseRef(tmpRepo.path, "")
        assertEquals("origin/main", result)
    }

    @Test
    fun resolveBaseRef_blankSettingTriggersAutoDetect() {
        writeGitConfig(originHead = "refs/remotes/origin/master")
        val result = RepoContext.resolveBaseRef(tmpRepo.path, "   ")
        assertEquals("origin/master", result)
    }

    // -----------------------------------------------------------------------
    // readOriginHeadFromGitConfig
    // -----------------------------------------------------------------------

    @Test
    fun readOriginHead_parsesRefsRemotesPrefix() {
        writeGitConfig(originHead = "refs/remotes/origin/main")
        val result = RepoContext.readOriginHeadFromGitConfig(tmpRepo.path)
        assertEquals("origin/main", result)
    }

    @Test
    fun readOriginHead_parsesNonStandardValue() {
        // Some setups write a bare branch name without refs/remotes/ prefix.
        writeGitConfig(originHead = "origin/release-2.0")
        val result = RepoContext.readOriginHeadFromGitConfig(tmpRepo.path)
        assertEquals("origin/release-2.0", result)
    }

    @Test
    fun readOriginHead_returnsNullWhenNoGitDir() {
        val result = RepoContext.readOriginHeadFromGitConfig(tmpRepo.path)
        assertNull(result)
    }

    @Test
    fun readOriginHead_returnsNullWhenNoRemoteOriginSection() {
        val gitDir = File(tmpRepo, ".git").also { it.mkdirs() }
        File(gitDir, "config").writeText(
            """
            [core]
                repositoryformatversion = 0
            """.trimIndent()
        )
        val result = RepoContext.readOriginHeadFromGitConfig(tmpRepo.path)
        assertNull(result)
    }

    @Test
    fun readOriginHead_caseInsensitiveHeadKey() {
        // The key in git config is case-insensitive per git spec.
        val gitDir = File(tmpRepo, ".git").also { it.mkdirs() }
        File(gitDir, "config").writeText(
            """
            [remote "origin"]
                fetch = +refs/heads/*:refs/remotes/origin/*
                HEAD = refs/remotes/origin/trunk
            """.trimIndent()
        )
        val result = RepoContext.readOriginHeadFromGitConfig(tmpRepo.path)
        assertEquals("origin/trunk", result)
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    /**
     * Write a minimal `.git/config` under [tmpRepo].
     * If [originHead] is null the `[remote "origin"]` section omits the HEAD key.
     */
    private fun writeGitConfig(originHead: String?) {
        val gitDir = File(tmpRepo, ".git").also { it.mkdirs() }
        val headLine = if (originHead != null) "    HEAD = $originHead\n" else ""
        File(gitDir, "config").writeText(
            """
            [core]
                repositoryformatversion = 0
                filemode = true
            [remote "origin"]
                fetch = +refs/heads/*:refs/remotes/origin/*
                url = https://github.com/example/repo.git
            $headLine
            """.trimIndent()
        )
    }
}
