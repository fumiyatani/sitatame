package dev.sitatame.web.server

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.nio.file.Path
import java.nio.file.Paths as NioPaths

/**
 * Parity tests against `internal/review/paths.go::resolveOutputRoot` (and its
 * helper `normaliseEnvOutputRoot`). When this diverges, the Web UI and the
 * Go CLI silently look at different directories for the same SITATAME_HOME
 * value — exactly the silent-data-invisibility class of bug we want a unit
 * test to surface immediately.
 */
class PathsTest {

    private val home: Path = NioPaths.get("/home/test")

    private fun resolve(env: String?): Path {
        return SitatamePaths.resolveOutputRoot(
            envLookup = { name -> if (name == SitatamePaths.ENV_OUTPUT_ROOT) env else null },
            homeDir = home,
        )
    }

    @Test
    fun `unset env falls back to home dot sitatame`() {
        val out = resolve(null)
        assertEquals(home.resolve(".sitatame"), out)
    }

    @Test
    fun `whitespace-only env is treated as unset`() {
        // Matches Go's strings.TrimSpace: " ", "\t", "\n" all collapse to unset
        // and we end up under <home>/.sitatame.
        assertEquals(home.resolve(".sitatame"), resolve("   "))
        assertEquals(home.resolve(".sitatame"), resolve("\t"))
        assertEquals(home.resolve(".sitatame"), resolve("\n  \t"))
    }

    @Test
    fun `empty env is treated as unset`() {
        assertEquals(home.resolve(".sitatame"), resolve(""))
    }

    @Test
    fun `absolute env is used verbatim`() {
        val out = resolve("/tmp/custom")
        assertEquals(NioPaths.get("/tmp/custom"), out)
    }

    @Test
    fun `env trimmed before use`() {
        // Surrounding whitespace must not change the resolved path — Go trims
        // before hitting the abs / tilde branches.
        val out = resolve("  /tmp/custom  ")
        assertEquals(NioPaths.get("/tmp/custom"), out)
    }

    @Test
    fun `tilde slash is expanded against homeDir`() {
        val out = resolve("~/work")
        assertEquals(home.resolve("work"), out)
    }

    @Test
    fun `bare tilde resolves to homeDir`() {
        val out = resolve("~")
        assertEquals(home, out)
    }

    @Test
    fun `relative env is absolutised`() {
        val out = resolve("rel/path")
        assertTrue(out.isAbsolute, "expected absolute path, got $out")
        assertTrue(out.toString().endsWith("rel/path"), "expected suffix rel/path: $out")
        // Sanity: must differ from the literal relative input so a downstream
        // resolve() against ProjectSlug ends up under cwd, not at /rel/path.
        assertNotEquals(NioPaths.get("rel/path"), out)
    }

    @Test
    fun `tilde without homeDir leaves prefix and absolutises`() {
        // When user.home is unset Go leaves `~/foo` untouched and then
        // filepath.Abs prepends cwd. Kotlin mirrors that: the path becomes
        // <cwd>/~/foo, which is wrong on purpose — it preserves the literal
        // user intent and surfaces it through the relative-warning path.
        val out = SitatamePaths.resolveOutputRoot(
            envLookup = { if (it == SitatamePaths.ENV_OUTPUT_ROOT) "~/foo" else null },
            homeDir = null,
        )
        assertTrue(out.isAbsolute, "expected absolute path, got $out")
    }
}
