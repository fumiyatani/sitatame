package dev.sitatame.intellij.inlay

import com.intellij.openapi.editor.event.EditorFactoryEvent
import com.intellij.openapi.editor.event.EditorFactoryListener

/**
 * Hooks the IntelliJ editor lifecycle to attach sitatame block inlays.
 *
 * Registered in `plugin.xml` as an `editorFactoryListener` extension.
 * The platform calls [editorCreated] on the EDT immediately after a new editor
 * is created (before it is painted), which is the earliest safe moment to add
 * inlay elements.
 *
 * [SitatameInlayService.attachInlays] schedules a background task for I/O and
 * switches back to EDT for InlayModel mutations — this listener merely
 * delegates to that service.
 */
class SitatameEditorFactoryListener : EditorFactoryListener {

    override fun editorCreated(event: EditorFactoryEvent) {
        SitatameInlayService.attachInlays(event.editor)
    }

    // editorReleased: no cleanup needed; inlays are owned by the editor and
    // disposed automatically when the editor is disposed.
}
