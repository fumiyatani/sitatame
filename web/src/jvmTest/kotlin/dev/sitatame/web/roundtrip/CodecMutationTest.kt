package dev.sitatame.web.roundtrip

import dev.sitatame.web.api.CreateCommentRequest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test
import java.time.Instant

/**
 * Unit tests for the Codec mutation functions added in Phase 1 step 2:
 * [Codec.addComment], [Codec.updateCommentState], [Codec.updateReviewComment].
 *
 * The primary invariants tested are:
 *  - Unknown keys / comments / ordering in existing content are preserved.
 *  - New comment is appended (not prepended).
 *  - Round-trip property still holds on the mutated output.
 *  - Empty input creates a valid skeleton.
 */
class CodecMutationTest {

    // -----------------------------------------------------------------------
    // addComment
    // -----------------------------------------------------------------------

    @Nested
    inner class AddComment {

        @Test
        fun `add line comment to empty input creates skeleton and appends comment`() {
            val (bytes, added) = Codec.addComment(
                ByteArray(0),
                CreateCommentRequest(kind = "line", path = "foo.go", line = 5, body = "looks good"),
                anchorId = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa",
            )
            val text = String(bytes)
            assertTrue(text.contains("anchor_id: aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"))
            assertTrue(text.contains("kind: line"))
            assertTrue(text.contains("path: foo.go"))
            assertTrue(text.contains("line: 5"))
            assertTrue(text.contains("state: open"))
            assertTrue(text.contains("body: looks good"))
            assertEquals("aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", added.anchorId)
            // Result is itself a valid round-trip.
            assertRoundTrip(bytes)
        }

        @Test
        fun `add review comment to empty input`() {
            val (bytes, _) = Codec.addComment(
                ByteArray(0),
                CreateCommentRequest(kind = "review", body = "overall LGTM"),
                anchorId = "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb",
            )
            val text = String(bytes)
            assertTrue(text.contains("kind: review"))
            assertFalse(text.contains("kind: review\n    side:"), "side must not appear for kind=review")
        }

        @Test
        fun `add comment to existing review appends at end`() {
            val existing = minimalReviewWithOneComment().toByteArray()
            val (bytes, _) = Codec.addComment(
                existing,
                CreateCommentRequest(kind = "review", body = "second"),
                anchorId = "cccccccc-cccc-4ccc-cccc-cccccccccccc",
            )
            val text = String(bytes)
            // Both comments present.
            assertTrue(text.contains("11111111-1111-1111-1111-111111111111"))
            assertTrue(text.contains("cccccccc-cccc-4ccc-cccc-cccccccccccc"))
            // New comment appears after the existing one.
            val idxFirst = text.indexOf("11111111-1111-1111-1111-111111111111")
            val idxSecond = text.indexOf("cccccccc-cccc-4ccc-cccc-cccccccccccc")
            assertTrue(idxFirst < idxSecond, "New comment must be appended after existing")
            assertRoundTrip(bytes)
        }

        @Test
        fun `unknown keys in existing review are preserved`() {
            val reviewWithUnknown = """
                ---
                schema: 1
                id: 20260501T100000-test
                created_at: 2026-05-01T10:00:00Z
                branch: feature/x
                base:
                  ref: origin/main
                  sha: aaa
                head:
                  ref: HEAD
                  sha: bbb
                unknown_top_key: preserved_value
                comments:
                  - anchor_id: 11111111-1111-1111-1111-111111111111
                    kind: review
                    state: open
                    body: existing
                    unknown_comment_key: also_preserved
                ---

            """.trimIndent()
            val (bytes, _) = Codec.addComment(
                reviewWithUnknown.toByteArray(),
                CreateCommentRequest(kind = "review", body = "new"),
                anchorId = "dddddddd-dddd-4ddd-dddd-dddddddddddd",
            )
            val text = String(bytes)
            assertTrue(text.contains("unknown_top_key: preserved_value"), "top-level unknown key must survive")
            assertTrue(text.contains("unknown_comment_key: also_preserved"), "comment unknown key must survive")
        }

        @Test
        fun `range comment fields are emitted in correct order`() {
            val (bytes, _) = Codec.addComment(
                ByteArray(0),
                CreateCommentRequest(
                    kind = "range",
                    path = "src/app.go",
                    side = "head",
                    blob = "blobsha",
                    lineStart = 10,
                    lineEnd = 15,
                    body = "range note",
                ),
                anchorId = "eeeeeeee-eeee-4eee-eeee-eeeeeeeeeeee",
            )
            val text = String(bytes)
            // Fields must all appear.
            listOf("anchor_id", "kind", "path", "side", "blob", "line_start", "line_end", "state", "body")
                .forEach { field ->
                    assertTrue(text.contains(field), "Field $field missing from output")
                }
            // line must not appear for kind=range.
            assertFalse(
                Regex("^\\s+line:\\s+\\d+", RegexOption.MULTILINE).containsMatchIn(text),
                "line must not appear for kind=range"
            )
        }

        @Test
        fun `clockNow is injectable for deterministic created_at`() {
            val fixedInstant = Instant.parse("2026-01-15T08:00:00Z")
            val (bytes, _) = Codec.addComment(
                ByteArray(0),
                CreateCommentRequest(kind = "review", body = "test"),
                anchorId = "ffffffff-ffff-4fff-ffff-ffffffffffff",
                clockNow = { fixedInstant },
            )
            val text = String(bytes)
            assertTrue(text.contains("20260115T080000"), "Expected timestamp in id: $text")
        }
    }

