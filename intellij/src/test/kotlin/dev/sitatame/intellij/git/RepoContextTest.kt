package dev.sitatame.intellij.git

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Unit tests for [RepoContext.resolveBranch] — the pure function that maps
 * raw Git4Idea state to a branch slug input.
 *
 * These tests exercise the decision table without the IntelliJ test platform:
 *
 * | branchName | revision | result                    |
 * |------------|----------|---------------------------|
 * | non-null   | any      | branchName (normal HEAD)  |
 * | null       | non-null | revision (detached HEAD)  |
 * | null       | null     | null  (transient — refuse)|
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
    fun resolveBranch_detachedHead_returnsRevision() {
        val sha = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
        val result = RepoContext.resolveBranch(null, sha)
        assertEquals(sha, result)
    }

    @Test
    fun resolveBranch_detachedHead_differentShas_produceDifferentSlugs() {
        val sha1 = "aaaa000000000000000000000000000000000000"
        val sha2 = "bbbb000000000000000000000000000000000000"
        val r1 = RepoContext.resolveBranch(null, sha1)
        val r2 = RepoContext.resolveBranch(null, sha2)
        // Two different detached commits must not share a slug input.
        org.junit.Assert.assertNotEquals(r1, r2)
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
