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

    /**
     * Unstaged working-tree edit: the index blob must not change until
     * `git add` is run. [BlobResolver] intentionally reads the index, so a
     * dirty working tree should return the previously-staged blob (stale
     * detection consistency).
     */
    @Test
    fun unstagedWorkingTreeEditReturnsStagedBlob() {
        val indexBlob = BlobResolver.headBlobSha(repoDir.toString(), "hello.txt")
        assertTrue("index blob must be non-empty", indexBlob.isNotEmpty())

        // Modify the working tree but do NOT stage.
        Files.writeString(repoDir.resolve("hello.txt"), "dirty working tree\n")

        val afterDirty = BlobResolver.headBlobSha(repoDir.toString(), "hello.txt")
        assertEquals(
            "unstaged edit must not change the index blob",
            indexBlob,
            afterDirty,
        )
    }

    /** Non-Git directory: resolver must return empty without throwing. */
    @Test
    fun nonGitDirectoryReturnsEmpty() {
        val nonGit = Files.createTempDirectory("sitatame-non-git-")
        try {
            val sha = BlobResolver.headBlobSha(nonGit.toString(), "hello.txt")
            assertEquals("", sha)
        } finally {
            deleteRecursively(nonGit)
        }
    }

    /** Non-ASCII (Japanese) path is tracked and returns a valid blob SHA. */
    @Test
    fun nonAsciiPathReturnsBlob() {
        val name = "日本語ファイル.txt"
        Files.writeString(repoDir.resolve(name), "unicode\n")
        git("add", name)
        git("commit", "-m", "add non-ascii file")

        val sha = BlobResolver.headBlobSha(repoDir.toString(), name)
        assertTrue("non-ASCII path must return 7-char SHA, got '$sha'", sha.length == 7)
        assertTrue("SHA must be hex", sha.matches(Regex("[0-9a-fA-F]{7}")))
    }

    /**
     * Leading-dash path: `--` separator in the ProcessBuilder command ensures
     * git does not interpret the path as a flag. Should return empty (file not
     * tracked), not throw or corrupt the subprocess.
     */
    @Test
    fun leadingDashPathReturnsEmpty() {
        val sha = BlobResolver.headBlobSha(repoDir.toString(), "--dangerous")
        assertEquals("leading-dash path must return empty", "", sha)
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
