package dev.sitatame.web.ui

import dev.sitatame.web.api.CreateCommentRequest
import dev.sitatame.web.api.EtagMismatchError
import dev.sitatame.web.api.UpdateCommentStateRequest
import dev.sitatame.web.api.UpdateReviewCommentRequest
import dev.sitatame.web.api.WorkspaceResponse
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.SerializationException
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * Minimal browser-fetch wrapper for the wasmJs target.
 *
 * Why not ktor-client-js?
 *   As of Ktor 3.0.x, the JS-engine artifact does not publish a wasmJs
 *   variant; pulling it in causes resolution failures. The Web UI only needs
 *   fetch + JSON round-trip, so we use the browser `fetch` API directly via
 *   JS interop. When ktor's wasmJs client is available, swap this out.
 */

// ---------------------------------------------------------------------------
// JS interop primitives
// ---------------------------------------------------------------------------

/**
 * Callback result from a JS fetch call — carries the HTTP status, the response
 * body text, and the ETag header (or empty string when absent).
 */
data class JsFetchResult(
    val status: Int,
    val body: String,
    val etag: String,
)

/**
 * GET-only JS glue (kept for backward-compat with the original workspace call).
 */
@JsFun(
    """
    (url, ok, err) => {
        fetch(url)
            .then(r => {
                const etag = r.headers.get('ETag') || '';
                if (!r.ok) {
                    err('HTTP ' + r.status + ' ' + r.statusText);
                    return;
                }
                return r.text().then(text => ok(r.status, text, etag));
            })
            .catch(e => err(String(e)));
    }
    """
)
private external fun jsFetchGet(
    url: String,
    ok: (Int, String, String) -> Unit,
    err: (String) -> Unit,
)

/**
 * Mutation JS glue: supports POST / PATCH / PUT with a JSON body and the
 * optional `If-Match` header.
 *
 * The [ifMatch] parameter should be the sentinel value `"empty"` for the
 * initial workspace state (no review.md yet) or the current ETag returned by
 * the server.  The server treats a missing `If-Match` header as an error (400),
 * so we always send it.
 *
 * The [ok] callback receives (status, body, etag) even for error statuses (412,
 * 422) so the Kotlin side can decode the error body without losing the current
 * ETag.
 */
@JsFun(
    """
    (method, url, body, ifMatch, ok, err) => {
        const headers = {
            'Content-Type': 'application/json',
            'If-Match': ifMatch,
        };
        fetch(url, { method: method, headers: headers, body: body })
            .then(r => {
                const etag = r.headers.get('ETag') || '';
                return r.text().then(text => ok(r.status, text, etag));
            })
            .catch(e => err(String(e)));
    }
    """
)
private external fun jsFetchMutate(
    method: String,
    url: String,
    body: String,
    ifMatch: String,
    ok: (Int, String, String) -> Unit,
    err: (String) -> Unit,
)

// ---------------------------------------------------------------------------
// Sealed result types
// ---------------------------------------------------------------------------

/** Carries a decoded response together with the server-returned ETag. */
data class EtaggedResponse<T>(val value: T, val etag: String)

/** All outcomes that a mutation call can produce. */
sealed interface MutationResult<out T> {
    data class Success<T>(val response: EtaggedResponse<T>) : MutationResult<T>
    data class EtagMismatch(val error: EtagMismatchError, val currentEtag: String) : MutationResult<Nothing>
    data class ValidationError(val errors: String) : MutationResult<Nothing>
    data class Unexpected(val status: Int, val body: String) : MutationResult<Nothing>
}

// ---------------------------------------------------------------------------
// JSON codec (shared, lenient)
// ---------------------------------------------------------------------------

private val json = Json { ignoreUnknownKeys = true }

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * GET /api/v1/workspace.
 *
 * Returns the parsed [WorkspaceResponse] together with the ETag header the
 * server sent.  The ETag is `"empty"` when no review.md exists yet (sentinel
 * defined in Phase A backend).
 */
suspend fun fetchWorkspace(): EtaggedResponse<WorkspaceResponse> {
    val (status, body, etag) = suspendGet("/api/v1/workspace")
    if (status !in 200..299) {
        throw RuntimeException("HTTP $status")
    }
    val ws = json.decodeFromString(WorkspaceResponse.serializer(), body)
    return EtaggedResponse(ws, etag.ifBlank { "empty" })
}

/**
 * POST /api/v1/comments — create a new inline or review-level comment.
 *
 * [ifMatch] must be the current ETag (or `"empty"` for the very first write).
 */
suspend fun postComment(
    request: CreateCommentRequest,
    ifMatch: String,
): MutationResult<Map<String, String>> {
    val body = json.encodeToString(request)
    val (status, resBody, etag) = suspendMutate("POST", "/api/v1/comments", body, ifMatch)
    return parseMutationResult(status, resBody, etag) { rb ->
        @Suppress("UNCHECKED_CAST")
        json.decodeFromString<Map<String, String>>(rb)
    }
}

/**
 * PATCH /api/v1/comments/{anchorId}/state — toggle open ↔ resolved.
 */
suspend fun patchCommentState(
    anchorId: String,
    request: UpdateCommentStateRequest,
    ifMatch: String,
): MutationResult<Unit> {
    val body = json.encodeToString(request)
    val (status, resBody, etag) =
        suspendMutate("PATCH", "/api/v1/comments/$anchorId/state", body, ifMatch)
    return parseMutationResult(status, resBody, etag) { }
}

/**
 * PUT /api/v1/review-comment — replace the overall review comment text.
 */
suspend fun putReviewComment(
    request: UpdateReviewCommentRequest,
    ifMatch: String,
): MutationResult<Unit> {
    val body = json.encodeToString(request)
    val (status, resBody, etag) =
        suspendMutate("PUT", "/api/v1/review-comment", body, ifMatch)
    return parseMutationResult(status, resBody, etag) { }
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

private suspend fun suspendGet(url: String): JsFetchResult =
    suspendCancellableCoroutine { cont ->
        jsFetchGet(
            url = url,
            ok = { status, body, etag -> cont.resume(JsFetchResult(status, body, etag)) },
            err = { msg -> cont.resumeWithException(RuntimeException(msg)) },
        )
    }

private suspend fun suspendMutate(
    method: String,
    url: String,
    body: String,
    ifMatch: String,
): JsFetchResult =
    suspendCancellableCoroutine { cont ->
        jsFetchMutate(
            method = method,
            url = url,
            body = body,
            ifMatch = ifMatch,
            ok = { status, resBody, etag -> cont.resume(JsFetchResult(status, resBody, etag)) },
            err = { msg -> cont.resumeWithException(RuntimeException(msg)) },
        )
    }

private fun <T> parseMutationResult(
    status: Int,
    body: String,
    etag: String,
    decode: (String) -> T,
): MutationResult<T> = when (status) {
    in 200..299 -> {
        val value = decode(body)
        MutationResult.Success(EtaggedResponse(value, etag.ifBlank { "empty" }))
    }
    412 -> {
        val err = try {
            json.decodeFromString(EtagMismatchError.serializer(), body)
        } catch (_: SerializationException) {
            EtagMismatchError(expected = "", actual = etag)
        }
        MutationResult.EtagMismatch(err, etag)
    }
    422 -> MutationResult.ValidationError(body)
    else -> MutationResult.Unexpected(status, body)
}
