package dev.sitatame.intellij.storage

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.fail
import org.junit.Test
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths

/**
 * Round-trip the same fixtures the Web UI PoC (PR #65) verifies, so the
 * IntelliJ codec and the Web UI codec stay aligned. Any divergence here
 * indicates that the IntelliJ plugin would corrupt comments, key order or
 * unknown keys on save — which would break the cross-route contract.
 */
class CodecTest {

    @Test
    fun roundtripMinimal() = assertRoundtrip("minimal.yaml")

    @Test
    fun roundtripUnknownTop() = assertRoundtrip("unknown-top.yaml")

    @Test
    fun roundtripUnknownComment() = assertRoundtrip("unknown-comment.yaml")

    @Test
    fun roundtripWithYamlComments() = assertRoundtrip("with-yaml-comments.yaml")

    @Test
    fun roundtripArrayOrder() = assertRoundtrip("array-order.yaml")

    @Test
    fun decodeMinimalProducesTypedReview() {
        val bytes = Files.readAllBytes(resolveFixtureDir().resolve("minimal.yaml"))
        val review = Codec.decode(bytes)
        assertEquals(1, review.schema)
        assertEquals("20260501T100000-minimal", review.id)
        assertEquals("feature/minimal", review.branch)
        assertEquals(1, review.comments.size)
        val c = review.comments[0]
        assertEquals("11111111-1111-1111-1111-111111111111", c.anchor.anchorId)
        assertEquals(AnchorKind.LINE, c.anchor.kind)
        assertEquals("src/app.go", c.anchor.path)
        assertEquals(10, c.anchor.line)
        assertEquals(ReviewState.OPEN, c.state)
        assertEquals("please rename this variable.", c.body)
    }

    private fun assertRoundtrip(fixtureName: String) {
        val path = resolveFixtureDir().resolve(fixtureName)
        assertNotNull("fixture $fixtureName not found at $path", path)
        val expected = Files.readAllBytes(path)
        val actual = Codec.roundtrip(expected).bytes
        if (!expected.contentEquals(actual)) {
            fail(
                "round-trip diverged for $fixtureName:\n" +
                    "--- expected ---\n${String(expected, Charsets.UTF_8)}\n" +
                    "--- actual ---\n${String(actual, Charsets.UTF_8)}",
            )
        }
    }

    private fun resolveFixtureDir(): Path {
        // Gradle runs tests with the intellij/ subproject dir as cwd. The
        // Go-generated fixtures live under <repo>/web/fixtures, so walk up.
        val candidates = listOf(
            Paths.get("..", "web", "fixtures"),
            Paths.get("web", "fixtures"),
            Paths.get(System.getProperty("user.dir"), "..", "web", "fixtures"),
        )
        for (c in candidates) {
            if (Files.isDirectory(c)) return c.toAbsolutePath().normalize()
        }
        error("could not locate web/fixtures; tried: $candidates")
    }
}
