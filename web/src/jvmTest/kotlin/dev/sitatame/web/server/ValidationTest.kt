package dev.sitatame.web.server

import dev.sitatame.web.api.CreateCommentRequest
import dev.sitatame.web.api.FileDto
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
    // Blob integrity
    // -----------------------------------------------------------------------

    @Nested
    inner class BlobIntegrity {

        private fun fileDto(
            path: String,
            blobBase: String? = "abc1234",
            blobHead: String? = "def5678",
        ) = FileDto(
            path = path,
            status = "M",
            adds = 1,
            dels = 1,
            hunks = emptyList(),
            blobBase = blobBase,
            blobHead = blobHead,
        )

        @Test
        fun `blob matches head side passes`() {
            val req = CreateCommentRequest(
                kind = "line",
                path = "foo.go",
                side = "head",
                line = 5,
                blob = "def5678",
                body = "ok",
            )
            val files = listOf(fileDto("foo.go"))
            assertTrue(Validation.validate(req, files).isEmpty())
        }

        @Test
        fun `blob matches base side passes`() {
            val req = CreateCommentRequest(
                kind = "line",
                path = "foo.go",
                side = "base",
                line = 3,
                blob = "abc1234",
                body = "ok",
            )
            val files = listOf(fileDto("foo.go"))
            assertTrue(Validation.validate(req, files).isEmpty())
        }

        @Test
        fun `abbreviated client blob is prefix of full server blob passes`() {
            val req = CreateCommentRequest(
                kind = "line",
                path = "foo.go",
                side = "head",
                line = 1,
                blob = "def567",  // shorter than server "def5678"
                body = "ok",
            )
            val files = listOf(fileDto("foo.go", blobHead = "def5678abcd1234"))
            assertTrue(Validation.validate(req, files).isEmpty())
        }

        @Test
        fun `blob mismatch fails with stale anchor message`() {
            val req = CreateCommentRequest(
                kind = "line",
                path = "foo.go",
                side = "head",
                line = 1,
                blob = "aaaaaa1",  // wrong blob
                body = "ok",
            )
            val files = listOf(fileDto("foo.go"))
            val errors = Validation.validate(req, files)
            assertTrue(errors.any { "blob mismatch" in it || "stale" in it }, "expected stale error, got: $errors")
        }

        @Test
        fun `base side blob mismatch fails`() {
            val req = CreateCommentRequest(
                kind = "line",
                path = "foo.go",
                side = "base",
                line = 1,
                blob = "xxxxxxx",  // wrong base blob
                body = "ok",
            )
            val files = listOf(fileDto("foo.go"))
            val errors = Validation.validate(req, files)
            assertTrue(errors.any { "blob mismatch" in it || "stale" in it })
        }

        @Test
        fun `blob check skipped when files is null`() {
            val req = CreateCommentRequest(
                kind = "line",
                path = "foo.go",
                side = "head",
                line = 1,
                blob = "totally_wrong_sha",
                body = "ok",
            )
            // No files provided — blob check must be skipped entirely.
            assertTrue(Validation.validate(req, files = null).isEmpty())
        }

        @Test
        fun `blob check skipped when blob is null`() {
            val req = CreateCommentRequest(
                kind = "line",
                path = "foo.go",
                side = "head",
                line = 1,
                blob = null,
                body = "ok",
            )
            val files = listOf(fileDto("foo.go"))
            // Null blob → no blob check, should pass shape validation.
            assertTrue(Validation.validate(req, files).isEmpty())
        }

        @Test
        fun `blob check skipped when path not in file list`() {
            val req = CreateCommentRequest(
                kind = "line",
                path = "other.go",
                side = "head",
                line = 1,
                blob = "completely_wrong",
                body = "ok",
            )
            val files = listOf(fileDto("foo.go"))
            // other.go not in files list → no blob check.
            assertTrue(Validation.validate(req, files).isEmpty())
        }
    }

    // -----------------------------------------------------------------------
    // blobShasCompatible
    // -----------------------------------------------------------------------

    @Nested
    inner class BlobShasCompatible {

        @Test
        fun `identical strings match`() {
            assertTrue(Validation.blobShasCompatible("abc1234", "abc1234"))
        }

        @Test
        fun `shorter is prefix of longer`() {
            assertTrue(Validation.blobShasCompatible("abc", "abc1234"))
            assertTrue(Validation.blobShasCompatible("abc1234", "abc"))
        }

        @Test
        fun `non-prefix strings do not match`() {
            assertFalse(Validation.blobShasCompatible("abc1234", "def5678"))
            assertFalse(Validation.blobShasCompatible("abc", "def1234"))
        }

        @Test
        fun `case insensitive match`() {
            assertTrue(Validation.blobShasCompatible("ABC1234", "abc1234"))
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
