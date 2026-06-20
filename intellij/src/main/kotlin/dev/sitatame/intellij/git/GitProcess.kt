package dev.sitatame.intellij.git

import java.util.concurrent.FutureTask
import java.util.concurrent.TimeUnit

/**
 * Runs short-lived git commands with a hard timeout that holds even when the
 * process wedges without closing its output.
 *
 * stdout is read on a daemon thread so a git process that never reaches EOF
 * can't outlast the 5-second [Process.waitFor] timeout — `readText()` blocks
 * until EOF, so reading inline would defeat the timeout. stderr is merged into
 * stdout to avoid pipe-buffer deadlocks.
 *
 * Intentionally avoids Git4Idea APIs so callers can be exercised in plain JUnit
 * tests without the IntelliJ Platform. MUST be called off the EDT — it spawns a
 * subprocess.
 */
internal object GitProcess {

    private const val TIMEOUT_SECONDS = 5L

    /** Run [args] in [repoRoot]; returns trimmed, non-empty stdout lines, or empty on any failure. */
    fun run(repoRoot: String, vararg args: String): List<String> {
        if (repoRoot.isEmpty()) return emptyList()
        var proc: Process? = null
        return try {
            val p = ProcessBuilder(*args)
                .directory(java.io.File(repoRoot))
                .redirectErrorStream(true)
                .start()
            proc = p
            val reader = FutureTask { p.inputStream.bufferedReader().readText() }
            Thread(reader, "sitatame-git-reader").apply { isDaemon = true }.start()

            if (!p.waitFor(TIMEOUT_SECONDS, TimeUnit.SECONDS)) {
                p.destroyForcibly()
                reader.cancel(true)
                return emptyList()
            }
            if (p.exitValue() != 0) return emptyList()

            // Process exited → stdout is at EOF → the read has completed; the
            // short get() timeout is just a safety net against a stuck reader.
            reader.get(1, TimeUnit.SECONDS)
                .lineSequence()
                .map { it.trim() }
                .filter { it.isNotEmpty() }
                .toList()
        } catch (_: Exception) {
            proc?.destroyForcibly()
            emptyList()
        }
    }
}
