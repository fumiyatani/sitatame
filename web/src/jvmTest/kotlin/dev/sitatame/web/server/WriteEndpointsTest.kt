package dev.sitatame.web.server

import dev.sitatame.web.api.CreateCommentRequest
import dev.sitatame.web.api.EtagMismatchError
import dev.sitatame.web.api.UpdateCommentStateRequest
import dev.sitatame.web.api.UpdateReviewCommentRequest
import io.ktor.client.call.body
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.get
import io.ktor.client.request.header
import io.ktor.client.request.patch
import io.ktor.client.request.post
import io.ktor.client.request.put
import io.ktor.client.request.setBody
import io.ktor.client.statement.bodyAsText
import io.ktor.http.ContentType
import io.ktor.http.HttpStatusCode
import io.ktor.http.contentType
import io.ktor.serialization.kotlinx.json.json
import io.ktor.server.testing.testApplication
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import kotlin.io.path.writeText

/**
 * Integration tests for write-path endpoints: POST /api/v1/comments,
 * PATCH /api/v1/comments/{anchorId}/state, PUT /api/v1/review-comment, and
 * the ETag header on GET /api/v1/workspace.
 *
 * Tests use a real temp-dir git repo so the full server module() path is
 * exercised, same approach as [ServerTest].
 */
class WriteEndpointsTest {

    // -----------------------------------------------------------------------
    // GET /workspace — ETag header
    // -----------------------------------------------------------------------

