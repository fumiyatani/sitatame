package dev.sitatame.web.server

import dev.sitatame.web.api.CreateCommentRequest
import dev.sitatame.web.api.EtagMismatchError
import dev.sitatame.web.api.UpdateCommentStateRequest
import dev.sitatame.web.api.UpdateReviewCommentRequest
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
import io.ktor.server.request.receive
import io.ktor.server.response.header
import io.ktor.server.response.respond
import io.ktor.server.response.respondText
import io.ktor.server.routing.get
import io.ktor.server.routing.patch
import io.ktor.server.routing.post
import io.ktor.server.routing.put
import io.ktor.server.routing.route
import io.ktor.server.routing.routing
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import java.nio.file.Files
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
            val snapshot = workspace.snapshot()
            // Attach ETag of the current review.md (or "empty" sentinel).
            val reviewBytes = snapshot.sourcePath
                ?.let { p -> runCatching { Files.readAllBytes(Path.of(p)) }.getOrNull() }
                ?: ByteArray(0)
            call.response.header("ETag", computeEtag(reviewBytes))
            call.respond(snapshot)
        }
        get("/api/v1/health") {
            call.respondText("ok")
        }

        // Write-path endpoints (Phase 1 step 2).
        // All mutating endpoints require an If-Match header for optimistic
        // concurrency control.

        route("/api/v1/comments") {
            post {
                val ifMatch = call.request.headers["If-Match"]
                    ?: return@post call.respond(
                        HttpStatusCode.BadRequest,
                        mapOf("error" to "If-Match header is required"),
                    )
                val req = call.receive<CreateCommentRequest>()
                val mutationService = workspace.mutationService()

                when (val result = mutationService.addComment(req, ifMatch)) {
                    is MutationResult.Success -> {
                        call.response.header("ETag", result.newEtag)
                        call.respond(HttpStatusCode.OK, mapOf("anchor_id" to result.anchorId))
                    }
                    is MutationResult.EtagMismatch -> {
                        call.response.header("ETag", result.actual)
                        call.respond(
                            HttpStatusCode.PreconditionFailed,
                            EtagMismatchError(expected = result.expected, actual = result.actual),
                        )
                    }
                    is MutationResult.ValidationError -> {
                        call.respond(
                            HttpStatusCode.UnprocessableEntity,
                            mapOf("errors" to result.errors),
                        )
                    }
                    is MutationResult.NotFound -> {
                        call.respond(HttpStatusCode.NotFound, mapOf("error" to result.message))
                    }
                }
            }
        }

        route("/api/v1/comments/{anchorId}/state") {
            patch {
                val ifMatch = call.request.headers["If-Match"]
                    ?: return@patch call.respond(
                        HttpStatusCode.BadRequest,
                        mapOf("error" to "If-Match header is required"),
                    )
                val anchorId = call.parameters["anchorId"]
                    ?: return@patch call.respond(
                        HttpStatusCode.BadRequest,
                        mapOf("error" to "anchorId path parameter is required"),
                    )
                val req = call.receive<UpdateCommentStateRequest>()
                val mutationService = workspace.mutationService()

                when (val result = mutationService.updateState(anchorId, req, ifMatch)) {
                    is MutationResult.Success -> {
                        call.response.header("ETag", result.newEtag)
                        call.respond(HttpStatusCode.OK, mapOf("anchor_id" to anchorId))
                    }
                    is MutationResult.EtagMismatch -> {
                        call.response.header("ETag", result.actual)
                        call.respond(
                            HttpStatusCode.PreconditionFailed,
                            EtagMismatchError(expected = result.expected, actual = result.actual),
                        )
                    }
                    is MutationResult.ValidationError -> {
                        call.respond(
                            HttpStatusCode.UnprocessableEntity,
                            mapOf("errors" to result.errors),
                        )
                    }
                    is MutationResult.NotFound -> {
                        call.respond(HttpStatusCode.NotFound, mapOf("error" to result.message))
                    }
                }
            }
        }

        route("/api/v1/review-comment") {
            put {
                val ifMatch = call.request.headers["If-Match"]
                    ?: return@put call.respond(
                        HttpStatusCode.BadRequest,
                        mapOf("error" to "If-Match header is required"),
                    )
                val req = call.receive<UpdateReviewCommentRequest>()
                val mutationService = workspace.mutationService()

                when (val result = mutationService.updateReviewComment(req, ifMatch)) {
                    is MutationResult.Success -> {
                        call.response.header("ETag", result.newEtag)
                        call.respond(HttpStatusCode.OK, mapOf("ok" to true))
                    }
                    is MutationResult.EtagMismatch -> {
                        call.response.header("ETag", result.actual)
                        call.respond(
                            HttpStatusCode.PreconditionFailed,
                            EtagMismatchError(expected = result.expected, actual = result.actual),
                        )
                    }
                    is MutationResult.ValidationError -> {
                        call.respond(
                            HttpStatusCode.UnprocessableEntity,
                            mapOf("errors" to result.errors),
                        )
                    }
                    is MutationResult.NotFound -> {
                        call.respond(HttpStatusCode.NotFound, mapOf("error" to result.message))
                    }
                }
            }
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

    /**
     * Return a [ReviewMutationService] scoped to the current branch's review path.
     *
     * The service is created once per branch slug and cached. This ensures the
     * path-scoped [kotlinx.coroutines.sync.Mutex] inside [ReviewMutationService]
     * is shared across concurrent requests on the same branch, which is the
     * foundation of the optimistic-concurrency guarantee.
     *
     * When the branch changes (e.g. a worktree session that switches branches),
     * a new service is created and the old one is discarded. The old per-path
     * Mutex is discarded with it; that is safe because the old path is no longer
     * being written to.
     */
    private val mutationServiceLock = Any()
    @Volatile private var _cachedMutationService: Pair<String, ReviewMutationService>? = null

    fun mutationService(): ReviewMutationService {
        val git = Git(workdir)
        val repoRoot = git.repoRoot()
        val rawBranch = git.currentBranch()
        val headSha = git.headSHA()
        val branch = normalizeBranch(rawBranch, headSha)
        // Fast path: no lock needed when the cached entry matches.
        val cached = _cachedMutationService
        if (cached != null && cached.first == branch) return cached.second
        // Slow path: create a new service under a lock so concurrent callers
        // share the same instance (and its path-scoped Mutex pool).
        return synchronized(mutationServiceLock) {
            val rechecked = _cachedMutationService
            if (rechecked != null && rechecked.first == branch) {
                rechecked.second
            } else {
                val paths = SitatamePaths.resolve(repoRoot, branch)
                val service = ReviewMutationService(paths)
                _cachedMutationService = branch to service
                service
            }
        }
    }
}
