package dev.sitatame.web.roundtrip

import dev.sitatame.web.api.CreateCommentRequest
import org.snakeyaml.engine.v2.api.DumpSettings
import org.snakeyaml.engine.v2.api.LoadSettings
import org.snakeyaml.engine.v2.api.StreamDataWriter
import org.snakeyaml.engine.v2.api.lowlevel.Compose
import org.snakeyaml.engine.v2.common.FlowStyle
import org.snakeyaml.engine.v2.common.ScalarStyle
import org.snakeyaml.engine.v2.emitter.Emitter
import org.snakeyaml.engine.v2.nodes.MappingNode
import org.snakeyaml.engine.v2.nodes.Node
import org.snakeyaml.engine.v2.nodes.NodeTuple
import org.snakeyaml.engine.v2.nodes.ScalarNode
import org.snakeyaml.engine.v2.nodes.SequenceNode
import org.snakeyaml.engine.v2.nodes.Tag
import org.snakeyaml.engine.v2.serializer.Serializer
import java.io.StringReader
import java.time.Instant
import java.time.format.DateTimeFormatter

/**
 * YAML codec for the sitatame review document.
 *
 * The review document is a `---`-delimited YAML front matter followed by a
 * Markdown body. The Go side (internal/review/codec.go) preserves unknown keys
 * in `Extras` maps so future schema additions survive a decode/encode round
 * trip. For the Web UI route to be viable, the Kotlin side must reach the same
 * bit-exact round-trip property using snakeyaml-engine's Node tree API.
 *
 * This codec implements the round trip exactly that way:
 *
 *   bytes -> splitFrontMatter -> Compose -> Node tree -> Present -> bytes
 *
 * No data class mapping happens. The whole point is to keep every key (known
 * or unknown), every comment, and every array ordering decision in the Node
 * tree as snakeyaml-engine parsed them.
 */
object Codec {

    private const val DELIM = "---"

    /** Result of a round trip with the metadata needed for a byte comparison. */
    data class Roundtripped(val bytes: ByteArray)

    /**
     * Decode the given bytes, then re-emit them. The output should be a fixed
     * point of the input when the codec preserves comments, key order and
     * spacing — that property is what the bit-exact assertion in
     * RoundtripTest verifies.
     */
    fun roundtrip(input: ByteArray): Roundtripped {
        val text = String(input, Charsets.UTF_8)
        val split = splitFrontMatter(text)

        val node = composeFrontMatter(split.frontMatter)
        val emittedYaml = presentNode(node)

        val out = buildString {
            append(DELIM).append('\n')
            append(emittedYaml)
            // Compose+Present emits a trailing newline after the document; the
            // Go side writes the closing delim on its own line directly after
            // the YAML, so do not add an extra blank line here.
            append(DELIM).append('\n')
            if (split.body.isNotEmpty()) {
                // Mirror Go: exactly one blank line between closing delim and
                // the body, body keeps its trailing newline.
                append('\n')
                append(split.body)
            }
        }
        return Roundtripped(out.toByteArray(Charsets.UTF_8))
    }

    private data class Split(val frontMatter: String, val body: String)

    /**
     * Split the document into the YAML front matter (between the two `---`
     * lines) and the Markdown body that follows. The split mirrors the Go
     * splitFrontMatter so the two sides agree on what is YAML.
     */
    private fun splitFrontMatter(text: String): Split {
        // Tolerate a leading BOM / whitespace before the opening delim, same
        // as the Go side.
        val trimmed = text.trimStart('﻿', ' ', '\t', '\r', '\n')
        require(trimmed.startsWith(DELIM)) { "missing opening \"---\" delimiter" }
        val afterOpenIdx = trimmed.indexOf('\n', DELIM.length)
        require(afterOpenIdx >= 0) { "missing newline after opening delimiter" }
        val rest = trimmed.substring(afterOpenIdx + 1)

        val closeIdx = indexOfLine(rest, DELIM)
        require(closeIdx >= 0) { "missing closing \"---\" delimiter" }

        val fm = rest.substring(0, closeIdx)
        var after = rest.substring(closeIdx + DELIM.length)
        if (after.startsWith("\n")) after = after.substring(1)
        // Drop a single leading blank line so the body matches the Go output.
        val body = after.trimStart('\n')
        return Split(frontMatter = fm, body = body)
    }

