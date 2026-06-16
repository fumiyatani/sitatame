package dev.sitatame.intellij.listeners

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Pure unit tests for [SitatameVfsListener] path-predicate and context-extraction
 * helpers. No IntelliJ Platform is required.
 */
class SitatameVfsListenerTest {

    // -----------------------------------------------------------------------
    // isRelevant — positive cases
    // -----------------------------------------------------------------------

    @Test
    fun isRelevant_canonicalUnixPath_returnsTrue() {
        val path = "/Users/alice/.sitatame/my-project/my-branch/review.md"
        assertTrue(SitatameVfsListener.isRelevant(path))
    }

    @Test
    fun isRelevant_deepOutputRoot_returnsTrue() {
        val path = "/home/bob/work/.sitatame/acme-corp/feature-login/review.md"
        assertTrue(SitatameVfsListener.isRelevant(path))
    }

    @Test
    fun isRelevant_customSitatameHome_returnsTrue() {
        // When SITATAME_HOME points to a directory that doesn't contain ".sitatame",
        // the path won't match — but that's the correct behaviour: we only watch
        // the default layout. Custom SITATAME_HOME paths won't contain "/.sitatame/".
        // This test confirms the positive default case.
        val path = "/tmp/sitatame-ci/.sitatame/proj/branch/review.md"
        assertTrue(SitatameVfsListener.isRelevant(path))
    }

    // -----------------------------------------------------------------------
    // isRelevant — negative cases
    // -----------------------------------------------------------------------

    @Test
    fun isRelevant_notReviewMd_returnsFalse() {
        val path = "/Users/alice/.sitatame/project/branch/review.md.bak"
        assertFalse(SitatameVfsListener.isRelevant(path))
    }

    @Test
    fun isRelevant_noSitatameSegment_returnsFalse() {
        val path = "/Users/alice/documents/review.md"
        assertFalse(SitatameVfsListener.isRelevant(path))
    }

    @Test
    fun isRelevant_sitatameInFilename_returnsFalse() {
        // ".sitatame" appears in the filename, not as a segment
        val path = "/Users/alice/.sitatame-alt/project/branch/review.md"
        assertFalse(SitatameVfsListener.isRelevant(path))
    }

    @Test
    fun isRelevant_rescueJson_returnsFalse() {
        val path = "/Users/alice/.sitatame/proj/branch/review.md.rescue.20250101T120000-000000000.json"
        assertFalse(SitatameVfsListener.isRelevant(path))
    }

    @Test
    fun isRelevant_tmpFile_returnsFalse() {
        val path = "/Users/alice/.sitatame/proj/branch/.review.123.tmp"
        assertFalse(SitatameVfsListener.isRelevant(path))
    }

    // -----------------------------------------------------------------------
    // extractContext — positive cases
    // -----------------------------------------------------------------------

    @Test
    fun extractContext_canonicalPath_returnsBranchSlug() {
        val path = "/Users/alice/.sitatame/my-project/feature-login/review.md"
        val result = SitatameVfsListener.extractContext(path)
        assertNotNull(result)
        assertEquals("feature-login", result!!.second)
    }

    @Test
    fun extractContext_canonicalPath_returnsOutputRoot() {
        val path = "/Users/alice/.sitatame/my-project/feature-login/review.md"
        val result = SitatameVfsListener.extractContext(path)
        assertNotNull(result)
        // outputRoot is everything up to (not including) /my-project
        assertEquals("/Users/alice/.sitatame", result!!.first)
    }

    @Test
    fun extractContext_shortPath_returnsContext() {
        // Path layout: /<outputRoot>/<projectSlug>/<branchSlug>/review.md
        // /root/proj/branch/review.md → outputRoot=/root, branchSlug=branch
        val path = "/root/proj/branch/review.md"
        val result = SitatameVfsListener.extractContext(path)
        assertNotNull(result)
        assertEquals("branch", result!!.second)
        assertEquals("/root", result.first)
    }

    // -----------------------------------------------------------------------
    // extractContext — negative / degenerate cases
    // -----------------------------------------------------------------------

    @Test
    fun extractContext_tooFewSegments_returnsNull() {
        // Only "review.md" — no parent dirs at all
        val path = "review.md"
        assertNull(SitatameVfsListener.extractContext(path))
    }

    @Test
    fun extractContext_singleSegmentBeforeFile_returnsNull() {
        val path = "/branch/review.md"
        assertNull(SitatameVfsListener.extractContext(path))
    }

    // -----------------------------------------------------------------------
    // prepareChange — no relevant events → returns null
    // -----------------------------------------------------------------------

    @Test
    fun prepareChange_noRelevantEvents_returnsNull() {
        val listener = SitatameVfsListener(
            invalidateCache = { },
            publishChanged = { _, _ -> },
        )
        // prepareChange with an empty list should return null (no-op).
        val applier = listener.prepareChange(emptyList())
        assertNull(applier)
    }
}
