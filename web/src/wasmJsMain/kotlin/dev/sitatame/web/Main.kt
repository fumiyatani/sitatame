package dev.sitatame.web

import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.window.CanvasBasedWindow
import dev.sitatame.web.ui.SitatameApp

/**
 * Wasm entry point. CanvasBasedWindow targets the `<canvas id="ComposeTarget">`
 * declared in index.html. Compose for Web's Wasm renderer paints into that
 * single canvas — there is no DOM tree to manage.
 */
@OptIn(ExperimentalComposeUiApi::class)
fun main() {
    CanvasBasedWindow(canvasElementId = "ComposeTarget") {
        SitatameApp()
    }
}
