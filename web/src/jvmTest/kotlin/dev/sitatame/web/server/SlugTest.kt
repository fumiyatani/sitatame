package dev.sitatame.web.server

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * Parity tests against the Go side (`internal/review/slug_test.go`). Fixed-value
 * expectations are reused so a divergence here surfaces immediately rather than
 * waiting for an on-disk directory mismatch in production.
 */
class SlugTest {

    @Test
    fun `BranchSlug PRD example matches Go`() {
        val branch = "feature/auth-refactor"
        val got = Slug.branchSlug(branch)
        assertTrue(got.startsWith("feature_auth-refactor__"), "expected unsafe '/' replaced with '_': $got")
        val parts = got.split("__")
        assertEquals(2, parts.size)
        assertEquals(8, parts[1].length)
    }

    @Test
    fun `BranchSlug truncates to 32`() {
        val branch = "a".repeat(50)
        val got = Slug.branchSlug(branch)
        val parts = got.split("__")
        assertEquals(2, parts.size)
        assertEquals(32, parts[0].length)
        assertEquals(8, parts[1].length)
    }

    @Test
    fun `BranchSlug all-unsafe falls back to branch`() {
        val got = Slug.branchSlug("////")
        assertTrue(got.startsWith("branch__"), "expected 'branch__' prefix, got $got")
    }

    @Test
    fun `BranchSlug empty branch matches Go fixed value`() {
        // sha1("")[:8] == da39a3ee — this is the value the Go side bakes into
        // its detached-HEAD test and into Paths.Slug. If this drifts, the Web
        // backend will look at a different directory than the TUI.
        val got = Slug.branchSlug("")
        assertEquals("branch__da39a3ee", got)
    }

    @Test
    fun `BranchSlug deterministic`() {
        assertEquals(Slug.branchSlug("dev"), Slug.branchSlug("dev"))
        assertNotEquals(Slug.branchSlug("dev"), Slug.branchSlug("Dev"))
    }

    @Test
    fun `ProjectSlug deterministic and basename prefix`() {
        val a = Slug.projectSlug("/Users/me/code/sitatame")
        val b = Slug.projectSlug("/Users/me/code/sitatame")
        assertEquals(a, b)
        assertTrue(a.startsWith("sitatame__"), "expected sitatame__ prefix: $a")
    }

    @Test
    fun `ProjectSlug distinguishes checkouts`() {
        val a = Slug.projectSlug("/Users/me/code/sitatame")
        val b = Slug.projectSlug("/Users/me/work/sitatame")
        assertNotEquals(a, b)
    }
}
