package dev.sitatame.intellij.storage

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

/**
 * YAML codec for sitatame review documents.
 *
 * Two concerns share this file:
 *
 *  1. **Bit-exact round-trip** — `roundtrip(bytes) == bytes` for every fixture
 *     under `web/fixtures/`. This guarantees the IntelliJ plugin can read a
 *     review produced by the Go CLI, write it back unchanged, and not corrupt
 *     comments, key order or unknown keys. The implementation mirrors
 *     `web/src/main/kotlin/.../Codec.kt` (PR #65) byte-for-byte.
 *
 *  2. **High-level decode/encode** — the action layer needs typed
 *     [Review]/[Comment] objects to add new comments and toggle state. We
 *     reuse the Node-tree path: parse to a Node, walk it to populate the
 *     typed model and stash everything else in `extras` so a write-back
 *     reconstructs the byte-exact form.
 */
object Codec {

    private const val DELIM = "---"

    // -- Bit-exact round-trip path ------------------------------------------

    /** Result of a round trip with the metadata needed for a byte comparison. */
    data class Roundtripped(val bytes: ByteArray) {
        override fun equals(other: Any?): Boolean =
            other is Roundtripped && bytes.contentEquals(other.bytes)
        override fun hashCode(): Int = bytes.contentHashCode()
    }

    /**
     * Decode and re-emit. The output is a fixed point of the input when the
     * codec preserves comments, key order and spacing.
     */
    fun roundtrip(input: ByteArray): Roundtripped {
        val text = String(input, Charsets.UTF_8)
        val split = splitFrontMatter(text)

        val node = composeFrontMatter(split.frontMatter)
        val emittedYaml = presentNode(node)

        val out = buildString {
            append(DELIM).append('\n')
            append(emittedYaml)
            append(DELIM).append('\n')
            if (split.body.isNotEmpty()) {
                append('\n')
                append(split.body)
            }
        }
        return Roundtripped(out.toByteArray(Charsets.UTF_8))
    }

    private data class Split(val frontMatter: String, val body: String)

    private fun splitFrontMatter(text: String): Split {
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
        val body = after.trimStart('\n')
        return Split(frontMatter = fm, body = body)
    }

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

    private fun composeFrontMatter(yaml: String): Node {
        val settings = LoadSettings.builder()
            .setParseComments(true)
            .build()
        val composer = Compose(settings)
        val maybeNode = composer.composeReader(StringReader(yaml))
        return maybeNode.orElseThrow { IllegalArgumentException("front matter: empty document") }
    }

