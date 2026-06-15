package dev.sitatame.intellij.git

import dev.sitatame.intellij.storage.Slug
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Unit tests for [RepoContext.resolveBranch] — the pure function that maps
 * raw Git4Idea state to a branch slug input.
 *
 * These tests exercise the decision table without the IntelliJ test platform:
 *
 * | branchName | revision | result                           |
 * |------------|----------|----------------------------------|
 * | non-null   | any      | branchName (normal HEAD)         |
 * | null       | non-null | "detached/<revision[:12]>"       |
 * | null       | null     | null  (transient — refuse)       |
 *
 * The "detached/<sha12>" form mirrors the TUI normalisation in cmd/root.go:
 *   if branch == "" && len(headSHA) >= 12 { branch = "detached/" + headSHA[:12] }
 * Both sides then call BranchSlug/Slug.branchSlug with the same input,
 * ensuring the on-disk path is identical regardless of which tool wrote it.
 *
 * The transient-null case guards against the `"branch__da39a3ee"` slug
 * collision (SHA-1 of the empty string) that would silently mix comments
 * across unrelated commits. See issue #118.
 */
class RepoContextTest {

    // ------------------------------------------------------------------ normal HEAD

    @Test
    fun resolveBranch_normalHead_returnsBranchName() {
        val result = RepoContext.resolveBranch("main", "abc123def456")
        assertEquals("main", result)
    }

    @Test
    fun resolveBranch_featureBranch_returnsBranchName() {
        val result = RepoContext.resolveBranch("feature/auth-refactor", "deadbeef1234")
        assertEquals("feature/auth-refactor", result)
    }

    @Test
    fun resolveBranch_branchPresent_revisionNull_returnsBranchName() {
        // currentRevision can be null when branch is checked out but the index
        // hasn't been scanned yet. Branch name takes priority when available.
        val result = RepoContext.resolveBranch("main", null)
        assertEquals("main", result)
    }

    // ------------------------------------------------------------------ detached HEAD

    @Test
    fun resolveBranch_detachedHead_returnsDetachedPrefix() {
        // Full 40-char SHA: result must be "detached/<first 12 chars>"
        val sha = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
        val result = RepoContext.resolveBranch(null, sha)
        assertEquals("detached/a1b2c3d4e5f6", result)
    }

    @Test
    fun resolveBranch_detachedHead_exactlyMatchesTuiNormalisation() {
        // Pin the cross-tool contract: TUI does headSHA[:12] with Go string
        // slicing (bytes 0..11), Kotlin does revision.take(12) — identical for
        // hex-only SHAs. Assert the literal string the TUI would produce.
        val sha = "abcdef0123456789abcdef0123456789abcdef01"
        val result = RepoContext.resolveBranch(null, sha)
        assertEquals("detached/abcdef012345", result)
    }

    @Test
    fun resolveBranch_detachedHead_slugParityWithTui() {
        // Verify that IntelliJ's Slug.branchSlug on the resolveBranch output
        // equals the slug that the TUI would produce for the same commit.
        // TUI: BranchSlug("detached/abcdef012345") — computed from slug.go:
        //   safePrefix("detached/abcdef012345") = "detached_abcdef012345" (/ → _)
        //   sha1("detached/abcdef012345")[:8] = "edc8ea64"  (see below)
        // We assert the prefix shape so the test stays readable and the exact
        // sha1 is verified by SlugTest.branchSlug_prdExample cross-check.
        val sha = "abcdef0123456789abcdef0123456789abcdef01"
        val branchInput = RepoContext.resolveBranch(null, sha)!! // "detached/abcdef012345"
        val slug = Slug.branchSlug(branchInput)
        // The prefix is the first 32 chars of branchInput with '/' → '_'.
        // "detached/abcdef012345" is 21 chars so no truncation occurs.
        assert(slug.startsWith("detached_abcdef012345__")) {
            "Expected slug to start with 'detached_abcdef012345__', got: $slug"
        }
        // Hash portion must be exactly 8 hex chars.
        val hash = slug.substringAfterLast("__")
        assertEquals("slug hash must be 8 hex chars", 8, hash.length)
        assert(hash.all { it in '0'..'9' || it in 'a'..'f' }) {
            "slug hash must be lowercase hex, got: $hash"
        }
    }

    @Test
    fun resolveBranch_detachedHead_differentShas_produceDifferentSlugInputs() {
        val sha1 = "aaaa000000000000000000000000000000000000"
        val sha2 = "bbbb000000000000000000000000000000000000"
        val r1 = RepoContext.resolveBranch(null, sha1)
        val r2 = RepoContext.resolveBranch(null, sha2)
        // Two different detached commits must not share a slug input.
        assertNotEquals(r1, r2)
    }

    @Test
    fun resolveBranch_detachedHead_sameSha_isDeterministic() {
        val sha = "cafebabe0000000000000000000000000000cafe"
        val r1 = RepoContext.resolveBranch(null, sha)
        val r2 = RepoContext.resolveBranch(null, sha)
        assertEquals("same SHA must produce same result", r1, r2)
    }

    // ------------------------------------------------------------------ transient state

    @Test
    fun resolveBranch_transientState_returnsNull() {
        // Both null: mid-rebase / mid-reset. Refusing to produce a slug is
        // safer than writing to the "branch__da39a3ee" fallback dir.
        val result = RepoContext.resolveBranch(null, null)
        assertNull(
            "transient state (no branch, no SHA) must return null to prevent slug collision",
            result
        )
    }

    @Test
    fun resolveBranch_transientState_wouldHaveProducedCollisionSlug() {
        // Illustrate the bug that this fix prevents: if we had fallen through
        // to branchSlug(""), the result would be "branch__da39a3ee" for every
        // transient state, mixing comments across unrelated commits.
        val transientResult = RepoContext.resolveBranch(null, null)
        assertNull(transientResult)
        // The fixed path never reaches Slug.branchSlug(""), so we cannot
        // observe "branch__da39a3ee" here — that's the point.
    }
}
