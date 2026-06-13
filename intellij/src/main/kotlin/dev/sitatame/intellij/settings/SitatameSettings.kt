package dev.sitatame.intellij.settings

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
 *  - Base ref — used by Phase 2 when we wire up diff context, kept here so a
 *    Phase 1 → Phase 2 upgrade doesn't migrate state.
 */
@Service(Service.Level.APP)
@State(name = "SitatameSettings", storages = [Storage("sitatame.xml")])
class SitatameSettings : PersistentStateComponent<SitatameSettings.PersistedState> {

    data class PersistedState(
        var sitatameHomeOverride: String = "",
        var baseRef: String = "origin/main",
    )

    private var myState = PersistedState()

    override fun getState(): PersistedState = myState

    override fun loadState(state: PersistedState) {
        XmlSerializerUtil.copyBean(state, myState)
    }
}
