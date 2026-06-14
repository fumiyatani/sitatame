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
import org.junit.jupiter.api.Assertions.assertNull
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
        // Detached HEAD is represented as an empty rawBranch (symbolic-ref
        // exited non-zero and Git.currentBranch returned ""), matching the Go
        // CLI behaviour in gitx.Repo.CurrentBranch.
        val got = normalizeBranch("", "abcdef0123456789abcdef0123456789abcdef01")
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
    fun `empty branch with short sha falls back to empty`() {
        // Matches Go: when both branch and sha resolution fail the CLI leaves
        // branch == "" and BranchSlug("") absorbs the case.
        assertEquals("", normalizeBranch("", ""))
        assertEquals("", normalizeBranch("", "abc"))
    }
}

/**
 * Parity tests for [Git.currentBranch] against `gitx.Repo.CurrentBranch`. The
 * Go side returns "" for detached HEAD; this Kotlin port must do the same
 * (using `symbolic-ref --quiet --short HEAD`) so the downstream review-slug
 * derivation lines up across TUI and Web. We materialise a real git repo per
 * test because the only way to exercise the detached path is to actually
 * detach HEAD.
 */
class CurrentBranchTest {

    @Test
    fun `detached HEAD returns empty string`() {
        val gitAvailable = try {
            val p = ProcessBuilder("git", "--version").redirectErrorStream(true).start()
            p.waitFor(5, TimeUnit.SECONDS) && p.exitValue() == 0
        } catch (_: Exception) {
            false
        }
        org.junit.jupiter.api.Assumptions.assumeTrue(gitAvailable, "git not available; skipping")

        val repo = Files.createTempDirectory("sitatame-web-detached")
        try {
            initRepo(repo)
            repo.resolve("a.txt").writeText("a\n")
            runGit(repo, "add", "a.txt")
            runGit(repo, "commit", "-m", "first")
            // Move HEAD to the commit SHA — detaches HEAD.
            val sha = readGit(repo, "rev-parse", "HEAD").trim()
            runGit(repo, "checkout", "--detach", sha)
            val branch = Git(repo).currentBranch()
            assertEquals("", branch, "detached HEAD should yield empty branch, matching Go CLI")
        } finally {
            runCatching { rmrf(repo) }
        }
    }

    @Test
    fun `on a real branch returns the branch name`() {
        val gitAvailable = try {
            val p = ProcessBuilder("git", "--version").redirectErrorStream(true).start()
            p.waitFor(5, TimeUnit.SECONDS) && p.exitValue() == 0
        } catch (_: Exception) {
            false
        }
        org.junit.jupiter.api.Assumptions.assumeTrue(gitAvailable, "git not available; skipping")

        val repo = Files.createTempDirectory("sitatame-web-branch")
        try {
            initRepo(repo)
            repo.resolve("a.txt").writeText("a\n")
            runGit(repo, "add", "a.txt")
            runGit(repo, "commit", "-m", "first")
            runGit(repo, "checkout", "-b", "feature/x")
            val branch = Git(repo).currentBranch()
            assertEquals("feature/x", branch)
        } finally {
            runCatching { rmrf(repo) }
        }
    }

    private fun initRepo(repo: Path) {
        runGit(repo, "init", "--initial-branch=base")
        runGit(repo, "config", "user.email", "test@example.com")
        runGit(repo, "config", "user.name", "Test User")
        runGit(repo, "config", "commit.gpgsign", "false")
    }

    private fun runGit(repo: Path, vararg args: String) {
        val cmd = mutableListOf("git").apply { addAll(args) }
        val p = ProcessBuilder(cmd).directory(repo.toFile()).redirectErrorStream(true).start()
        val ok = p.waitFor(20, TimeUnit.SECONDS)
        check(ok) { "git ${args.joinToString(" ")} timed out" }
        val out = p.inputStream.readAllBytes().toString(Charsets.UTF_8)
        check(p.exitValue() == 0) { "git ${args.joinToString(" ")} failed: $out" }
    }

    private fun readGit(repo: Path, vararg args: String): String {
        val cmd = mutableListOf("git").apply { addAll(args) }
        val p = ProcessBuilder(cmd).directory(repo.toFile()).redirectErrorStream(false).start()
        val ok = p.waitFor(20, TimeUnit.SECONDS)
        check(ok) { "git ${args.joinToString(" ")} timed out" }
        val out = p.inputStream.readAllBytes().toString(Charsets.UTF_8)
        check(p.exitValue() == 0) { "git ${args.joinToString(" ")} failed" }
        return out
    }

    private fun rmrf(p: Path) {
        if (!Files.exists(p)) return
        Files.walk(p).use { walk ->
            walk.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }
}

/**
 * Verifies the ReviewLoader picks up review.md under the new 1-branch-1-file
 * layout via [SitatamePaths.branchDir]. No git required.
 */
class ReviewLoaderIntegrationTest {

    @Test
    fun `loads review when present`() {
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
            val branchDir = paths.branchDir()
            Files.createDirectories(branchDir)
            val md = branchDir.resolve("review.md")
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

            val reviewPath = ReviewLoader.findReviewPath(branchDir)
            assertNotNull(reviewPath)
            val review = ReviewLoader.load(reviewPath!!)
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

    @Test
    fun `findReviewPath returns null when review dot md absent`() {
        val dir = Files.createTempDirectory("sitatame-web-no-review")
        try {
            // branchDir exists but has no review.md
            assertNull(ReviewLoader.findReviewPath(dir))
        } finally {
            runCatching { rmrf(dir) }
        }
    }

    @Test
    fun `findReviewPath returns null when branchDir does not exist`() {
        val dir = Files.createTempDirectory("sitatame-web-missing-parent")
        val nonExistent = dir.resolve("no-such-branch")
        try {
            assertNull(ReviewLoader.findReviewPath(nonExistent))
        } finally {
            runCatching { rmrf(dir) }
        }
    }

    @Test
    fun `findReviewPath ignores legacy timestamped md files`() {
        val dir = Files.createTempDirectory("sitatame-web-legacy")
        try {
            // Legacy layout left timestamped files; new code must ignore them.
            dir.resolve("20260101T000000-alpha.md").writeText("---\nid: alpha\n---\n\n")
            // review.md is absent — must return null regardless of other .md files.
            assertNull(ReviewLoader.findReviewPath(dir))
        } finally {
            runCatching { rmrf(dir) }
        }
    }

    private fun rmrf(p: Path) {
        if (!Files.exists(p)) return
        Files.walk(p).use { walk ->
            walk.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
        }
    }
}
