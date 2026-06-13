package dev.sitatame.web.server

import dev.sitatame.web.api.WorkspaceResponse
import io.ktor.client.call.body
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.get
import io.ktor.client.statement.bodyAsText
import io.ktor.http.HttpStatusCode
import io.ktor.serialization.kotlinx.json.json
import io.ktor.server.testing.testApplication
import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import kotlin.io.path.writeText

/**
 * Spins up a fresh git repo in a temp dir, plants one commit on the base ref
 * and one on the feature branch, then asserts the Ktor server returns a
 * structurally sound [WorkspaceResponse].
 *
 * The test is gated on `git` being available on PATH — when it isn't (rare in
 * CI but possible in sandboxed environments) we skip the body. Running git
 * commands in tests is the only way to exercise the full diff parse path with
 * realistic input.
 */
class ServerTest {

    @Test
    fun `workspace endpoint returns branch and at least one file`() = testApplication {
        val gitAvailable = try {
            val p = ProcessBuilder("git", "--version").redirectErrorStream(true).start()
            p.waitFor(5, TimeUnit.SECONDS) && p.exitValue() == 0
        } catch (_: Exception) {
            false
        }
        org.junit.jupiter.api.Assumptions.assumeTrue(gitAvailable, "git not available; skipping")

        val repo = Files.createTempDirectory("sitatame-web-test")
        try {
            initRepo(repo)
            // Base commit.
            repo.resolve("foo.go").writeText("package foo\n")
            git(repo, "add", "foo.go")
            git(repo, "commit", "-m", "base")
            // Create a fake "origin/main" pointing at this commit so the diff
            // call has something to range against.
            git(repo, "update-ref", "refs/remotes/origin/main", "HEAD")
            // Feature commit.
            git(repo, "checkout", "-b", "feature/web")
            repo.resolve("foo.go").writeText("package foo\n\nfunc Hello() {}\n")
            git(repo, "add", "foo.go")
            git(repo, "commit", "-m", "feature")

            application {
                module(repo, "origin/main")
            }
            val client = createClient {
                install(ContentNegotiation) { json(Json { ignoreUnknownKeys = true }) }
            }

            val resp = client.get("/api/v1/workspace")
            assertEquals(HttpStatusCode.OK, resp.status, resp.bodyAsText())
            val workspace: WorkspaceResponse = resp.body()

            assertEquals("feature/web", workspace.branch)
            assertTrue(workspace.projectSlug.contains("__"), "projectSlug: ${workspace.projectSlug}")
            assertEquals(1, workspace.files.size, "files: ${workspace.files}")
            val file = workspace.files[0]
            assertEquals("foo.go", file.path)
            assertEquals(2, file.adds)
            assertEquals(0, file.dels)
            assertTrue(file.hunks.isNotEmpty())
            assertEquals(null, workspace.review) // no review .md planted
        } finally {
            // Best-effort cleanup.
            runCatching { rmrf(repo) }
        }
    }

    @Test
    fun `health endpoint responds ok`() = testApplication {
        application {
            // Use the test resource dir as workdir — health doesn't touch git.
            module(Path.of(System.getProperty("user.dir")), "origin/main")
        }
        val resp = client.get("/api/v1/health")
        assertEquals(HttpStatusCode.OK, resp.status)
        assertEquals("ok", resp.bodyAsText())
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

/**
 * Parity tests for [normalizeBranch] against the Go CLI behaviour in
 * `cmd/root.go`. The CLI folds detached HEAD into `detached/<sha[:12]>`; the
 * Web UI must do the same or the two clients will silently look at different
 * `~/.sitatame/<slug>/reviews/<branch-slug>/` directories for the same repo.
 */
class NormalizeBranchTest {

    @Test
    fun `detached HEAD with sha becomes detached prefix`() {
        val got = normalizeBranch("HEAD", "abcdef0123456789abcdef0123456789abcdef01")
        assertEquals("detached/abcdef012345", got)
        // The slug derived from this must match what BranchSlug("detached/abcdef012345")
        // produces — i.e. the same value the Go CLI's NewPaths(...) would store under.
        val slug = Slug.branchSlug(got)
        assertEquals(Slug.branchSlug("detached/abcdef012345"), slug)
    }

    @Test
    fun `regular branch passes through unchanged`() {
        assertEquals("feature/web", normalizeBranch("feature/web", "abcdef0123456789"))
        assertEquals("main", normalizeBranch("main", "abcdef0123456789"))
    }

    @Test
    fun `HEAD with short sha falls back to raw HEAD`() {
        // Matches Go: when HeadSHA returns "" (unborn / pathological HEAD) the
        // CLI leaves branch == "" and BranchSlug("") absorbs the case. We don't
        // get the "" branch (rev-parse --abbrev-ref returns "HEAD") so the
        // fallback here is the literal "HEAD". The Go side's empty-branch path
        // is reached when `--abbrev-ref` itself errors, which we don't model.
        assertEquals("HEAD", normalizeBranch("HEAD", ""))
        assertEquals("HEAD", normalizeBranch("HEAD", "abc"))
    }

    @Test
    fun `empty branch passes through`() {
        assertEquals("", normalizeBranch("", "abcdef0123456789abcdef01"))
    }
}

/**
 * Verifies the ReviewLoader picks up a planted .md file via the
 * SitatamePaths.reviewsDir() layout. Doesn't need git at all.
 */
class ReviewLoaderIntegrationTest {

    @Test
    fun `loads latest review when present`() {
        val home = Files.createTempDirectory("sitatame-web-home")
        try {
            val repo = Files.createTempDirectory("sitatame-web-repo")
            val branch = "feature/x"

            val paths = SitatamePaths.resolve(
                repoRoot = repo,
                branch = branch,
                envLookup = { if (it == "SITATAME_HOME") home.toString() else null },
                homeDir = home,
            )
            val reviewsDir = paths.reviewsDir()
            Files.createDirectories(reviewsDir)
            val md = reviewsDir.resolve("20260501T000000-test.md")
            md.writeText(
                """
                ---
                schema: 1
                id: 20260501T000000-test
                created_at: 2026-05-01T00:00:00Z
                branch: feature/x
                base:
                  ref: origin/main
                  sha: aaa
                head:
                  ref: HEAD
                  sha: bbb
                comments:
                  - anchor_id: 11111111-1111-1111-1111-111111111111
                    kind: line
                    path: foo.go
                    side: head
                    line: 3
                    state: open
                    body: rename this variable
                ---

                body
                """.trimIndent() + "\n"
            )

            val latest = ReviewLoader.findLatestPath(reviewsDir)
            assertNotNull(latest)
            val review = ReviewLoader.load(latest!!)
            assertEquals("20260501T000000-test", review.id)
            assertEquals("feature/x", review.branch)
            assertEquals("origin/main", review.baseRef)
            assertEquals(1, review.comments.size)
            val c = review.comments[0]
            assertEquals("line", c.kind)
            assertEquals("foo.go", c.path)
            assertEquals(3, c.line)
            assertEquals("open", c.state)
            assertEquals("rename this variable", c.body)
        } finally {
            runCatching { rmrf(home) }
        }
    }

    private fun rmrf(p: Path) {
        if (!Files.exists(p)) return
        Files.walk(p).use { walk ->
            walk.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }
}
