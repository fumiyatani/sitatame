package dev.sitatame.intellij.git

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.nio.file.Files
import java.nio.file.Path

/**
 * Verifies [BlobResolver.headBlobSha] against a real (minimal) git repository
 * constructed in a temp directory. No IntelliJ Platform required.
 */
class BlobResolverTest {

    private lateinit var repoDir: Path

    @Before
    fun setUp() {
        repoDir = Files.createTempDirectory("sitatame-blob-resolver-test-")
        git("init", "-b", "main")
        git("config", "user.email", "test@example.com")
        git("config", "user.name", "Test")
        // Create a tracked file and commit it.
        val file = repoDir.resolve("hello.txt")
        Files.writeString(file, "hello\n")
        git("add", "hello.txt")
        git("commit", "-m", "initial")
    }

    @After
    fun tearDown() {
        deleteRecursively(repoDir)
    }

    @Test
    fun trackedFileReturnsSevenCharSha() {
        val sha = BlobResolver.headBlobSha(repoDir.toString(), "hello.txt")
        assertTrue("expected 7-char blob SHA, got '$sha'", sha.length == 7)
        assertTrue("SHA must be hex", sha.matches(Regex("[0-9a-f]{7}")))
    }

    @Test
    fun untrackedFileReturnsEmpty() {
        val sha = BlobResolver.headBlobSha(repoDir.toString(), "nonexistent.txt")
        assertEquals("", sha)
    }

    @Test
    fun emptyRepoRootReturnsEmpty() {
        val sha = BlobResolver.headBlobSha("", "hello.txt")
        assertEquals("", sha)
    }

    @Test
    fun emptyPathReturnsEmpty() {
        val sha = BlobResolver.headBlobSha(repoDir.toString(), "")
        assertEquals("", sha)
    }

    @Test
    fun blobChangeAfterModify() {
        val before = BlobResolver.headBlobSha(repoDir.toString(), "hello.txt")
        assertTrue("initial SHA must be non-empty", before.isNotEmpty())

        // Modify the file and re-add it to the index.
        Files.writeString(repoDir.resolve("hello.txt"), "world\n")
        git("add", "hello.txt")

        val after = BlobResolver.headBlobSha(repoDir.toString(), "hello.txt")
        assertTrue("post-modify SHA must be non-empty", after.isNotEmpty())
        assertTrue("SHA must change after content changes", before != after)
    }

    // -- helpers ---------------------------------------------------------------

    private fun git(vararg args: String) {
        val cmd = listOf("git") + args.toList()
        val proc = ProcessBuilder(cmd)
            .directory(repoDir.toFile())
            .redirectErrorStream(true)
            .start()
        val exitCode = proc.waitFor()
        val output = proc.inputStream.bufferedReader().readText().trim()
        check(exitCode == 0) { "git ${args.joinToString(" ")} failed (exit $exitCode): $output" }
    }

    private fun deleteRecursively(root: Path) {
        if (!Files.exists(root)) return
        Files.walk(root).use { stream ->
            stream.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }
}