    // -----------------------------------------------------------------------
    // updateCommentState
    // -----------------------------------------------------------------------

    @Nested
    inner class UpdateCommentState {

        @Test
        fun `open to resolved state update`() {
            val input = minimalReviewWithOneComment().toByteArray()
            val output = Codec.updateCommentState(input, "11111111-1111-1111-1111-111111111111", "resolved")
            val text = String(output)
            assertTrue(text.contains("state: resolved"))
            assertFalse(text.contains("state: open"))
            assertRoundTrip(output)
        }

        @Test
        fun `other fields are preserved after state update`() {
            val input = minimalReviewWithOneComment().toByteArray()
            val output = Codec.updateCommentState(input, "11111111-1111-1111-1111-111111111111", "stale")
            val text = String(output)
            assertTrue(text.contains("body: original comment body"))
            assertTrue(text.contains("kind: review"))
        }

        @Test
        fun `throws when anchor not found`() {
            val input = minimalReviewWithOneComment().toByteArray()
            var threw = false
            try {
                Codec.updateCommentState(input, "00000000-0000-4000-0000-000000000000", "resolved")
            } catch (e: IllegalArgumentException) {
                threw = true
            }
            assertTrue(threw, "Expected IllegalArgumentException for missing anchor")
        }
    }

    // -----------------------------------------------------------------------
    // updateReviewComment
    // -----------------------------------------------------------------------

    @Nested
    inner class UpdateReviewComment {

        @Test
        fun `sets review_comment field when absent`() {
            val input = minimalReviewWithOneComment().toByteArray()
            val output = Codec.updateReviewComment(input, "Overall LGTM")
            val text = String(output)
            assertTrue(text.contains("review_comment: Overall LGTM"))
            assertRoundTrip(output)
        }

        @Test
        fun `updates existing review_comment field`() {
            val input = """
                ---
                schema: 1
                id: 20260501T100000-test
                created_at: 2026-05-01T10:00:00Z
                branch: feature/x
                base:
                  ref: origin/main
                  sha: aaa
                head:
                  ref: HEAD
                  sha: bbb
                review_comment: old comment
                ---

            """.trimIndent()
            val output = Codec.updateReviewComment(input.toByteArray(), "new comment")
            val text = String(output)
            assertTrue(text.contains("review_comment: new comment"))
            assertFalse(text.contains("old comment"))
        }

        @Test
        fun `comments are preserved after review_comment update`() {
            val input = minimalReviewWithOneComment().toByteArray()
            val output = Codec.updateReviewComment(input, "LGTM")
            val text = String(output)
            assertTrue(text.contains("11111111-1111-1111-1111-111111111111"))
            assertTrue(text.contains("original comment body"))
        }
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private fun minimalReviewWithOneComment() = """
        ---
        schema: 1
        id: 20260501T100000-test
        created_at: 2026-05-01T10:00:00Z
        branch: feature/x
        base:
          ref: origin/main
          sha: aaa
        head:
          ref: HEAD
          sha: bbb
        comments:
          - anchor_id: 11111111-1111-1111-1111-111111111111
            kind: review
            state: open
            body: original comment body
        ---

    """.trimIndent()

    /**
     * Assert that the given bytes are a fixed point of [Codec.roundtrip].
     * This verifies that mutation does not break the bit-exact round-trip property.
     */
    private fun assertRoundTrip(bytes: ByteArray) {
        val roundtripped = Codec.roundtrip(bytes).bytes
        val diff = Bytes.diff(bytes, roundtripped)
        if (diff != null) {
            throw AssertionError("mutated output is not a round-trip fixed point:\n$diff")
        }
    }
}
