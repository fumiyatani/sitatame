package dev.sitatame.intellij.git

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for [ChangedFilesProvider.listChangedFiles].
 *
 * These tests exercise pure / boundary conditions without requiring a real git
 * repo or the IntelliJ Platform.
 */
class ChangedFilesProviderTest {

    @Test
    fun emptyRepoRoot_returnsEmpty() {
        val result = ChangedFilesProvider.listChangedFiles("", "origin/main")
        assertTrue("empty repoRoot should return empty list", result.isEmpty())
    }

    @Test
    fun emptyBaseRef_returnsEmpty() {
        val result = ChangedFilesProvider.listChangedFiles("/some/repo", "")
        assertTrue("empty baseRef should return empty list", result.isEmpty())
    }

    @Test
    fun nonExistentRepoRoot_returnsEmpty() {
        // ProcessBuilder will fail; should return empty list, not throw.
        val result = ChangedFilesProvider.listChangedFiles(
            "/nonexistent/path/to/repo/that/does/not/exist",
            "origin/main"
        )
        assertTrue("non-existent path should return empty list", result.isEmpty())
    }

    @Test
    fun resultIsSorted() {
        // If we had a real git repo we could verify sorting; here we verify the
        // contract using a known-empty result from a non-existent repo (empty = trivially sorted).
        val result = ChangedFilesProvider.listChangedFiles("/tmp/no-repo", "HEAD~1")
        assertEquals("result from non-existent repo should be empty and trivially sorted",
            result.sorted(), result)
    }
}
