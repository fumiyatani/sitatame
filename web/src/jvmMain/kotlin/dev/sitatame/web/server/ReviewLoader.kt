package dev.sitatame.web.server

import dev.sitatame.web.api.CommentDto
import dev.sitatame.web.api.ReviewDto
import org.snakeyaml.engine.v2.api.Load
import org.snakeyaml.engine.v2.api.LoadSettings
import java.nio.file.Files
import java.nio.file.Path
import kotlin.io.path.isRegularFile
import kotlin.io.path.name

/**
 * Reads the most recent review .md from
 * <SitatamePaths.reviewsDir()>/<id>.md and parses the YAML front matter into a
 * [ReviewDto].
 *
 * The Go side (internal/review/codec.go) uses a Node-tree codec to preserve
 * unknown keys for round-trip. The Web UI never writes the file back through
 * Kotlin, so we use a plain `Load` here and drop unknown keys silently — the
 * frontend only needs what's modelled in [ReviewDto]. Write-path support
 * (Phase 1 step 2) will route through the existing snakeyaml-engine Node tree
 * round-trip path or, more likely, through the Go CLI shelled out from the
 * backend, to keep the schema-drift contract intact.
 */
object ReviewLoader {

    /** Returns the newest .md path under [reviewsDir], or null. */
    fun findLatestPath(reviewsDir: Path): Path? {
        if (!Files.isDirectory(reviewsDir)) return null
        return Files.list(reviewsDir).use { stream ->
            stream
                .filter { it.isRegularFile() && it.name.endsWith(".md") }
                .max(compareBy { Files.getLastModifiedTime(it).toMillis() })
                .orElse(null)
        }
    }

    /** Load [path] and parse its YAML front matter into a [ReviewDto]. */
    fun load(path: Path): ReviewDto {
        val bytes = Files.readAllBytes(path)
        val text = bytes.toString(Charsets.UTF_8)
        val frontMatter = extractFrontMatter(text)
            ?: throw IllegalArgumentException("missing YAML front matter in $path")
        val settings = LoadSettings.builder().build()
        val load = Load(settings)
        val raw = load.loadFromString(frontMatter)
            ?: throw IllegalArgumentException("empty YAML front matter in $path")
        @Suppress("UNCHECKED_CAST")
        val map = raw as? Map<String, Any?>
            ?: throw IllegalArgumentException("YAML front matter is not a mapping in $path")
        return toReviewDto(map)
    }

    private fun extractFrontMatter(text: String): String? {
        // Tolerate BOM / leading whitespace, same as Codec.
        val trimmed = text.trimStart('﻿', ' ', '\t', '\r', '\n')
        if (!trimmed.startsWith("---")) return null
        val afterOpen = trimmed.indexOf('\n', startIndex = 3)
        if (afterOpen < 0) return null
        val rest = trimmed.substring(afterOpen + 1)
        // Find closing `---` as its own line.
        var i = 0
        while (i < rest.length) {
            val end = rest.indexOf('\n', startIndex = i)
            val line = if (end < 0) rest.substring(i) else rest.substring(i, end)
            if (line == "---") return rest.substring(0, i)
            if (end < 0) return null
            i = end + 1
        }
        return null
    }

    private fun toReviewDto(map: Map<String, Any?>): ReviewDto {
        val id = (map["id"] as? String).orEmpty()
        val branch = (map["branch"] as? String).orEmpty()
        val base = map["base"] as? Map<*, *>
        val head = map["head"] as? Map<*, *>
        val baseRef = (base?.get("ref") as? String).orEmpty()
        val headRef = (head?.get("ref") as? String).orEmpty()
        val reviewComment = map["review_comment"] as? String
        val comments = (map["comments"] as? List<*>)
            ?.filterIsInstance<Map<*, *>>()
            ?.map { commentMap -> toCommentDto(commentMap) }
            ?: emptyList()
        return ReviewDto(
            id = id,
            branch = branch,
            baseRef = baseRef,
            headRef = headRef,
            reviewComment = reviewComment,
            comments = comments,
        )
    }

    private fun toCommentDto(map: Map<*, *>): CommentDto {
        return CommentDto(
            anchorId = (map["anchor_id"] as? String).orEmpty(),
            kind = (map["kind"] as? String).orEmpty(),
            path = (map["path"] as? String).orEmpty(),
            side = map["side"] as? String,
            line = (map["line"] as? Number)?.toInt(),
            lineStart = (map["line_start"] as? Number)?.toInt(),
            lineEnd = (map["line_end"] as? Number)?.toInt(),
            state = (map["state"] as? String).orEmpty(),
            body = (map["body"] as? String).orEmpty(),
        )
    }
}
