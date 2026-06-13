package dev.sitatame.web.roundtrip

import org.snakeyaml.engine.v2.api.DumpSettings
import org.snakeyaml.engine.v2.api.LoadSettings
import org.snakeyaml.engine.v2.api.StreamDataWriter
import org.snakeyaml.engine.v2.api.lowlevel.Compose
import org.snakeyaml.engine.v2.common.FlowStyle
import org.snakeyaml.engine.v2.common.ScalarStyle
import org.snakeyaml.engine.v2.emitter.Emitter
import org.snakeyaml.engine.v2.nodes.Node
import org.snakeyaml.engine.v2.serializer.Serializer
import java.io.StringReader

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
}
