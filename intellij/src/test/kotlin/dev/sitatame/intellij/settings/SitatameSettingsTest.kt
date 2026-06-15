package dev.sitatame.intellij.settings

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Pure unit tests for [SitatameSettings.PersistedState].
 *
 * These run without the IntelliJ Platform (no ApplicationManager) because they
 * only exercise the data class directly. Platform-level serialisation
 * round-trips are covered by the IDE's built-in PersistentStateComponent test
 * harness, which requires BasePlatformTestCase; that lives in a follow-up when
 * the platform test fixtures are fully wired.
 */
class SitatameSettingsTest {

    @Test
    fun defaultBaseRefIsEmpty() {
        val state = SitatameSettings.PersistedState()
        assertEquals(
            "Default baseRef must be empty (auto-detect mode)",
            "",
            state.baseRef,
        )
    }

    @Test
    fun defaultSitatameHomeOverrideIsEmpty() {
        val state = SitatameSettings.PersistedState()
        assertEquals(
            "Default sitatameHomeOverride must be empty",
            "",
            state.sitatameHomeOverride,
        )
    }

    @Test
    fun stateRoundtripPreservesBaseRef() {
        val original = SitatameSettings.PersistedState(
            sitatameHomeOverride = "/custom/home",
            baseRef = "origin/develop",
        )
        // Simulate loadState via copy (XmlSerializerUtil.copyBean does the same)
        val copy = original.copy()
        assertEquals(original.sitatameHomeOverride, copy.sitatameHomeOverride)
        assertEquals(original.baseRef, copy.baseRef)
    }

    @Test
    fun stateRoundtripPreservesEmptyBaseRef() {
        val original = SitatameSettings.PersistedState(
            sitatameHomeOverride = "",
            baseRef = "",
        )
        val copy = original.copy()
        assertEquals("", copy.baseRef)
    }

    @Test
    fun explicitBaseRefIsPreserved() {
        val state = SitatameSettings.PersistedState(baseRef = "origin/release")
        assertEquals("origin/release", state.baseRef)
    }
}