    @Nested
    inner class WorkspaceEtag {

        @Test
        fun `workspace returns ETag header when review dot md absent`() {
            assumeGit()
            withGitRepo { repo, home ->
                testApplication {
                    application {
                        module(repo, "origin/main")
                    }
                    val client = jsonClient()
                    val resp = client.get("/api/v1/workspace")
                    assertEquals(HttpStatusCode.OK, resp.status)
                    val etag = resp.headers["ETag"]
                    assertNotNull(etag, "ETag header missing")
                    // No review.md -> sentinel
                    assertTrue(etag!!.contains("empty"), "expected 'empty' ETag, got: $etag")
                }
            }
        }

        @Test
        fun `workspace returns non-empty ETag when review dot md exists`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    // First, create a comment so review.md is created via the service.
                    val ws0 = client.get("/api/v1/workspace")
                    val etag0 = ws0.headers["ETag"]!!
                    val postResp = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag0)
                        setBody(CreateCommentRequest(kind = "review", body = "seed for etag test"))
                    }
                    assertEquals(HttpStatusCode.OK, postResp.status, postResp.bodyAsText())

                    // Now GET /workspace should return a sha256 ETag.
                    val resp = client.get("/api/v1/workspace")
                    assertEquals(HttpStatusCode.OK, resp.status)
                    val etag = resp.headers["ETag"]
                    assertNotNull(etag)
                    assertTrue(etag!!.contains("sha256-"), "expected sha256 ETag, got: $etag")
                }
            }
        }
    }

    // -----------------------------------------------------------------------
    // POST /api/v1/comments
    // -----------------------------------------------------------------------

    @Nested
    inner class PostComments {

        @Test
        fun `adds comment to empty review and returns 200 with ETag`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    // First get workspace to obtain initial ETag.
                    val wsResp = client.get("/api/v1/workspace")
                    val initialEtag = wsResp.headers["ETag"]!!

                    val resp = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", initialEtag)
                        setBody(
                            CreateCommentRequest(
                                kind = "line",
                                path = "foo.go",
                                side = "head",
                                line = 5,
                                body = "looks good",
                            )
                        )
                    }
                    assertEquals(HttpStatusCode.OK, resp.status, resp.bodyAsText())
                    val newEtag = resp.headers["ETag"]
                    assertNotNull(newEtag, "ETag header missing from 200 response")
                    assertTrue(newEtag!!.contains("sha256-"), "ETag should be sha256-based: $newEtag")

                    val body = Json.decodeFromString<JsonObject>(resp.bodyAsText())
                    assertNotNull(body["anchor_id"])
                }
            }
        }

        @Test
        fun `adds second comment to existing review`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    val ws1 = client.get("/api/v1/workspace")
                    val etag1 = ws1.headers["ETag"]!!

                    val r1 = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag1)
                        setBody(CreateCommentRequest(kind = "review", body = "first"))
                    }
                    assertEquals(HttpStatusCode.OK, r1.status, r1.bodyAsText())
                    val etag2 = r1.headers["ETag"]!!

                    val r2 = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag2)
                        setBody(CreateCommentRequest(kind = "review", body = "second"))
                    }
                    assertEquals(HttpStatusCode.OK, r2.status, r2.bodyAsText())
                    val etag3 = r2.headers["ETag"]!!
                    assertTrue(etag3 != etag2, "ETag must change after second write")
                }
            }
        }

        @Test
        fun `returns 412 when If-Match is stale`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    val ws = client.get("/api/v1/workspace")
                    val etag = ws.headers["ETag"]!!

                    // First post succeeds.
                    val r1 = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag)
                        setBody(CreateCommentRequest(kind = "review", body = "first"))
                    }
                    assertEquals(HttpStatusCode.OK, r1.status)

                    // Second post with the old ETag must get 412.
                    val r2 = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag)
                        setBody(CreateCommentRequest(kind = "review", body = "stale attempt"))
                    }
                    assertEquals(HttpStatusCode.PreconditionFailed, r2.status)
                    val mismatch = r2.body<EtagMismatchError>()
                    assertEquals("etag_mismatch", mismatch.error)
                    assertNotNull(r2.headers["ETag"])
                }
            }
        }

        @Test
        fun `returns 422 for kind=line with missing line`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    val ws = client.get("/api/v1/workspace")
                    val etag = ws.headers["ETag"]!!

                    val resp = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag)
                        setBody(
                            CreateCommentRequest(
                                kind = "line",
                                path = "foo.go",
                                // line intentionally missing
                                body = "oops",
                            )
                        )
                    }
                    assertEquals(HttpStatusCode.UnprocessableEntity, resp.status)
                    val body = Json.decodeFromString<JsonObject>(resp.bodyAsText())
                    val errors = body["errors"]
                    assertNotNull(errors)
                }
            }
        }

        @Test
        fun `returns 400 when If-Match header is absent`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    val resp = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        // No If-Match header
                        setBody(CreateCommentRequest(kind = "review", body = "hi"))
                    }
                    assertEquals(HttpStatusCode.BadRequest, resp.status)
                }
            }
        }
    }

    // -----------------------------------------------------------------------
    // PATCH /api/v1/comments/{anchorId}/state
    // -----------------------------------------------------------------------

    @Nested
    inner class PatchCommentState {

        @Test
        fun `transitions comment state open to resolved`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    val ws1 = client.get("/api/v1/workspace")
                    val etag1 = ws1.headers["ETag"]!!

                    // Create a comment first.
                    val postResp = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag1)
                        setBody(CreateCommentRequest(kind = "review", body = "resolve me"))
                    }
                    assertEquals(HttpStatusCode.OK, postResp.status)
                    val anchorId = Json.decodeFromString<JsonObject>(postResp.bodyAsText())["anchor_id"]!!
                        .jsonPrimitive.content
                    val etag2 = postResp.headers["ETag"]!!

                    val patchResp = client.patch("/api/v1/comments/$anchorId/state") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag2)
                        setBody(UpdateCommentStateRequest(state = "resolved"))
                    }
                    assertEquals(HttpStatusCode.OK, patchResp.status, patchResp.bodyAsText())
                    assertNotNull(patchResp.headers["ETag"])
                }
            }
        }

        @Test
        fun `returns 412 for stale ETag on state patch`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    val ws1 = client.get("/api/v1/workspace")
                    val etag1 = ws1.headers["ETag"]!!

                    val postResp = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag1)
                        setBody(CreateCommentRequest(kind = "review", body = "test"))
                    }
                    assertEquals(HttpStatusCode.OK, postResp.status)
                    val anchorId = Json.decodeFromString<JsonObject>(postResp.bodyAsText())["anchor_id"]!!
                        .jsonPrimitive.content

                    // Use the stale ETag (etag1) instead of the post's ETag.
                    val patchResp = client.patch("/api/v1/comments/$anchorId/state") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag1) // stale
                        setBody(UpdateCommentStateRequest(state = "resolved"))
                    }
                    assertEquals(HttpStatusCode.PreconditionFailed, patchResp.status)
                }
            }
        }

        @Test
        fun `returns 422 for invalid state value`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    val ws1 = client.get("/api/v1/workspace")
                    val etag1 = ws1.headers["ETag"]!!

                    val postResp = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag1)
                        setBody(CreateCommentRequest(kind = "review", body = "test"))
                    }
                    assertEquals(HttpStatusCode.OK, postResp.status)
                    val anchorId = Json.decodeFromString<JsonObject>(postResp.bodyAsText())["anchor_id"]!!
                        .jsonPrimitive.content
                    val etag2 = postResp.headers["ETag"]!!

                    val patchResp = client.patch("/api/v1/comments/$anchorId/state") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag2)
                        setBody(UpdateCommentStateRequest(state = "INVALID"))
                    }
                    assertEquals(HttpStatusCode.UnprocessableEntity, patchResp.status)
                }
            }
        }
    }

    // -----------------------------------------------------------------------
    // PUT /api/v1/review-comment
    // -----------------------------------------------------------------------

    @Nested
    inner class PutReviewComment {

        @Test
        fun `updates review-level comment and returns 200`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    val ws1 = client.get("/api/v1/workspace")
                    val etag1 = ws1.headers["ETag"]!!

                    // Need an existing review.md first (PUT requires existing file).
                    // Create it by posting a comment.
                    val postResp = client.post("/api/v1/comments") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag1)
                        setBody(CreateCommentRequest(kind = "review", body = "seed"))
                    }
                    assertEquals(HttpStatusCode.OK, postResp.status)
                    val etag2 = postResp.headers["ETag"]!!

                    val putResp = client.put("/api/v1/review-comment") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag2)
                        setBody(UpdateReviewCommentRequest(text = "Overall LGTM"))
                    }
                    assertEquals(HttpStatusCode.OK, putResp.status, putResp.bodyAsText())
                    assertNotNull(putResp.headers["ETag"])
                }
            }
        }

        @Test
        fun `returns 404 when review dot md does not exist`() {
            assumeGit()
            withGitRepo { repo, home ->
                git(repo, "checkout", "-b", "feature/web")

                testApplication {
                    application { module(repo, "origin/main") }
                    val client = jsonClient()

                    val ws = client.get("/api/v1/workspace")
                    val etag = ws.headers["ETag"]!!

                    val putResp = client.put("/api/v1/review-comment") {
                        contentType(ContentType.Application.Json)
                        header("If-Match", etag)
                        setBody(UpdateReviewCommentRequest(text = "no review yet"))
                    }
                    assertEquals(HttpStatusCode.NotFound, putResp.status)
                }
            }
        }
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun io.ktor.server.testing.ApplicationTestBuilder.jsonClient() = createClient {
        install(ContentNegotiation) {
            json(Json {
                ignoreUnknownKeys = true
                encodeDefaults = true
            })
        }
    }

    /** Run [block] with a fresh temp git repo + home dir. */
    private fun withGitRepo(block: (repo: Path, home: Path) -> Unit) {
        val home = Files.createTempDirectory("sitatame-web-home")
        val repo = Files.createTempDirectory("sitatame-web-repo")
        try {
            initRepo(repo)
            repo.resolve("foo.go").writeText("package foo\n")
            git(repo, "add", "foo.go")
            git(repo, "commit", "-m", "base")
            git(repo, "update-ref", "refs/remotes/origin/main", "HEAD")
            block(repo, home)
        } finally {
            runCatching { rmrf(home) }
            runCatching { rmrf(repo) }
        }
    }

    private fun assumeGit() {
        val available = try {
            val p = ProcessBuilder("git", "--version").redirectErrorStream(true).start()
            p.waitFor(5, TimeUnit.SECONDS) && p.exitValue() == 0
        } catch (_: Exception) {
            false
        }
        org.junit.jupiter.api.Assumptions.assumeTrue(available, "git not available; skipping")
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
