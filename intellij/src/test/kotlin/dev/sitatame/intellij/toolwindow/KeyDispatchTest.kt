package dev.sitatame.intellij.toolwindow

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.awt.event.KeyEvent

/**
 * Unit tests for [SitatameToolWindowContent.Companion.keyCodeToAction].
 *
 * Pure function tests — no IntelliJ Platform required.
 */
class KeyDispatchTest {

    @Test
    fun enter_mapsToJump() {
        assertEquals(
            "VK_ENTER should map to JUMP",
            SitatameToolWindowContent.Companion.KeyAction.JUMP,
            SitatameToolWindowContent.keyCodeToAction(KeyEvent.VK_ENTER),
        )
    }

    @Test
    fun space_mapsToToggle() {
        assertEquals(
            "VK_SPACE should map to TOGGLE",
            SitatameToolWindowContent.Companion.KeyAction.TOGGLE,
            SitatameToolWindowContent.keyCodeToAction(KeyEvent.VK_SPACE),
        )
    }

    @Test
    fun tab_mapsToNull() {
        assertNull(
            "VK_TAB should have no binding",
            SitatameToolWindowContent.keyCodeToAction(KeyEvent.VK_TAB),
        )
    }

    @Test
    fun escape_mapsToNull() {
        assertNull(
            "VK_ESCAPE should have no binding",
            SitatameToolWindowContent.keyCodeToAction(KeyEvent.VK_ESCAPE),
        )
    }

    @Test
    fun unknownKey_mapsToNull() {
        assertNull(
            "Unmapped key codes should return null",
            SitatameToolWindowContent.keyCodeToAction(KeyEvent.VK_F1),
        )
    }
}
