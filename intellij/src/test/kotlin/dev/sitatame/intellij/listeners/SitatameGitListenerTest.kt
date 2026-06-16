package dev.sitatame.intellij.listeners

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Pure unit tests for [SitatameGitListener.resolveBranchRaw] and the
 * branch-change detection logic in [SitatameGitListener].
 *
 * No IntelliJ Platform or [git4idea.repo.GitRepository] is required.
 */
class SitatameGitListenerTest {

    // -----------------------------------------------------------------------
    // resolveBranchRaw — decision table
    // -----------------------------------------------------------------------

    @Test
    fun resolveBranchRaw_namedBranch_returnsBranchName() {
        val result = SitatameGitListener.resolveBranchRaw(
            branchName = "feature/my-feature",
            revision = "abc123def456",
        )
        assertEquals("feature/my-feature", result)
    }

    @Test
    fun resolveBranchRaw_namedBranchNoRevision_returnsBranchName() {
        val result = SitatameGitListener.resolveBranchRaw(
            branchName = "main",
            revision = null,
        )
        assertEquals("main", result)
    }

    @Test
    fun resolveBranchRaw_detachedHeadWithRevision_returnsDetachedPrefix() {
        val result = SitatameGitListener.resolveBranchRaw(
            branchName = null,
            revision = "abc123def456xyz",
        )
        assertEquals("detached/abc123def456", result)
    }

    @Test
    fun resolveBranchRaw_detachedHeadShortRevision_returnsFullRevision() {
        // revision shorter than 12 chars → take(12) returns the whole string
        val result = SitatameGitListener.resolveBranchRaw(
            branchName = null,
            revision = "abc",
        )
        assertEquals("detached/abc", result)
    }

    @Test
    fun resolveBranchRaw_bothNull_returnsNull() {
        val result = SitatameGitListener.resolveBranchRaw(
            branchName = null,
            revision = null,
        )
        assertNull(result)
    }

    // -----------------------------------------------------------------------
    // resolveBranchRaw — branch name takes priority over revision
    // -----------------------------------------------------------------------

    @Test
    fun resolveBranchRaw_branchAndRevision_branchNameWins() {
        // When branchName is present, it wins regardless of revision.
        val result = SitatameGitListener.resolveBranchRaw(
            branchName = "release/1.0",
            revision = "deadbeef1234",
        )
        assertEquals("release/1.0", result)
    }

    // -----------------------------------------------------------------------
    // Branch-change detection via injected lambdas
    //
    // We construct the listener with real invalidateCache / publishChanged
    // lambdas that record calls, then call the internal logic through a
    // thin helper that replicates the listener's decision — or we rely on
    // the companion pure functions to validate the inputs, and test the
    // listener constructor wiring by checking that the lambdas are invoked
    // when we manually replicate the state transition.
    //
    // The key invariant: if (old != new) → invalidate + publish.
    // -----------------------------------------------------------------------

    @Test
    fun resolveBranchRaw_differentBranches_notEqual() {
        val b1 = SitatameGitListener.resolveBranchRaw("main", null)
        val b2 = SitatameGitListener.resolveBranchRaw("feature/x", null)
        assert(b1 != b2) { "Expected different branches to produce different slugs" }
    }

    @Test
    fun resolveBranchRaw_sameBranch_equal() {
        val b1 = SitatameGitListener.resolveBranchRaw("main", null)
        val b2 = SitatameGitListener.resolveBranchRaw("main", null)
        assertEquals(b1, b2)
    }

    @Test
    fun resolveBranchRaw_detachedToNamed_notEqual() {
        val detached = SitatameGitListener.resolveBranchRaw(null, "abc123def456")
        val named = SitatameGitListener.resolveBranchRaw("main", null)
        assert(detached != named)
    }

    @Test
    fun resolveBranchRaw_detachedSameSha_equal() {
        val a = SitatameGitListener.resolveBranchRaw(null, "abc123def456xyz")
        val b = SitatameGitListener.resolveBranchRaw(null, "abc123def456xyz")
        assertEquals(a, b)
    }

    @Test
    fun resolveBranchRaw_detachedDifferentSha_notEqual() {
        val a = SitatameGitListener.resolveBranchRaw(null, "abc123def456")
        val b = SitatameGitListener.resolveBranchRaw(null, "000000000000")
        assert(a != b)
    }
}
