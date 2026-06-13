package dev.sitatame.web.roundtrip

import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

/**
 * Bit-exact round-trip test for the Web UI YAML codec (B-route gate).
 *
 * Each fixture under `web/fixtures/` is produced by the Go side
 * (`make web-fixtures`). Decoding and re-emitting with the Kotlin codec must
 * yield the same bytes; any divergence in spacing, key order, comments or
 * unknown-key preservation breaks the schema-drift contract and triggers the
 * route's Kill criterion.
 */
class RoundtripTest {

    @TestFactory
    fun bitExactRoundtrip(): List<DynamicTest> {
        val fixtureDir = resolveFixtureDir()
        val fixtures = listOf(
            "minimal.yaml",
            "unknown-top.yaml",
            "unknown-comment.yaml",
            "with-yaml-comments.yaml",
            "array-order.yaml",
        )
        return fixtures.map { name ->
            DynamicTest.dynamicTest(name) {
                val path = fixtureDir.resolve(name)
                check(Files.exists(path)) {
                    "fixture $name is missing; run `make web-fixtures` from the repo root"
                }
                val expected = Files.readAllBytes(path)
                val actual = Codec.roundtrip(expected).bytes
                val diff = Bytes.diff(expected, actual)
                if (diff != null) {
                    fail<Unit>("round-trip diverged for $name:\n$diff")
                }
            }
        }
    }

    /**
     * Resolve the fixture directory. Gradle runs tests with the project dir
     * as the cwd; `web/fixtures` is a sibling of build.gradle.kts. When the
     * test is launched from the repo root (e.g. by an IDE), fall back to
     * `web/fixtures` relative to that.
     */
    private fun resolveFixtureDir(): Path {
        val candidates = listOf(
            Paths.get("fixtures"),
            Paths.get("web/fixtures"),
            Paths.get(System.getProperty("user.dir"), "fixtures"),
        )
        for (c in candidates) {
            if (Files.isDirectory(c)) return c.toAbsolutePath()
        }
        error("could not locate fixtures directory; tried: $candidates")
    }
}
