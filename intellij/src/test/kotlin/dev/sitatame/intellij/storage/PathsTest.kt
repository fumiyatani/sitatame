package dev.sitatame.intellij.storage

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Kotlin counterpart to `internal/review/paths_test.go`'s TestPaths* cases.
 * Uses [PathsFactory.newPathsWithRoot] so the test is independent of the
 * user's actual `SITATAME_HOME` / `HOME`.
 *
 * As of issue #76 the layout is:
 *   `<OutputRoot>/<ProjectSlug>/<BranchSlug>/` (branchDir)
 * with a single review.md, review.md.bak, and rescue glob.
 */
class PathsTest {

    @Test
    fun branchDirLayoutMatchesGo() {
        val p = PathsFactory.newPathsWithRoot("/out", "/repo", "feature/x")
        assertTrue("branch slug should be set", p.slug.isNotEmpty() && p.slug.contains("__"))
        assertTrue("project slug should be set", p.projectSlug.isNotEmpty() && p.projectSlug.contains("__"))

        val projectRoot = File("/out", p.projectSlug).path
        assertEquals(File(projectRoot, p.slug).path, p.branchDir())
    }

    @Test
    fun reviewFileIsInsideBranchDir() {
        val p = PathsFactory.newPathsWithRoot("/out", "/repo", "feature/x")
        assertEquals(File(p.branchDir(), "review.md").path, p.reviewFile())
    }

    @Test
    fun bakFileIsInsideBranchDir() {
        val p = PathsFactory.newPathsWithRoot("/out", "/repo", "feature/x")
        assertEquals(File(p.branchDir(), "review.md.bak").path, p.bakFile())
    }

    @Test
    fun rescueFilePatternIsInsideBranchDir() {
        val p = PathsFactory.newPathsWithRoot("/out", "/repo", "feature/x")
        assertEquals(File(p.branchDir(), "review.md.rescue.*.json").path, p.rescueFilePattern())
    }

    @Test
    fun emptyBranchUsesDetachedSlug() {
        val p = PathsFactory.newPathsWithRoot("/out", "/repo", "")
        assertEquals("branch__da39a3ee", p.slug)
        val wantBranchDir = File("/out", p.projectSlug).resolve("branch__da39a3ee").path
        assertEquals(wantBranchDir, p.branchDir())
    }

    @Test
    fun differentRootsYieldDifferentProjectSlugs() {
        val a = PathsFactory.newPathsWithRoot("/out", "/Users/me/code/sitatame", "feature")
        val b = PathsFactory.newPathsWithRoot("/out", "/Users/me/work/sitatame", "feature")
        assertNotEquals(a.projectSlug, b.projectSlug)
    }
}
