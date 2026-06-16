package dev.sitatame.intellij.storage

import com.intellij.util.messages.Topic

/**
 * Application-level message bus topic published whenever a mutation
 * (addComment / toggleComment / removeComment) succeeds.
 *
 * Tool windows subscribe on this topic (scoped to their Disposable) and
 * refresh only when [repoRoot] and [branch] match their own project context.
 */
fun interface ReviewChangedListener {
    fun onChanged(repoRoot: String, branch: String)
}

val REVIEW_CHANGED_TOPIC: Topic<ReviewChangedListener> =
    Topic.create("Sitatame Review Changed", ReviewChangedListener::class.java)
