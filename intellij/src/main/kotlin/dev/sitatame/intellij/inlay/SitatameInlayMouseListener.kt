package dev.sitatame.intellij.inlay

import com.intellij.openapi.editor.Inlay
import com.intellij.openapi.editor.event.EditorMouseEvent
import com.intellij.openapi.editor.event.EditorMouseListener

/**
 * Routes mouse clicks on the editor content component to the
 * "Resolve / Reopen" button painted inside each [SitatameCommentInlayRenderer].
 *
 * One instance of this listener is registered per editor (not per inlay).
 * It walks the list of tracked inlays and delegates hit-testing to each
 * renderer. Registered via [com.intellij.openapi.editor.EditorFactory.addEditorFactoryListener]
 * (indirectly, via [SitatameEditorFactoryListener] → [SitatameInlayService]).
 *
 * **Thread model**: [EditorMouseListener.mouseClicked] is called on the EDT.
 */
internal class SitatameInlayMouseListener(
    private val inlays: List<Inlay<SitatameCommentInlayRenderer>>,
) : EditorMouseListener {

    override fun mouseClicked(event: EditorMouseEvent) {
        val point = event.mouseEvent.point
        for (inlay in inlays) {
            val region = inlay.bounds ?: continue
            val renderer = inlay.renderer
            val hitIndex = renderer.hitTestButton(
                targetRegion = region,
                clickX = point.x,
                clickY = point.y,
            )
            if (hitIndex >= 0) {
                renderer.onToggle(hitIndex)
                inlay.update()
                event.mouseEvent.consume()
                return  // one click → one action
            }
        }
    }
}
