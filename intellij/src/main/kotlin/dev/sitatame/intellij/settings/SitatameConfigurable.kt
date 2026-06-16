package dev.sitatame.intellij.settings

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.options.Configurable
import com.intellij.openapi.ui.TextFieldWithBrowseButton
import com.intellij.ui.components.JBLabel
import dev.sitatame.intellij.storage.ReviewStore
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import java.awt.Insets
import javax.swing.JComponent
import javax.swing.JPanel
import javax.swing.JTextField

/**
 * Settings UI under Preferences → Tools → sitatame review.
 *
 * Resetting the [ReviewStore] cache on apply ensures the next read uses the
 * new SITATAME_HOME without restarting the IDE.
 */
class SitatameConfigurable : Configurable {

    private val homeField = TextFieldWithBrowseButton(JTextField(40))
    private val baseRefField = JTextField(30)

    private val settings: SitatameSettings
        get() = ApplicationManager.getApplication().getService(SitatameSettings::class.java)

    private val store: ReviewStore
        get() = ApplicationManager.getApplication().getService(ReviewStore::class.java)

    override fun getDisplayName(): String = "sitatame review"

    override fun createComponent(): JComponent {
        val panel = JPanel(GridBagLayout())
        val gbc = GridBagConstraints().apply {
            insets = Insets(4, 4, 4, 4)
            anchor = GridBagConstraints.WEST
            fill = GridBagConstraints.HORIZONTAL
        }

        gbc.gridx = 0; gbc.gridy = 0; gbc.weightx = 0.0
        panel.add(JBLabel("SITATAME_HOME override (blank = default):"), gbc)
        gbc.gridx = 1; gbc.weightx = 1.0
        panel.add(homeField, gbc)

        gbc.gridx = 0; gbc.gridy = 1; gbc.weightx = 0.0
        panel.add(JBLabel("Base ref (blank = auto-detect):"), gbc)
        gbc.gridx = 1; gbc.weightx = 1.0
        panel.add(baseRefField, gbc)

        reset()
        return panel
    }

    override fun isModified(): Boolean {
        val s = settings.state
        return homeField.text != s.sitatameHomeOverride || baseRefField.text != s.baseRef
    }

    override fun apply() {
        val s = settings.state
        s.sitatameHomeOverride = homeField.text.trim()
        // Empty means auto-detect (origin/HEAD → origin/main): store as-is.
        // After apply, users should press Refresh in the tool window to pick up
        // the new base ref (auto-refresh wired in a follow-up PR after
        // REVIEW_CHANGED_TOPIC lands on main via PR #95/#96).
        s.baseRef = baseRefField.text.trim()
        // Reset the in-memory cache so subsequent reads honour the new home.
        store.invalidate()
    }

    override fun reset() {
        val s = settings.state
        homeField.text = s.sitatameHomeOverride
        baseRefField.text = s.baseRef
    }
}
