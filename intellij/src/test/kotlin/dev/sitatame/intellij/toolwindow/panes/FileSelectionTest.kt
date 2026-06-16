package dev.sitatame.intellij.toolwindow.panes

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for [FileSelection] sealed model.
 *
 * Pure data model — no IntelliJ Platform required.
 */
class FileSelectionTest {

    @Test
    fun all_isSingleton() {
        // Both references must be the same object (object declaration).
        assertTrue("FileSelection.All should be a singleton", FileSelection.All === FileSelection.All)
    }

    @Test
    fun one_equality_sameRelPath() {
        val a = FileSelection.One("src/Foo.kt")
        val b = FileSelection.One("src/Foo.kt")
        assertEquals("Two One with same relPath should be equal", a, b)
    }

    @Test
    fun one_equality_differentRelPath() {
        val a = FileSelection.One("src/Foo.kt")
        val b = FileSelection.One("src/Bar.kt")
        assertNotEquals("Two One with different relPath should not be equal", a, b)
    }

    @Test
    fun all_notEqualToOne() {
        val one = FileSelection.One("src/Foo.kt")
        assertNotEquals("All should not equal One", FileSelection.All, one)
    }

    @Test
    fun whenExhaustive_allBranchesCovered() {
        // Compile-time check: when is exhaustive on sealed class.
        val sel: FileSelection = FileSelection.All
        val result: String = when (sel) {
            is FileSelection.All -> "all"
            is FileSelection.One -> "one:${sel.relPath}"
        }
        assertEquals("all", result)
    }

    @Test
    fun whenExhaustive_oneBranchCarriesRelPath() {
        val sel: FileSelection = FileSelection.One("app/Main.kt")
        val result: String = when (sel) {
            is FileSelection.All -> "all"
            is FileSelection.One -> "one:${sel.relPath}"
        }
        assertEquals("one:app/Main.kt", result)
    }
}