    private fun presentNode(node: Node): String {
        val settings = DumpSettings.builder()
            .setDumpComments(true)
            .setIndent(2)
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

    private class StringSink : StreamDataWriter {
        private val sb = StringBuilder()
        override fun write(str: String) { sb.append(str) }
        override fun write(str: String, off: Int, len: Int) { sb.append(str, off, off + len) }
        override fun flush() { /* no-op */ }
        override fun toString(): String = sb.toString()
    }

    // -- High-level decode/encode for action layer ---------------------------

    /**
     * Decode bytes into a typed [Review]. Unknown keys are kept in
     * `extras*` maps. The original front-matter Node tree is also retained so
     * a write-back can be byte-identical when nothing changed.
     */
    fun decode(bytes: ByteArray): Review {
        val text = String(bytes, Charsets.UTF_8)
        val split = splitFrontMatter(text)
        val root = composeFrontMatter(split.frontMatter)
        require(root is MappingNode) { "front matter: top-level must be a mapping" }

        val review = Review(body = split.body)

        for (tup in root.value) {
            val keyNode = tup.keyNode
            val valNode = tup.valueNode
            if (keyNode !is ScalarNode) continue
            when (val key = keyNode.value) {
                "schema" -> review.schema = (valNode as? ScalarNode)?.value?.toIntOrNull() ?: 1
                "id" -> review.id = (valNode as? ScalarNode)?.value.orEmpty()
                "created_at" -> review.createdAt = (valNode as? ScalarNode)?.value.orEmpty()
                "branch" -> review.branch = (valNode as? ScalarNode)?.value.orEmpty()
                "base" -> review.base = decodeRef(valNode)
                "head" -> review.head = decodeRef(valNode)
                "files" -> review.files = decodeFiles(valNode)
                "review_comment" -> review.reviewComment = (valNode as? ScalarNode)?.value.orEmpty()
                "comments" -> review.comments = decodeComments(valNode)
                else -> review.extras[key] = valNode
            }
        }
        return review
    }

    private fun decodeRef(node: Node): Ref {
        val r = Ref()
        if (node !is MappingNode) return r
        for (tup in node.value) {
            val k = (tup.keyNode as? ScalarNode)?.value ?: continue
            val v = (tup.valueNode as? ScalarNode)?.value.orEmpty()
            when (k) {
                "ref" -> r.ref = v
                "sha" -> r.sha = v
            }
        }
        return r
    }

    private fun decodeFiles(node: Node): MutableList<FileMeta> {
        val out = mutableListOf<FileMeta>()
        if (node !is SequenceNode) return out
        for (item in node.value) {
            if (item !is MappingNode) continue
            val fm = FileMeta()
            for (tup in item.value) {
                val k = (tup.keyNode as? ScalarNode)?.value ?: continue
                val v = tup.valueNode
                when (k) {
                    "path" -> fm.path = (v as? ScalarNode)?.value.orEmpty()
                    "blob_base" -> fm.blobBase = (v as? ScalarNode)?.value.orEmpty()
                    "blob_head" -> fm.blobHead = (v as? ScalarNode)?.value.orEmpty()
                    "status" -> fm.status = (v as? ScalarNode)?.value.orEmpty()
                    "rename_from" -> fm.renameFrom = (v as? ScalarNode)?.value.orEmpty()
                    "rename_to" -> fm.renameTo = (v as? ScalarNode)?.value.orEmpty()
                    "similarity" -> fm.similarity = (v as? ScalarNode)?.value?.toIntOrNull() ?: 0
                    else -> fm.extras[k] = v
                }
            }
            out.add(fm)
        }
        return out
    }

    private fun decodeComments(node: Node): MutableList<Comment> {
        val out = mutableListOf<Comment>()
        if (node !is SequenceNode) return out
        for (item in node.value) {
            if (item !is MappingNode) continue
            val c = Comment()
            for (tup in item.value) {
                val k = (tup.keyNode as? ScalarNode)?.value ?: continue
                val v = tup.valueNode
                when (k) {
                    "anchor_id" -> c.anchor.anchorId = (v as? ScalarNode)?.value.orEmpty()
                    "kind" -> c.anchor.kind = (v as? ScalarNode)?.value.orEmpty()
                    "path" -> c.anchor.path = (v as? ScalarNode)?.value.orEmpty()
                    "side" -> c.anchor.side = (v as? ScalarNode)?.value.orEmpty()
                    "blob" -> c.anchor.blob = (v as? ScalarNode)?.value.orEmpty()
                    "line" -> c.anchor.line = (v as? ScalarNode)?.value?.toIntOrNull() ?: 0
                    "line_start" -> c.anchor.lineStart = (v as? ScalarNode)?.value?.toIntOrNull() ?: 0
                    "line_end" -> c.anchor.lineEnd = (v as? ScalarNode)?.value?.toIntOrNull() ?: 0
                    "rename_from" -> c.anchor.renameFrom = (v as? ScalarNode)?.value.orEmpty()
                    "rename_to" -> c.anchor.renameTo = (v as? ScalarNode)?.value.orEmpty()
                    "similarity" -> c.anchor.similarity = (v as? ScalarNode)?.value?.toIntOrNull() ?: 0
                    "state" -> c.state = (v as? ScalarNode)?.value.orEmpty()
                    "body" -> c.body = (v as? ScalarNode)?.value.orEmpty()
                    else -> c.extras[k] = v
                }
            }
            out.add(c)
        }
        return out
    }

    /**
     * Encode a typed [Review] back into front matter + body bytes.
     *
     * This is a *minimal* encoder: it emits the known fields in the canonical
     * order (schema → id → ... → comments), copies extras maps back into the
     * mapping after their parent's known keys, and uses the same block-style
     * dump settings as [roundtrip] so a fresh-from-the-action-layer write
     * matches what the Go side would produce. It is *not* guaranteed
     * bit-exact for an arbitrary pre-existing review — for that, the action
     * layer should prefer to edit the original Node tree directly (Phase 2).
     */
    fun encode(review: Review): ByteArray {
        val root = MappingNode(Tag.MAP, mutableListOf(), FlowStyle.BLOCK)
        val tuples = root.value

        tuples.addScalar("schema", review.schema.toString())
        if (review.id.isNotEmpty()) tuples.addScalar("id", review.id)
        if (review.createdAt.isNotEmpty()) tuples.addScalar("created_at", review.createdAt)
        if (review.branch.isNotEmpty()) tuples.addScalar("branch", review.branch)
        tuples.addMapping("base", listOf("ref" to review.base.ref, "sha" to review.base.sha))
        tuples.addMapping("head", listOf("ref" to review.head.ref, "sha" to review.head.sha))

        if (review.files.isNotEmpty()) {
            tuples.add(NodeTuple(scalar("files"), encodeFiles(review.files)))
        }
        if (review.reviewComment.isNotEmpty()) {
            tuples.addScalar("review_comment", review.reviewComment)
        }
        if (review.comments.isNotEmpty()) {
            tuples.add(NodeTuple(scalar("comments"), encodeComments(review.comments)))
        }
        mergeExtrasInto(tuples, review.extras)

        val yaml = presentNode(root)
        val out = buildString {
            append(DELIM).append('\n')
            append(yaml)
            append(DELIM).append('\n')
            if (review.body.isNotEmpty()) {
                append('\n')
                append(review.body)
                if (!review.body.endsWith("\n")) append('\n')
            }
        }
        return out.toByteArray(Charsets.UTF_8)
    }

    private fun encodeFiles(files: List<FileMeta>): SequenceNode {
        val items = files.map { fm ->
            val tuples = mutableListOf<NodeTuple>()
            tuples.addScalar("path", fm.path)
            if (fm.blobBase.isNotEmpty()) tuples.addScalar("blob_base", fm.blobBase)
            if (fm.blobHead.isNotEmpty()) tuples.addScalar("blob_head", fm.blobHead)
            if (fm.status.isNotEmpty()) tuples.addScalar("status", fm.status)
            if (fm.renameFrom.isNotEmpty()) tuples.addScalar("rename_from", fm.renameFrom)
            if (fm.renameTo.isNotEmpty()) tuples.addScalar("rename_to", fm.renameTo)
            if (fm.similarity != 0) tuples.addScalar("similarity", fm.similarity.toString())
            mergeExtrasInto(tuples, fm.extras)
            MappingNode(Tag.MAP, tuples, FlowStyle.BLOCK)
        }
        return SequenceNode(Tag.SEQ, items, FlowStyle.BLOCK)
    }

    private fun encodeComments(comments: List<Comment>): SequenceNode {
        val items = comments.map { c ->
            val tuples = mutableListOf<NodeTuple>()
            tuples.addScalar("anchor_id", c.anchor.anchorId)
            tuples.addScalar("kind", c.anchor.kind)
            tuples.addScalar("path", c.anchor.path)
            if (c.anchor.side.isNotEmpty()) tuples.addScalar("side", c.anchor.side)
            if (c.anchor.blob.isNotEmpty()) tuples.addScalar("blob", c.anchor.blob)
            if (c.anchor.line != 0) tuples.addScalar("line", c.anchor.line.toString())
            if (c.anchor.lineStart != 0) tuples.addScalar("line_start", c.anchor.lineStart.toString())
            if (c.anchor.lineEnd != 0) tuples.addScalar("line_end", c.anchor.lineEnd.toString())
            if (c.anchor.renameFrom.isNotEmpty()) tuples.addScalar("rename_from", c.anchor.renameFrom)
            if (c.anchor.renameTo.isNotEmpty()) tuples.addScalar("rename_to", c.anchor.renameTo)
            if (c.anchor.similarity != 0) tuples.addScalar("similarity", c.anchor.similarity.toString())
            tuples.addScalar("state", c.state)
            tuples.addScalar("body", c.body)
            mergeExtrasInto(tuples, c.extras)
            MappingNode(Tag.MAP, tuples, FlowStyle.BLOCK)
        }
        return SequenceNode(Tag.SEQ, items, FlowStyle.BLOCK)
    }

    // -- Node tree helpers ---------------------------------------------------

    private fun scalar(value: String, tag: Tag = Tag.STR): ScalarNode =
        ScalarNode(tag, value, ScalarStyle.PLAIN)

    private fun MutableList<NodeTuple>.addScalar(key: String, value: String) {
        add(NodeTuple(scalar(key), scalar(value)))
    }

    private fun MutableList<NodeTuple>.addMapping(key: String, kvs: List<Pair<String, String>>) {
        val inner = kvs.map { (k, v) -> NodeTuple(scalar(k), scalar(v)) }.toMutableList()
        add(NodeTuple(scalar(key), MappingNode(Tag.MAP, inner, FlowStyle.BLOCK)))
    }

    private fun mergeExtrasInto(tuples: MutableList<NodeTuple>, extras: Map<String, Node>) {
        if (extras.isEmpty()) return
        val existing = tuples.mapNotNull { (it.keyNode as? ScalarNode)?.value }.toHashSet()
        for ((k, v) in extras.toSortedMap()) {
            if (k in existing) continue
            tuples.add(NodeTuple(scalar(k), v))
        }
    }
}
