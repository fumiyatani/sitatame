package dev.sitatame.web.server

import dev.sitatame.web.api.CreateCommentRequest
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.get
import io.ktor.client.request.header
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.HttpStatusCode
import io.ktor.http.contentType
import io.ktor.serialization.kotlinx.json.json
import io.ktor.server.testing.testApplication
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import kotlin.io.path.writeText

/**
 * Concurrency test: two simultaneous POST /comments with the same ETag must
 * result in exactly one 200 and one 412.
 *
 * The path-scoped Mutex in [ReviewMutationService] serialises the
 * ETag-check → write sequence so the loser sees the updated ETag and gets 412.
 */
class ConcurrentEditTest {

    @Test
    fun `concurrent posts with same ETag produce one 200 and one 412`() {
        val gitAvailable = try {
            val p = ProcessBuilder("git", "--version").redirectErrorStream(true).start()
            p.waitFor(5, TimeUnit.SECONDS) && p.exitValue() == 0
        } catch (_: Exception) {
            false
        }
        org.junit.jupiter.api.Assumptions.assumeTrue(gitAvailable, "git not available; skipping")

        val home = Files.createTempDirectory("sitatame-concurrent-home")
        val repo = Files.createTempDirectory("sitatame-concurrent-repo")
        try {
            initRepo(repo)
            repo.resolve("foo.go").writeText("package foo\n")
            git(repo, "add", "foo.go")
            git(repo, "commit", "-m", "base")
            git(repo, "update-ref", "refs/remotes/origin/main", "HEAD")
            git(repo, "checkout", "-b", "feature/concurrent")

            testApplication {
                application { module(repo, "origin/main") }
                val client = createClient {
                    install(ContentNegotiation) {
                        json(Json { ignoreUnknownKeys = true; encodeDefaults = true })
                    }
                }

                // Obtain initial ETag.
                val wsResp = client.get("/api/v1/workspace")
                val initialEtag = wsResp.headers["ETag"]!!

                // Launch two concurrent requests with the same ETag.
                val results = runBlocking(Dispatchers.IO) {
                    val d1 = async {
                        client.post("/api/v1/comments") {
                            contentType(ContentType.Application.Json)
                            header("If-Match", initialEtag)
                            setBody(CreateCommentRequest(kind = "review", body = "first concurrent"))
                        }
                    }
                    val d2 = async {
                        client.post("/api/v1/comments") {
                            contentType(ContentType.Application.Json)
                            header("If-Match", initialEtag)
                            setBody(CreateCommentRequest(kind = "review", body = "second concurrent"))
                        }
                    }
                    listOf(d1.await(), d2.await())
                }

                val statuses = results.map { it.status }
                val successCount = statuses.count { it == HttpStatusCode.OK }
                val conflictCount = statuses.count { it == HttpStatusCode.PreconditionFailed }

                assertTrue(successCount == 1, "Expected exactly one 200, got: $statuses")
                assertTrue(conflictCount == 1, "Expected exactly one 412, got: $statuses")
            }
        } finally {
            runCatching { rmrf(home) }
            runCatching { rmrf(repo) }
        }
    }

    private fun initRepo(repo: Path) {
        git(repo, "init", "--initial-branch=base")
        git(repo, "config", "user.email", "test@example.com")
        git(repo, "config", "user.name", "Test User")
        git(repo, "config", "commit.gpgsign", "false")
    }

    private fun git(repo: Path, vararg args: String) {
        val cmd = mutableListOf("git").apply { addAll(args) }
        val p = ProcessBuilder(cmd).directory(repo.toFile()).redirectErrorStream(true).start()
        val ok = p.waitFor(20, TimeUnit.SECONDS)
        check(ok) { "git ${args.joinToString(" ")} timed out" }
        val out = p.inputStream.readAllBytes().toString(Charsets.UTF_8)
        check(p.exitValue() == 0) { "git ${args.joinToString(" ")} failed: $out" }
    }

    private fun rmrf(p: Path) {
        if (!Files.exists(p)) return
        Files.walk(p).use { walk ->
            walk.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }
}
