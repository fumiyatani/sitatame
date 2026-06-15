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
        // blob and side must round-trip through decode
        assertEquals("bb", c.anchor.blob)
        assertEquals(AnchorSide.HEAD, c.anchor.side)
    }

    @Test
    fun decodeDeletedLineAnchorPopulatesBaseFields() {
        val bytes = Files.readAllBytes(resolveFixtureDir().resolve("deleted-line-anchor.yaml"))
        val review = Codec.decode(bytes)
        assertEquals(1, review.comments.size)
        val c = review.comments[0]
        assertEquals(AnchorKind.LINE, c.anchor.kind)
        assertEquals(AnchorSide.BASE, c.anchor.side)
        assertEquals("aa", c.anchor.blob)
        assertEquals(42, c.anchor.line)
    }

    @Test
    fun decodeLegacyMissingBlobAndSideDefaultsToEmpty() {
        // Simulate an old review that has no blob or side fields.
        // Codec must decode without error; blob defaults to "" and side to "".
        val yaml = """
            ---
            schema: 1
            id: legacy-no-blob
            created_at: 2026-01-01T00:00:00Z
            branch: legacy
            base:
              ref: origin/main
              sha: aaa
            head:
              ref: HEAD
              sha: bbb
            comments:
              - anchor_id: 00000000-0000-0000-0000-000000000001
                kind: line
                path: src/foo.go
                line: 5
                state: open
                body: legacy comment without blob or side
            ---
        """.trimIndent()
        val review = Codec.decode(yaml.toByteArray(Charsets.UTF_8))
        assertEquals(1, review.comments.size)
        val c = review.comments[0]
        // blob absent in YAML → decode to "" (backward-compat: no stale blob lookup)
        assertEquals("", c.anchor.blob)
        // side absent in YAML → Anchor default value applies ("head")
        assertEquals(AnchorSide.HEAD, c.anchor.side)
        assertEquals("src/foo.go", c.anchor.path)
        assertEquals(5, c.anchor.line)
    }

    @Test
    fun encodeBlobAndSideRoundtrip() {
        // Build a Review with blob and side set, encode → decode, assert preserved.
        val original = dev.sitatame.intellij.storage.Review(
            schema = 1,
            id = "test-blob-side",
            createdAt = "2026-06-01T00:00:00Z",
            branch = "feature/x",
            base = dev.sitatame.intellij.storage.Ref("origin/main", "aaa"),
            head = dev.sitatame.intellij.storage.Ref("HEAD", "bbb"),
        ).apply {
            comments.add(
                dev.sitatame.intellij.storage.Comment(
                    anchor = Anchor(
                        anchorId = "aaaabbbb-0000-0000-0000-000000000001",
                        kind = AnchorKind.LINE,
                        path = "src/auth.go",
                        side = AnchorSide.HEAD,
                        blob = "9c8d7e6",
                        line = 22,
                    ),
                    state = ReviewState.OPEN,
                    body = "check this",
                ),
            )
        }
        val bytes = Codec.encode(original)
        val decoded = Codec.decode(bytes)
        assertEquals(1, decoded.comments.size)
        val c = decoded.comments[0]
        assertEquals(AnchorSide.HEAD, c.anchor.side)
        assertEquals("9c8d7e6", c.anchor.blob)
        assertEquals(22, c.anchor.line)
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
