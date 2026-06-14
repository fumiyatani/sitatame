package dev.sitatame.web.server

import dev.sitatame.web.api.CreateCommentRequest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test

/**
 * Unit tests for [Validation].
 *
 * Covers each [kind] branch plus cross-cutting rules (body, side values).
 */
class ValidationTest {

    // -----------------------------------------------------------------------
    // kind=line
    // -----------------------------------------------------------------------

    @Nested
    inner class KindLine {

        @Test
        fun `valid line comment passes`() {
            val req = CreateCommentRequest(
                kind = "line",
                path = "src/app.go",
                side = "head",
                line = 10,
                body = "good",
            )
            assertTrue(Validation.validate(req).isEmpty())
        }

        @Test
        fun `missing path fails`() {
            val req = CreateCommentRequest(kind = "line", line = 5, body = "x")
            val errors = Validation.validate(req)
            assertTrue(errors.any { "path" in it })
        }

        @Test
        fun `missing line fails`() {
            val req = CreateCommentRequest(kind = "line", path = "foo.go", body = "x")
            val errors = Validation.validate(req)
            assertTrue(errors.any { "line" in it })
        }

        @Test
        fun `negative line fails`() {
            val req = CreateCommentRequest(kind = "line", path = "foo.go", line = -1, body = "x")
            val errors = Validation.validate(req)
            assertTrue(errors.any { "line" in it })
        }

        @Test
        fun `invalid side fails`() {
            val req = CreateCommentRequest(kind = "line", path = "foo.go", side = "neither", line = 1, body = "x")
            val errors = Validation.validate(req)
            assertTrue(errors.any { "side" in it })
        }

        @Test
        fun `side=base is valid`() {
            val req = CreateCommentRequest(kind = "line", path = "foo.go", side = "base", line = 1, body = "x")
            assertTrue(Validation.validate(req).isEmpty())
        }

        @Test
        fun `line_start present fails for kind=line`() {
            val req = CreateCommentRequest(
                kind = "line", path = "foo.go", line = 5, lineStart = 3, body = "x"
            )
            val errors = Validation.validate(req)
            assertTrue(errors.any { "line_start" in it || "line_end" in it })
        }
    }

    // -----------------------------------------------------------------------
    // kind=range
    // -----------------------------------------------------------------------

    @Nested
    inner class KindRange {

        @Test
        fun `valid range comment passes`() {
            val req = CreateCommentRequest(
                kind = "range",
                path = "src/app.go",
                side = "head",
                lineStart = 5,
                lineEnd = 10,
                body = "range note",
            )
            assertTrue(Validation.validate(req).isEmpty())
        }

        @Test
        fun `missing path fails`() {
            val req = CreateCommentRequest(kind = "range", lineStart = 5, lineEnd = 10, body = "x")
            assertTrue(Validation.validate(req).any { "path" in it })
        }

        @Test
        fun `missing line_start fails`() {
            val req = CreateCommentRequest(kind = "range", path = "foo.go", lineEnd = 10, body = "x")
            assertTrue(Validation.validate(req).any { "line_start" in it })
        }

        @Test
        fun `missing line_end fails`() {
            val req = CreateCommentRequest(kind = "range", path = "foo.go", lineStart = 5, body = "x")
            assertTrue(Validation.validate(req).any { "line_end" in it })
        }

        @Test
        fun `line_start greater than line_end fails`() {
            val req = CreateCommentRequest(kind = "range", path = "foo.go", lineStart = 10, lineEnd = 5, body = "x")
            assertTrue(Validation.validate(req).any { "line_start" in it && "line_end" in it })
        }

        @Test
        fun `line present fails for kind=range`() {
            val req = CreateCommentRequest(kind = "range", path = "foo.go", lineStart = 5, lineEnd = 10, line = 7, body = "x")
            assertTrue(Validation.validate(req).any { "line" in it })
        }
    }

    // -----------------------------------------------------------------------
    // kind=file
    // -----------------------------------------------------------------------

