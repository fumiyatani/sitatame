package dev.sitatame.intellij.settings

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.PersistentStateComponent
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage
import com.intellij.util.xmlb.XmlSerializerUtil

/**
 * Application-level persistent settings.
 *
 * Two knobs Phase 1 needs:
 *  - SITATAME_HOME override — lets the user point review storage at a
 *    non-default directory without editing their shell rc.
 *  - Base ref — the explicit base ref for diff context. When blank (default),
 *    RepoContext auto-detects via remote.origin.head git config, falling back
 *    to "origin/main". Set an explicit value here to pin a specific ref
 *    (e.g. "origin/develop") and skip auto-detection.
 */
@Service(Service.Level.APP)
@State(name = "SitatameSettings", storages = [Storage("sitatame.xml")])
class SitatameSettings : PersistentStateComponent<SitatameSettings.PersistedState> {

    data class PersistedState(
        var sitatameHomeOverride: String = "",
        /** Empty string means auto-detect (origin/HEAD → origin/main fallback). */
        var baseRef: String = "",
    )

    private var myState = PersistedState()

    override fun getState(): PersistedState = myState

    override fun loadState(state: PersistedState) {
        XmlSerializerUtil.copyBean(state, myState)
    }

    companion object {
        fun getInstance(): SitatameSettings =
            ApplicationManager.getApplication().getService(SitatameSettings::class.java)
    }
}
