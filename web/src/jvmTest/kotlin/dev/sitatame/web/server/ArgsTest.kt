package dev.sitatame.web.server

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import java.nio.file.Files

/**
 * Unit tests for [parseArgs] and [validateRepoPath].
 *
 * All tests use a real temp directory with / without `.git` so path validation
 * exercises the actual filesystem rather than mocking [Files.exists].
 *
 * Resolution priority: CLI arg > env var > system default.
 */
class ArgsTest {

    // ---------- helpers ----------

    private fun makeGitDir(): java.nio.file.Path {
        val dir = Files.createTempDirectory("sitatame-args-test")
        Files.createDirectory(dir.resolve(".git"))
        return dir
    }

    private fun makeNonGitDir(): java.nio.file.Path =
        Files.createTempDirectory("sitatame-args-test-nogit")

    private fun rmrf(p: java.nio.file.Path) {
        if (!Files.exists(p)) return
        Files.walk(p).use { walk ->
            walk.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }

    private fun noEnv(name: String): String? = null

    // ---------- --repo flag ----------

    @Test
    fun `--repo flag overrides env and default`() {
        val dir = makeGitDir()
        try {
            val got = parseArgs(
                args = arrayOf("--repo", dir.toString()),
                envLookup = { if (it == "SITATAME_REPO") "/some/other/path" else null },
            )
            assertEquals(dir.toAbsolutePath().normalize(), got.repoPath)
        } finally {
            rmrf(dir)
        }
    }

    @Test
    fun `--repo without value throws ArgParseException`() {
        assertThrows<ArgParseException> {
            parseArgs(arrayOf("--repo"), envLookup = ::noEnv)
        }
    }

    // ---------- --base flag ----------

    @Test
    fun `--base flag overrides env and default`() {
        val dir = makeGitDir()
        try {
            val got = parseArgs(
                args = arrayOf("--repo", dir.toString(), "--base", "origin/develop"),
                envLookup = { if (it == "SITATAME_BASE") "origin/ignored" else null },
            )
            assertEquals("origin/develop", got.baseRef)
        } finally {
            rmrf(dir)
        }
    }

    @Test
    fun `--base without value throws ArgParseException`() {
        val dir = makeGitDir()
        try {
            assertThrows<ArgParseException> {
                parseArgs(arrayOf("--repo", dir.toString(), "--base"), envLookup = ::noEnv)
            }
        } finally {
            rmrf(dir)
        }
    }

    // ---------- environment variable fallback ----------

    @Test
    fun `SITATAME_REPO env used when no --repo flag`() {
        val dir = makeGitDir()
        try {
            val got = parseArgs(
                args = arrayOf("--base", "origin/main"),
                envLookup = {
                    when (it) {
                        "SITATAME_REPO" -> dir.toString()
                        else -> null
                    }
                },
            )
            assertEquals(dir.toAbsolutePath().normalize(), got.repoPath)
        } finally {
            rmrf(dir)
        }
    }

    @Test
    fun `SITATAME_BASE env used when no --base flag`() {
        val dir = makeGitDir()
        try {
            val got = parseArgs(
                args = arrayOf("--repo", dir.toString()),
                envLookup = {
                    when (it) {
                        "SITATAME_BASE" -> "origin/release"
                        else -> null
                    }
                },
            )
            assertEquals("origin/release", got.baseRef)
        } finally {
            rmrf(dir)
        }
    }

    @Test
    fun `blank SITATAME_BASE env falls back to default`() {
        val dir = makeGitDir()
        try {
            val got = parseArgs(
                args = arrayOf("--repo", dir.toString()),
                envLookup = {
                    when (it) {
                        "SITATAME_BASE" -> "   "
                        else -> null
                    }
                },
            )
            assertEquals(DEFAULT_BASE_REF, got.baseRef)
        } finally {
            rmrf(dir)
        }
    }

    // ---------- defaults ----------

    @Test
    fun `default baseRef is origin slash main`() {
        val dir = makeGitDir()
        try {
            val got = parseArgs(
                args = arrayOf("--repo", dir.toString()),
                envLookup = ::noEnv,
            )
            assertEquals(DEFAULT_BASE_REF, got.baseRef)
        } finally {
            rmrf(dir)
        }
    }

    // ---------- validation ----------

    @Test
    fun `non-existent repo path throws ArgParseException`() {
        val ex = assertThrows<ArgParseException> {
            parseArgs(
                args = arrayOf("--repo", "/no/such/path/that/exists/hopefully"),
                envLookup = ::noEnv,
            )
        }
        assertTrue(ex.message!!.contains("does not exist"), "message: ${ex.message}")
    }

    @Test
    fun `directory without dot-git throws ArgParseException`() {
        val dir = makeNonGitDir()
        try {
            val ex = assertThrows<ArgParseException> {
                parseArgs(args = arrayOf("--repo", dir.toString()), envLookup = ::noEnv)
            }
            assertTrue(ex.message!!.contains(".git"), "message: ${ex.message}")
        } finally {
            rmrf(dir)
        }
    }

    @Test
    fun `worktree-style dot-git file is accepted`() {
        // git worktrees use a plain file named `.git` instead of a directory.
        val dir = Files.createTempDirectory("sitatame-args-worktree")
        try {
            dir.resolve(".git").toFile().writeText("gitdir: /some/path/.git/worktrees/wt1\n")
            val got = parseArgs(args = arrayOf("--repo", dir.toString()), envLookup = ::noEnv)
            assertEquals(dir.toAbsolutePath().normalize(), got.repoPath)
        } finally {
            rmrf(dir)
        }
    }

    // ---------- unknown flags ----------

    @Test
    fun `unknown flag throws ArgParseException`() {
        val ex = assertThrows<ArgParseException> {
            parseArgs(args = arrayOf("--unknown"), envLookup = ::noEnv)
        }
        assertTrue(ex.message!!.contains("Unknown flag"), "message: ${ex.message}")
    }

    // ---------- --help ----------

    @Test
    fun `--help throws HelpRequestedException`() {
        assertThrows<HelpRequestedException> {
            parseArgs(args = arrayOf("--help"), envLookup = ::noEnv)
        }
    }
}
