package dev.sitatame.intellij.storage

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Kotlin counterpart to `internal/review/slug_test.go`'s `TestPaths*` cases.
 * Uses [PathsFactory.newPathsWithRoot] so the test is independent of the
 * user's actual `SITATAME_HOME` / `HOME`.
 */
class PathsTest {

    @Test
    fun pathsLayoutMatchesGo() {
        val p = PathsFactory.newPathsWithRoot("/out", "/repo", "feature/x")
        assertTrue("branch slug should be set", p.slug.isNotEmpty() && p.slug.contains("__"))
        assertTrue("project slug should be set", p.projectSlug.isNotEmpty() && p.projectSlug.contains("__"))

        val projectRoot = File("/out", p.projectSlug).path
        assertEquals(File(projectRoot, "reviews").resolve(p.slug).path, p.reviewsDir())
        assertEquals(File(projectRoot, "drafts").resolve(p.slug).path, p.draftsDir())
        assertEquals(
            File(p.reviewsDir(), "20260501T000000-x.md").path,
            p.reviewFile("20260501T000000-x"),
        )
        assertEquals(
            File(p.draftsDir(), "20260501T000000-x.md").path,
            p.draftFile("20260501T000000-x"),
        )
    }

    @Test
    fun emptyBranchUsesDetachedSlug() {
        val p = PathsFactory.newPathsWithRoot("/out", "/repo", "")
        assertEquals("branch__da39a3ee", p.slug)
        val wantDrafts = File("/out", p.projectSlug).resolve("drafts").resolve("branch__da39a3ee").path
        assertEquals(wantDrafts, p.draftsDir())
    }

    @Test
    fun differentRootsYieldDifferentProjectSlugs() {
        val a = PathsFactory.newPathsWithRoot("/out", "/Users/me/code/sitatame", "feature")
        val b = PathsFactory.newPathsWithRoot("/out", "/Users/me/work/sitatame", "feature")
        assertNotEquals(a.projectSlug, b.projectSlug)
    }
}