    /** Returns the offset where [marker] appears as a whole line, or -1. */
    private fun indexOfLine(s: String, marker: String): Int {
        var i = 0
        while (i < s.length) {
            val end = s.indexOf('\n', i)
            val line = if (end < 0) s.substring(i) else s.substring(i, end)
            if (line == marker) return i
            if (end < 0) return -1
            i = end + 1
        }
        return -1
    }

    /** Compose the YAML chunk into a Node tree, parsing comments along the way. */
    private fun composeFrontMatter(yaml: String): Node {
        val settings = LoadSettings.builder()
            .setParseComments(true)
            .build()
        val composer = Compose(settings)
        val maybeNode = composer.composeReader(StringReader(yaml))
        return maybeNode.orElseThrow { IllegalArgumentException("front matter: empty document") }
    }

    /**
     * Emit a Node tree using settings that match the Go writer:
     * 2-space indent, block style, plain scalars where possible, no extra
     * document delimiter (we write our own `---` lines).
     *
     * Implementation note: Serializer + Emitter are used directly instead of
     * the higher-level Present helper because Present's `emitToString` shape
     * varies across snakeyaml-engine 2.x patch releases.
     */
    private fun presentNode(node: Node): String {
        val settings = DumpSettings.builder()
            .setDumpComments(true)
            .setIndent(2)
            // Indicator indent of 2 makes block sequences render their `-`
            // marker indented under the parent key, matching Go's yaml.v3
            // default ("comments:\n  - anchor_id: ...").
            .setIndicatorIndent(2)
            .setIndentWithIndicator(true)
            .setDefaultFlowStyle(FlowStyle.BLOCK)
            .setDefaultScalarStyle(ScalarStyle.PLAIN)
            .setExplicitStart(false)
            .setExplicitEnd(false)
            .setWidth(Int.MAX_VALUE)
            .build()

        val sink = StringSink()
        val emitter = Emitter(settings, sink)
        val serializer = Serializer(settings, emitter)
        serializer.emitStreamStart()
        serializer.serializeDocument(node)
        serializer.emitStreamEnd()
        return sink.toString()
    }

    /**
     * Adapter that lets the Emitter write into a string buffer.
     * StreamDataWriter declares `write(String)`, `write(String, off, len)` and
     * `flush()`. The interface has a default flush() in 2.x but explicitly
     * overriding it avoids surprises across patch releases.
     */
    private class StringSink : StreamDataWriter {
        private val sb = StringBuilder()
        override fun write(str: String) {
            sb.append(str)
        }
        override fun write(str: String, off: Int, len: Int) {
            sb.append(str, off, off + len)
        }
        override fun flush() {
            // no-op: backing store is an in-memory StringBuilder.
        }
        override fun toString(): String = sb.toString()
    }

    // -----------------------------------------------------------------------
    // Write-path mutations (Phase 1 step 2)
    // -----------------------------------------------------------------------

    /** Holds the anchor_id assigned to the newly created comment. */
    data class AddedComment(val anchorId: String)

    /**
     * Append a new comment to the review document.
     *
     * If [input] is empty (no review.md yet), a minimal front-matter skeleton
     * is generated so the append can proceed. The resulting bytes can be saved
     * directly to disk.
     *
     * [anchorId] must be a pre-generated UUID v4 string.
     * [clockNow] is injectable for deterministic testing.
     *
     * Returns a pair of (newBytes, AddedComment).
     */
    fun addComment(
        input: ByteArray,
        request: CreateCommentRequest,
        anchorId: String,
        clockNow: () -> Instant = { Instant.now() },
    ): Pair<ByteArray, AddedComment> {
        val text = if (input.isEmpty()) buildMinimalFrontMatter(clockNow()) else String(input, Charsets.UTF_8)
        val split = splitFrontMatter(text)
        val root = composeFrontMatter(split.frontMatter)

        val mapping = root as? MappingNode
            ?: error("front matter root is not a mapping")

        // Find or create the 'comments' sequence.
        val commentsSeq = findOrCreateCommentsSeq(mapping)

        // Build the new comment mapping node.
        val commentNode = buildCommentNode(request, anchorId)
        commentsSeq.value.add(commentNode)

        val emittedYaml = presentNode(root)
        val out = reassemble(emittedYaml, split.body)
        return out.toByteArray(Charsets.UTF_8) to AddedComment(anchorId)
    }

