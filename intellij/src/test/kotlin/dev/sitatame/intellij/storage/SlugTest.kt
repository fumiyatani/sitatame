package dev.sitatame.intellij.storage

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.MessageDigest

/**
 * Kotlin counterpart to `internal/review/slug_test.go`.
 *
 * The cross-route contract is byte-for-byte agreement between Go's
 * [BranchSlug] / [ProjectSlug] and the Kotlin port, so we re-encode the same
 * PRD example and assert the prefix shape.
 */
class SlugTest {

    @Test
    fun branchSlug_prdExample() {
        val branch = "feature/auth-refactor"
        val got = Slug.branchSlug(branch)
        val sum = MessageDigest.getInstance("SHA-1").digest(branch.toByteArray(Charsets.UTF_8))
        val wantHash = sum.toHex().substring(0, 8)
        assertEquals("feature_auth-refactor__$wantHash", got)
        assertTrue("expected unsafe '/' replaced with '_'", got.startsWith("feature_auth-refactor__"))
    }

    @Test
    fun branchSlug_truncatesTo32() {
        val branch = "a".repeat(50)
        val got = Slug.branchSlug(branch)
        val parts = got.split("__")
        assertEquals(2, parts.size)
        assertEquals(32, parts[0].length)
        assertEquals(8, parts[1].length)
    }

    @Test
    fun branchSlug_allUnsafeFallsBackToBranch() {
        val got = Slug.branchSlug("////")
        assertTrue("expected 'branch__' prefix, got $got", got.startsWith("branch__"))
    }

    @Test
    fun branchSlug_emptyBranchUsesGoDetachedSlug() {
        val got = Slug.branchSlug("")
        // The Go side encodes the empty-string SHA-1 as "branch__da39a3ee".
        // This is the collision slug that two *different* transient Git states
        // would share if RepoContext ever passed "" as the branch input.
        // As of issue #118, RepoContext.resolveBranch returns null for the
        // (branchName=null, revision=null) transient case, so Slug.branchSlug("")
        // is no longer reachable from normal plugin flows.
        // We keep this test to document the sentinel value and catch any future
        // Go-side drift in the cross-route contract.
        assertEquals("branch__da39a3ee", got)
    }

    @Test
    fun branchSlug_deterministicAcrossCalls() {
        assertEquals(Slug.branchSlug("dev"), Slug.branchSlug("dev"))
        assertNotEquals(Slug.branchSlug("dev"), Slug.branchSlug("Dev"))
    }

    @Test
    fun projectSlug_deterministic() {
        val a = Slug.projectSlug("/Users/me/code/sitatame")
        val b = Slug.projectSlug("/Users/me/code/sitatame")
        assertEquals(a, b)
        assertTrue("expected sitatame__ prefix: $a", a.startsWith("sitatame__"))
    }

    @Test
    fun projectSlug_distinguishesCheckouts() {
        val a = Slug.projectSlug("/Users/me/code/sitatame")
        val b = Slug.projectSlug("/Users/me/work/sitatame")
        assertNotEquals(a, b)
    }

    @Test
    fun projectSlug_nonAsciiBasenameDoesNotCrash() {
        val got = Slug.projectSlug("/Users/me/日本語")
        assertTrue("expected slug to contain '__': $got", got.contains("__"))
    }

    private fun ByteArray.toHex(): String {
        val sb = StringBuilder(size * 2)
        for (b in this) {
            val v = b.toInt() and 0xFF
            sb.append(HEX[v ushr 4]).append(HEX[v and 0x0F])
        }
        return sb.toString()
    }

    companion object {
        private val HEX = "0123456789abcdef".toCharArray()
    }
}
