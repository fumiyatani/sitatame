package dev.sitatame.web.server

import dev.sitatame.web.api.CreateCommentRequest
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.nio.file.Files
import java.nio.file.Path

/**
 * Direct unit tests for [ReviewMutationService] without HTTP layer.
 */
class MutationServiceDirectTest {

    @Test
    fun `two sequential addComment calls on same service both succeed`() {
        val home = Files.createTempDirectory("sitatame-direct-home")
        val repo = Files.createTempDirectory("sitatame-direct-repo")
        try {
            val paths = SitatamePaths.resolve(
                repoRoot = repo,
                branch = "feature/test",
                envLookup = { if (it == "SITATAME_HOME") home.toString() else null },
                homeDir = home,
            )

            val service = ReviewMutationService(paths)
            val initialEtag = computeEtag(ByteArray(0))

            val r1 = runBlocking {
                service.addComment(
                    CreateCommentRequest(kind = "review", body = "first"),
                    initialEtag,
                )
            }
            assertTrue(r1 is MutationResult.Success, "Expected Success for first add, got: $r1")
            val etag2 = (r1 as MutationResult.Success).newEtag

            assertTrue(Files.isRegularFile(paths.reviewFile()), "review.md must exist after first add")

            val r2 = runBlocking {
                service.addComment(
                    CreateCommentRequest(kind = "review", body = "second"),
                    etag2,
                )
            }
            assertTrue(r2 is MutationResult.Success, "Expected Success for second add, got: $r2")
        } finally {
            runCatching { rmrf(home) }
            runCatching { rmrf(repo) }
        }
    }

    private fun rmrf(p: Path) {
        if (!Files.exists(p)) return
        Files.walk(p).use { walk ->
            walk.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }
}