    /**
     * Update the state of an existing comment identified by [anchorId].
     *
     * Returns new bytes; throws [IllegalArgumentException] when the anchor is
     * not found.
     */
    fun updateCommentState(input: ByteArray, anchorId: String, newState: String): ByteArray {
        val text = String(input, Charsets.UTF_8)
        val split = splitFrontMatter(text)
        val root = composeFrontMatter(split.frontMatter)

        val mapping = root as? MappingNode
            ?: error("front matter root is not a mapping")

        val commentsSeq = findCommentsSeq(mapping)
            ?: throw IllegalArgumentException("no comments sequence in review")

        val commentNode = findCommentByAnchorId(commentsSeq, anchorId)
            ?: throw IllegalArgumentException("comment not found: $anchorId")

        setScalarField(commentNode, "state", newState)

        val emittedYaml = presentNode(root)
        return reassemble(emittedYaml, split.body).toByteArray(Charsets.UTF_8)
    }

    /**
     * Replace the top-level `review_comment` field.
     *
     * If the field does not yet exist, it is appended. An empty [text] sets the
     * field to the empty string (the caller decides whether to omit the field).
     */
    fun updateReviewComment(input: ByteArray, text: String): ByteArray {
        val docText = String(input, Charsets.UTF_8)
        val split = splitFrontMatter(docText)
        val root = composeFrontMatter(split.frontMatter)

        val mapping = root as? MappingNode
            ?: error("front matter root is not a mapping")

        setOrCreateScalarField(mapping, "review_comment", text)

        val emittedYaml = presentNode(root)
        return reassemble(emittedYaml, split.body).toByteArray(Charsets.UTF_8)
    }

    // -----------------------------------------------------------------------
    // Node-tree helpers
    // -----------------------------------------------------------------------

    /**
     * Find the `comments` sequence in [mapping], or create an empty one and
     * append it. Returns the mutable [SequenceNode].
     */
    private fun findOrCreateCommentsSeq(mapping: MappingNode): SequenceNode {
        for (tuple in mapping.value) {
            val key = tuple.keyNode
            if (key is ScalarNode && key.value == "comments") {
                return tuple.valueNode as? SequenceNode
                    ?: error("'comments' field is not a sequence")
            }
        }
        // Create new empty sequence and append.
        val keyNode = scalarStr("comments")
        val seqNode = SequenceNode(Tag.SEQ, mutableListOf(), FlowStyle.BLOCK)
        mapping.value.add(NodeTuple(keyNode, seqNode))
        return seqNode
    }

    private fun findCommentsSeq(mapping: MappingNode): SequenceNode? {
        for (tuple in mapping.value) {
            val key = tuple.keyNode
            if (key is ScalarNode && key.value == "comments") {
                return tuple.valueNode as? SequenceNode
            }
        }
        return null
    }

    /**
     * Find a comment node by its `anchor_id` field. Returns null when not found.
     */
    private fun findCommentByAnchorId(seq: SequenceNode, anchorId: String): MappingNode? {
        for (node in seq.value) {
            val m = node as? MappingNode ?: continue
            for (tuple in m.value) {
                val k = tuple.keyNode as? ScalarNode ?: continue
                if (k.value == "anchor_id") {
                    val v = tuple.valueNode as? ScalarNode ?: continue
                    if (v.value == anchorId) return m
                }
            }
        }
        return null
    }

    /**
     * Set [field] on [mapping] to [value]. If the field already exists its
     * value node is replaced in-place. Otherwise it is appended.
     */
    private fun setOrCreateScalarField(mapping: MappingNode, field: String, value: String) {
        val tuples = mapping.value
        for (i in tuples.indices) {
            val key = tuples[i].keyNode as? ScalarNode ?: continue
            if (key.value == field) {
                val newTuple = NodeTuple(tuples[i].keyNode, scalarStr(value))
                tuples[i] = newTuple
                return
            }
        }
        // Not found — append.
        tuples.add(NodeTuple(scalarStr(field), scalarStr(value)))
    }