    @Nested
    inner class KindFile {

        @Test
        fun `valid file comment passes`() {
            val req = CreateCommentRequest(kind = "file", path = "src/app.go", body = "file-level note")
            assertTrue(Validation.validate(req).isEmpty())
        }

        @Test
        fun `missing path fails`() {
            val req = CreateCommentRequest(kind = "file", body = "x")
            assertTrue(Validation.validate(req).any { "path" in it })
        }

        @Test
        fun `line present fails for kind=file`() {
            val req = CreateCommentRequest(kind = "file", path = "foo.go", line = 5, body = "x")
            assertTrue(Validation.validate(req).any { "line" in it || "line_start" in it || "line_end" in it })
        }

        @Test
        fun `line_start present fails for kind=file`() {
            val req = CreateCommentRequest(kind = "file", path = "foo.go", lineStart = 5, body = "x")
            assertTrue(Validation.validate(req).any { "line" in it || "line_start" in it || "line_end" in it })
        }
    }

    // -----------------------------------------------------------------------
    // kind=review
    // -----------------------------------------------------------------------

    @Nested
    inner class KindReview {

        @Test
        fun `valid review comment passes`() {
            val req = CreateCommentRequest(kind = "review", body = "overall LGTM")
            assertTrue(Validation.validate(req).isEmpty())
        }

        @Test
        fun `review with path does not fail`() {
            // path is silently accepted for kind=review; frontend may send it.
            val req = CreateCommentRequest(kind = "review", path = "foo.go", body = "ok")
            assertTrue(Validation.validate(req).isEmpty())
        }
    }

    // -----------------------------------------------------------------------
    // Cross-cutting rules
    // -----------------------------------------------------------------------

    @Nested
    inner class CrossCutting {

        @Test
        fun `blank body fails`() {
            val req = CreateCommentRequest(kind = "review", body = "   ")
            assertTrue(Validation.validate(req).any { "body" in it })
        }

        @Test
        fun `empty body fails`() {
            val req = CreateCommentRequest(kind = "review", body = "")
            assertTrue(Validation.validate(req).any { "body" in it })
        }

        @Test
        fun `invalid kind fails`() {
            val req = CreateCommentRequest(kind = "unknown", body = "x")
            assertTrue(Validation.validate(req).any { "kind" in it })
        }
    }

    // -----------------------------------------------------------------------
    // validateState
    // -----------------------------------------------------------------------

    @Nested
    inner class ValidateState {

        @Test
        fun `open is valid`() = assertTrue(Validation.validateState("open").isEmpty())

        @Test
        fun `resolved is valid`() = assertTrue(Validation.validateState("resolved").isEmpty())

        @Test
        fun `stale is valid`() = assertTrue(Validation.validateState("stale").isEmpty())

        @Test
        fun `arbitrary string fails`() {
            assertTrue(Validation.validateState("INVALID").isNotEmpty())
        }
    }

    // -----------------------------------------------------------------------
    // isValidUuidV4
    // -----------------------------------------------------------------------

    @Nested
    inner class UuidV4 {

        @Test
        fun `valid uuid v4 passes`() {
            // version nibble 4, variant nibble 8-b
            assertTrue(Validation.isValidUuidV4("550e8400-e29b-41d4-a716-446655440000"))
            assertTrue(Validation.isValidUuidV4("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"))
        }

        @Test
        fun `uuid version 1 fails`() {
            // version nibble 1
            assertFalse(Validation.isValidUuidV4("550e8400-e29b-11d4-a716-446655440000"))
        }

        @Test
        fun `random uuid v4 passes`() {
            val id = java.util.UUID.randomUUID().toString()
            assertTrue(Validation.isValidUuidV4(id), "UUID.randomUUID() must be v4: $id")
        }

        @Test
        fun `non-uuid string fails`() {
            assertFalse(Validation.isValidUuidV4("not-a-uuid"))
            assertFalse(Validation.isValidUuidV4(""))
        }
    }
}
