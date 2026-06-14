package dev.sitatame.web.server

import dev.sitatame.web.api.WorkspaceResponse
import io.ktor.http.HttpStatusCode
import io.ktor.serialization.kotlinx.json.json
import io.ktor.server.application.Application
import io.ktor.server.application.install
import io.ktor.server.engine.embeddedServer
import io.ktor.server.http.content.staticResources
import io.ktor.server.netty.Netty
import io.ktor.server.plugins.contentnegotiation.ContentNegotiation
import io.ktor.server.plugins.statuspages.StatusPages
import io.ktor.server.response.respond
import io.ktor.server.response.respondText
import io.ktor.server.routing.get
import io.ktor.server.routing.routing
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import java.nio.file.Path

/**
 * Ktor backend for the Compose for Web read-only viewer.
 *
 * Endpoints:
 *  - `GET /api/v1/workspace` — current repo's branch + file diff + latest
 *    review document. Empty `files` and `review == null` are the legitimate
 *    no-changes / no-review-yet responses.
 *  - `GET /` and `GET /{path...}` — Compose Wasm dist served from
 *    `resources/static`. The Wasm bundle is copied there by the build (see
 *    README).
 *
 * The base ref for `git diff` is hard-coded to `origin/main` for Phase 1 step
 * 1; per-repo configuration is Phase 1 step 2.
 */

/** Port 0 selects a free port. The actual bound port is printed on stdout. */
const val DEFAULT_PORT: Int = 0

/** Hard-coded base ref for Phase 1 step 1. */
const val DEFAULT_BASE_REF: String = "origin/main"

fun main() {
    val workdir = Path.of(System.getProperty("user.dir"))
    val server = embeddedServer(Netty, port = DEFAULT_PORT, host = "127.0.0.1") {
        module(workdir, DEFAULT_BASE_REF)
    }.start(wait = false)

    val resolved = runBlocking { server.engine.resolvedConnectors() }
    val port = resolved.firstOrNull()?.port
        ?: error("Ktor failed to bind any connector")
    // stdout, single line — the launcher / IDE parses this.
    println("SITATAME_WEB_URL=http://127.0.0.1:$port")

    // Block until the engine shuts down. embeddedServer(wait = true) would do
    // this for us but we need the connector info before the call returns.
    Runtime.getRuntime().addShutdownHook(Thread { server.stop(1_000, 5_000) })
    Thread.currentThread().join()
}

/**
 * Wire the application module. Split out so [io.ktor.server.testing.testApplication]
 * can reuse it without going through main().
 */
fun Application.module(workdir: Path, baseRef: String) {
    install(ContentNegotiation) {
        json(Json {
            prettyPrint = false
            ignoreUnknownKeys = true
            encodeDefaults = true
        })
    }
    install(StatusPages) {
        exception<Throwable> { call, cause ->
            // Surfacing the message in dev is fine — there is no auth and no
            // secret state. Stack traces stay server-side.
            call.respondText(
                text = "internal error: ${cause.message ?: cause::class.simpleName}",
                status = HttpStatusCode.InternalServerError,
            )
        }
    }

    val workspace = WorkspaceService(workdir, baseRef)

    routing {
        get("/api/v1/workspace") {
            call.respond(workspace.snapshot())
        }
        get("/api/v1/health") {
            call.respondText("ok")
        }

        // Compose Wasm bundle. Copy via
        // `./gradlew :web:wasmJsBrowserDistribution` and the build wires the
        // output into `src/jvmMain/resources/static/`. The route is registered
        // even when the static directory is missing so the JSON API is still
        // useful in dev.
        staticResources("/", "static") {
            default("index.html")
        }
    }
}

/**
 * Normalises the raw `Git.currentBranch()` output the same way the Go CLI does
 * in `cmd/root.go`: when HEAD is detached, `symbolic-ref` exits non-zero and
 * we get back an empty string, which on its own would route reviews to
 * `BranchSlug("")` and collide across every detached session on the same
 * machine. The CLI folds detached state into `detached/<sha[:12]>` so each
 * detached HEAD has its own per-SHA slug. Without this, the Web UI and CLI of
 * the same repo in the same detached state would look at different on-disk
 * directories for reviews.
 *
 * Falls back to the raw branch string when [sha] is too short to take a
 * 12-char prefix from (e.g. HeadSHA failed). This matches Go's behaviour:
 * when both branch and sha resolution fail the CLI also leaves branch == ""
 * and lets BranchSlug("") absorb the case.
 */
fun normalizeBranch(rawBranch: String, sha: String): String {
    if (rawBranch.isEmpty() && sha.length >= 12) {
        return "detached/" + sha.substring(0, 12)
    }
    return rawBranch
}

/**
 * Owns the assembly of the `WorkspaceResponse` from git + the review YAML on
 * disk. Stateless across requests (re-runs git each call) so the response
 * always reflects the current working tree.
 */
class WorkspaceService(
    private val workdir: Path,
    private val baseRef: String,
) {
    fun snapshot(): WorkspaceResponse {
        val git = Git(workdir)
        val repoRoot = git.repoRoot()
        val rawBranch = git.currentBranch()
        val headSha = git.headSHA()
        val branch = normalizeBranch(rawBranch, headSha)
        val files = try {
            DiffParser.parse(git.unifiedDiff(baseRef))
        } catch (e: Exception) {
            // When the base ref isn't reachable (e.g. fresh clone without
            // origin/main) we degrade gracefully to "no diff yet" instead of
            // 500ing the whole UI. The error is logged via stderr.
            System.err.println("sitatame-web: git diff failed: ${e.message}")
            emptyList()
        }

        val paths = SitatamePaths.resolve(repoRoot, branch)
        val reviewPath = ReviewLoader.findReviewPath(paths.branchDir())
        val review = reviewPath?.let { ReviewLoader.load(it) }

        return WorkspaceResponse(
            projectSlug = paths.projectSlug,
            branch = branch,
            files = files,
            review = review,
            sourcePath = reviewPath?.toAbsolutePath()?.toString(),
        )
    }
}
