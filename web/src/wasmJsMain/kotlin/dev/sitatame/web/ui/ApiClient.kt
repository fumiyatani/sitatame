package dev.sitatame.web.ui

import dev.sitatame.web.api.WorkspaceResponse
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.json.Json
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * Minimal browser-fetch wrapper for the wasmJs target.
 *
 * Why not ktor-client-js?
 *   As of Ktor 3.0.x, the JS-engine artifact does not publish a wasmJs
 *   variant; pulling it in causes resolution failures. The Web UI only needs
 *   a GET + a text-body fetch for Phase 1 step 1, so we use the browser
 *   `fetch` API directly via JS interop. When ktor's wasmJs client is
 *   available, swap this out.
 */

/**
 * JavaScript glue: fetches [url] and invokes the success callback with the
 * response body text, or the error callback with the message string.
 *
 * The two callbacks shape avoids exposing `Promise<JsString>` to Kotlin, which
 * requires the coroutines wasm-js await extension — that extension is
 * available in 1.8+ but the package layout differs across versions. The
 * callback shape works on every stable wasm-js coroutines version.
 */
@JsFun(
    """
    (url, ok, err) => {
        fetch(url)
            .then(r => {
                if (!r.ok) {
                    err('HTTP ' + r.status + ' ' + r.statusText);
                    return;
                }
                return r.text().then(text => ok(text));
            })
            .catch(e => err(String(e)));
    }
    """
)
private external fun jsFetch(url: String, ok: (String) -> Unit, err: (String) -> Unit)

private val json = Json { ignoreUnknownKeys = true }

suspend fun fetchWorkspace(): WorkspaceResponse {
    val text = suspendCancellableCoroutine { cont ->
        jsFetch(
            url = "/api/v1/workspace",
            ok = { body -> cont.resume(body) },
            err = { msg -> cont.resumeWithException(RuntimeException(msg)) },
        )
    }
    return json.decodeFromString(WorkspaceResponse.serializer(), text)
}
