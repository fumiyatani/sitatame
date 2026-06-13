package dev.sitatame.web.roundtrip

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths
import kotlin.streams.toList

/**
 * Bit-exact round-trip test for the Web UI YAML codec (B-route gate).
 *
 * Each fixture under `web/fixtures/` is produced by the Go side
 * (`make web-fixtures`). Decoding and re-emitting with the Kotlin codec must
 * yield the same bytes; any divergence in spacing, key order, comments or
 * unknown-key preservation breaks the schema-drift contract and triggers the
 * route's Kill criterion.
 *
 * Fixtures are auto-discovered: every `*.yaml` file under `web/fixtures/` is
 * exercised. Adding a new fixture on the Go side (`cmd/yamlfixture/main.go`)
 * automatically extends Kotlin coverage on the next build. A separate
 * [fixtureCountSanity] test guards against accidental fixture deletion that
 * would silently shrink coverage.
 */
class RoundtripTest {

    @TestFactory
    fun bitExactRoundtrip(): List<DynamicTest> {
        val fixtureDir = resolveFixtureDir()
        val fixtures = discoverFixtures(fixtureDir)
        check(fixtures.isNotEmpty()) {
            "no fixtures found under $fixtureDir; run `make web-fixtures` from the repo root"
        }
        return fixtures.map { path ->
            val name = path.fileName.toString()
            DynamicTest.dynamicTest(name) {
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
     * Sanity check that catches accidental fixture deletion. The fixture set
     * is meant to keep growing as new edge cases are added; it should never
     * shrink. The threshold is set to the count established at the time the
     * fixture set was expanded for sitatame#62 (12 fixtures) so a future PR
     * that drops a fixture without thinking trips here instead of silently
     * narrowing coverage.
     */
    @Test
    fun fixtureCountSanity() {
        val fixtureDir = resolveFixtureDir()
        val fixtures = discoverFixtures(fixtureDir)
        assertTrue(fixtures.size >= MIN_FIXTURE_COUNT) {
            "fixture count regression: found ${fixtures.size} under $fixtureDir, " +
                "want >= $MIN_FIXTURE_COUNT. " +
                "If you intentionally removed a fixture, also lower MIN_FIXTURE_COUNT " +
                "with a note in the commit message explaining why coverage shrank."
        }
    }

    private fun discoverFixtures(dir: Path): List<Path> =
        Files.list(dir).use { stream ->
            stream.filter { p -> Files.isRegularFile(p) && p.fileName.toString().endsWith(".yaml") }
                .sorted()
                .toList()
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

    private companion object {
        /**
         * Lower bound for the fixture count. See [fixtureCountSanity] for the
         * rationale.
         */
        const val MIN_FIXTURE_COUNT = 12
    }
}