    /**
     * Set [field] in an existing comment [mapping]. Throws when the field does
     * not exist (comment nodes are expected to be well-formed).
     */
    private fun setScalarField(mapping: MappingNode, field: String, value: String) {
        val tuples = mapping.value
        for (i in tuples.indices) {
            val key = tuples[i].keyNode as? ScalarNode ?: continue
            if (key.value == field) {
                tuples[i] = NodeTuple(tuples[i].keyNode, scalarStr(value))
                return
            }
        }
        // Field missing — append (tolerate schema evolution).
        tuples.add(NodeTuple(scalarStr(field), scalarStr(value)))
    }

    /**
     * Build a new comment MappingNode from [request] and [anchorId].
     *
     * Key order follows Go's `internal/review/codec.go` Encode order:
     *   anchor_id → kind → path → side → blob → line → line_start → line_end → state → body
     */
    private fun buildCommentNode(request: CreateCommentRequest, anchorId: String): MappingNode {
        val tuples = mutableListOf<NodeTuple>()
        tuples.add(NodeTuple(scalarStr("anchor_id"), scalarStr(anchorId)))
        tuples.add(NodeTuple(scalarStr("kind"), scalarStr(request.kind)))
        if (request.path != null) {
            tuples.add(NodeTuple(scalarStr("path"), scalarStr(request.path)))
        }
        // side: always present for non-review kinds; omit for kind=review
        if (request.kind != "review") {
            tuples.add(NodeTuple(scalarStr("side"), scalarStr(request.side)))
        }
        if (request.blob != null) {
            tuples.add(NodeTuple(scalarStr("blob"), scalarStr(request.blob)))
        }
        if (request.line != null) {
            tuples.add(NodeTuple(scalarStr("line"), scalarInt(request.line)))
        }
        if (request.lineStart != null) {
            tuples.add(NodeTuple(scalarStr("line_start"), scalarInt(request.lineStart)))
        }
        if (request.lineEnd != null) {
            tuples.add(NodeTuple(scalarStr("line_end"), scalarInt(request.lineEnd)))
        }
        tuples.add(NodeTuple(scalarStr("state"), scalarStr("open")))
        tuples.add(NodeTuple(scalarStr("body"), scalarStr(request.body)))
        return MappingNode(Tag.MAP, tuples, FlowStyle.BLOCK)
    }

    /** Create a plain string ScalarNode. */
    private fun scalarStr(value: String): ScalarNode =
        ScalarNode(Tag.STR, value, ScalarStyle.PLAIN)

    /** Create a plain integer ScalarNode. */
    private fun scalarInt(value: Int): ScalarNode =
        ScalarNode(Tag.INT, value.toString(), ScalarStyle.PLAIN)

    /** Reassemble front matter YAML and Markdown body into the review.md text. */
    private fun reassemble(emittedYaml: String, body: String): String = buildString {
        append(DELIM).append('\n')
        append(emittedYaml)
        append(DELIM).append('\n')
        if (body.isNotEmpty()) {
            append('\n')
            append(body)
        }
    }

    /**
     * Generate a minimal front-matter skeleton for a new review.md. Used when
     * the first comment is written before any `sitatame run` has created the file.
     */
    private fun buildMinimalFrontMatter(now: Instant): String {
        val ts = DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmmss")
            .withZone(java.time.ZoneOffset.UTC)
            .format(now)
        val createdAt = DateTimeFormatter.ISO_INSTANT.format(now)
        return buildString {
            appendLine("---")
            appendLine("schema: 1")
            appendLine("id: $ts-web")
            appendLine("created_at: $createdAt")
            appendLine("branch: \"\"")
            appendLine("base:")
            appendLine("  ref: \"\"")
            appendLine("  sha: \"\"")
            appendLine("head:")
            appendLine("  ref: \"\"")
            appendLine("  sha: \"\"")
            appendLine("---")
        }
    }
}
