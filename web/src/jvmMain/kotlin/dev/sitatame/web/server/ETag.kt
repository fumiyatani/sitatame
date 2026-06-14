package dev.sitatame.web.server

import java.security.MessageDigest

/**
 * ETag computation for review.md.
 *
 * The ETag is the SHA-256 hex digest of the raw bytes of the file. An
 * absent / empty review.md is represented by the sentinel string "empty".
 *
 * Format: `"sha256-<64 lowercase hex chars>"` (with surrounding double-quotes
 * as required by the HTTP ETag wire format) or `"empty"` for the initial state.
 *
 * The surrounding quotes are included so callers can set the header directly:
 *
 *     call.response.header("ETag", computeEtag(bytes))
 *
 * and the client can echo them back in `If-Match` unchanged.
 */
fun computeEtag(bytes: ByteArray?): String {
    if (bytes == null || bytes.isEmpty()) return "\"empty\""
    val digest = MessageDigest.getInstance("SHA-256").digest(bytes)
    val hex = digest.joinToString("") { "%02x".format(it) }
    return "\"sha256-$hex\""
}
