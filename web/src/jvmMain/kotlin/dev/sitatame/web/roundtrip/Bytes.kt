package dev.sitatame.web.roundtrip

/**
 * Byte-by-byte comparison helpers used by the round-trip tests.
 *
 * The Web UI route's gating requirement is bit-exact equivalence between the
 * Go-emitted fixture and the Kotlin round-trip output. When they diverge the
 * test must surface enough context to diagnose the divergence (which byte,
 * which line, which character) rather than a bare "not equal" message.
 */
object Bytes {

    /**
     * Build a human-readable diff between [expected] and [actual]. Returns
     * null when they are equal. The message points at the first differing
     * byte, prints both lines that contain it, and includes a caret marker.
     */
    fun diff(expected: ByteArray, actual: ByteArray): String? {
        if (expected.contentEquals(actual)) return null

        val firstDiff = firstDifferingIndex(expected, actual)
        val expectedText = String(expected, Charsets.UTF_8)
        val actualText = String(actual, Charsets.UTF_8)

        val sb = StringBuilder()
        sb.append("byte mismatch at offset ").append(firstDiff)
        sb.append(" (expected ").append(expected.size)
        sb.append(" bytes, actual ").append(actual.size).append(" bytes)\n")

        val (expLine, expCol) = lineColumn(expectedText, firstDiff)
        val (actLine, actCol) = lineColumn(actualText, firstDiff)
        sb.append("expected at line ").append(expLine).append(" col ").append(expCol).append(":\n")
        sb.append(formatLine(expectedText, expLine, expCol))
        sb.append("actual   at line ").append(actLine).append(" col ").append(actCol).append(":\n")
        sb.append(formatLine(actualText, actLine, actCol))

        // Full side-by-side for short fixtures so the failure mode is obvious.
        if (expected.size <= 4096 && actual.size <= 4096) {
            sb.append("--- expected ---\n").append(expectedText)
            if (!expectedText.endsWith("\n")) sb.append('\n')
            sb.append("--- actual ---\n").append(actualText)
            if (!actualText.endsWith("\n")) sb.append('\n')
        }
        return sb.toString()
    }

    private fun firstDifferingIndex(a: ByteArray, b: ByteArray): Int {
        val n = minOf(a.size, b.size)
        for (i in 0 until n) {
            if (a[i] != b[i]) return i
        }
        return n
    }

    private fun lineColumn(text: String, byteOffset: Int): Pair<Int, Int> {
        // byteOffset is a UTF-8 byte index. For diagnostic line/column we
        // approximate by walking characters; multi-byte characters in YAML
        // bodies (Japanese review text) shift this slightly but the line
        // number is still useful.
        if (byteOffset <= 0) return 1 to 1
        val bytes = text.toByteArray(Charsets.UTF_8)
        val safeOffset = minOf(byteOffset, bytes.size)
        var line = 1
        var col = 1
        for (i in 0 until safeOffset) {
            if (bytes[i] == '\n'.code.toByte()) {
                line += 1
                col = 1
            } else {
                col += 1
            }
        }
        return line to col
    }

    private fun formatLine(text: String, line: Int, col: Int): String {
        val lines = text.split('\n')
        val idx = (line - 1).coerceIn(0, lines.size - 1)
        val content = lines[idx]
        val caretPad = " ".repeat((col - 1).coerceAtLeast(0))
        return buildString {
            append("  | ").append(content).append('\n')
            append("  | ").append(caretPad).append("^\n")
        }
    }
}
