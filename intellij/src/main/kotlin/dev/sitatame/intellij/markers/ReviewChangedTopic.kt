package dev.sitatame.intellij.markers

import com.intellij.util.messages.Topic

/**
 * Application-level message bus topic published whenever the in-memory review
 * store is mutated (comment added, resolved, etc.).
 *
 * Subscribers call [DaemonCodeAnalyzer.restart] on the affected project so
 * that [SitatameLineMarkerProvider] picks up the fresh comment list without
 * requiring a manual refresh.
 *
 * Publish: action layer (AddCommentAction, ResolveCommentAction) after a
 *          successful store mutation, on the EDT (ProgressManager.invokeLater).
 * Subscribe: [SitatameProjectActivity] via messageBus.connect(project disposable).
 */
fun interface ReviewChangedListener {
    fun reviewChanged()
}

object ReviewChangedTopic {
    @JvmField
    val TOPIC: Topic<ReviewChangedListener> =
        Topic.create("sitatame.review.changed", ReviewChangedListener::class.java)
}
