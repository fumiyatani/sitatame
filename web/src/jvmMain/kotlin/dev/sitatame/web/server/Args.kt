package dev.sitatame.web.server

import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths as NioPaths

/**
 * Startup arguments for the Ktor backend. Resolved with the priority order
 *   CLI arg > environment variable > built-in default.
 *
 * Supported CLI flags (long-form only, space-separated — `--flag=value` is NOT supported):
 *   --repo <path>   Path to the git repository root. Overrides SITATAME_REPO.
 *   --base <ref>    Base ref for `git diff <ref>..HEAD`. Overrides SITATAME_BASE.
 *   --help          Print usage and exit(0).
 *
 * Environment variables:
 *   SITATAME_REPO   Same as --repo. Ignored when --repo is given.
 *   SITATAME_BASE   Same as --base. Ignored when --base is given.
 */
data class ServerArgs(
    val repoPath: Path,
    val baseRef: String,
    /** Indicates which source provided [baseRef], used to decide whether to validate
     *  at startup (explicit) or degrade gracefully (default). */
    val baseRefSource: Source = Source.DEFAULT,
)

/** Where a resolved value came from (CLI flag > env var > built-in default). */
enum class Source { CLI, ENV, DEFAULT }

/** Thrown when [parseArgs] finds a usage error. Message is user-visible. */
class ArgParseException(message: String) : Exception(message)

const val USAGE: String = """
Usage: sitatame-web [--repo <path>] [--base <ref>] [--help]

Options:
  --repo <path>   Path to the target git repository root.
                  Env fallback: SITATAME_REPO
                  Default: current working directory (user.dir)
  --base <ref>    Base ref for `git diff <ref>..HEAD`.
                  Env fallback: SITATAME_BASE
                  Default: origin/main
  --help          Print this help and exit.

Note: flags must be space-separated (--repo /path), not --repo=/path.

Examples:
  # Review another project
  cd web && ./gradlew :run --args="--repo /path/to/other-project"

  # Custom base ref
  cd web && ./gradlew :run --args="--repo /path/to/project --base origin/develop"

  # Via environment variables
  SITATAME_REPO=/path/to/project SITATAME_BASE=origin/develop cd web && ./gradlew :run
"""

/** Set of known long-form flags, used to detect flag-shaped values passed after --repo / --base. */
private val KNOWN_FLAGS = setOf("--repo", "--base", "--help", "-h")

/**
 * Parse [args] and resolve the effective [ServerArgs].
 *
 * Resolution order for each field:
 *  1. CLI flag (`--repo` / `--base`) — space-separated only; `--flag=value` is NOT supported
 *  2. Environment variable (`SITATAME_REPO` / `SITATAME_BASE`)
 *  3. Built-in default (`user.dir` / `"origin/main"`)
 *
 * @param args     CLI argument array (typically from `main(args: Array<String>)`)
 * @param envLookup Injectable env provider so tests can avoid real env reads.
 * @throws ArgParseException on unknown flag, missing argument value, blank value, or
 *                           flag-shaped value (next token starts with `--`).
 */
fun parseArgs(
    args: Array<String>,
    envLookup: (String) -> String? = System::getenv,
): ServerArgs {
    var repoArg: String? = null
    var baseArg: String? = null

    var i = 0
    while (i < args.size) {
        when (val flag = args[i]) {
            "--help", "-h" -> {
                print(USAGE.trimIndent() + "\n")
                // Throw a dedicated subclass so callers (main) can exit(0) cleanly
                // without coupling parseArgs to System.exit.
                throw HelpRequestedException()
            }
            "--repo" -> {
                val value = args.getOrNull(++i)
                    ?: throw ArgParseException("--repo requires a path argument")
                if (value in KNOWN_FLAGS || value.startsWith("--")) {
                    throw ArgParseException(
                        "--repo requires a path argument, but got flag-like token: $value"
                    )
                }
                if (value.isBlank()) {
                    throw ArgParseException("--repo value must not be blank")
                }
                repoArg = value
            }
            "--base" -> {
                val value = args.getOrNull(++i)
                    ?: throw ArgParseException("--base requires a ref argument")
                if (value in KNOWN_FLAGS || value.startsWith("--")) {
                    throw ArgParseException(
                        "--base requires a ref argument, but got flag-like token: $value"
                    )
                }
                if (value.isBlank()) {
                    throw ArgParseException("--base value must not be blank")
                }
                baseArg = value
            }
            else -> {
                if (flag.startsWith("-")) {
                    throw ArgParseException("Unknown flag: $flag\n${USAGE.trimIndent()}")
                }
                // Positional arguments are not used; skip silently so callers
                // passing Gradle system properties don't break.
            }
        }
        i++
    }

    val repoStr = repoArg
        ?: envLookup("SITATAME_REPO")?.takeIf { it.isNotBlank() }
        ?: System.getProperty("user.dir")
        ?: "."

    val (baseRef, baseRefSource) = when {
        baseArg != null -> baseArg to Source.CLI
        else -> {
            val envVal = envLookup("SITATAME_BASE")?.takeIf { it.isNotBlank() }
            if (envVal != null) envVal to Source.ENV else DEFAULT_BASE_REF to Source.DEFAULT
        }
    }

    val repoPath = try {
        NioPaths.get(repoStr).toAbsolutePath().normalize()
    } catch (_: Exception) {
        NioPaths.get(repoStr)
    }
    validateRepoPath(repoPath)

    return ServerArgs(repoPath = repoPath, baseRef = baseRef, baseRefSource = baseRefSource)
}

/**
 * Validates that [repoPath] is an existing directory that contains a `.git`
 * entry (file or directory). A plain file named `.git` is accepted because git
 * worktrees use it as a redirecting file.
 *
 * @throws ArgParseException with a user-friendly message when the check fails.
 */
fun validateRepoPath(repoPath: Path) {
    if (!Files.isDirectory(repoPath)) {
        throw ArgParseException(
            "Repository path does not exist or is not a directory: $repoPath\n" +
                "Hint: pass the root of a git repository via --repo or SITATAME_REPO."
        )
    }
    if (!Files.exists(repoPath.resolve(".git"))) {
        throw ArgParseException(
            "No .git found in: $repoPath\n" +
                "The --repo / SITATAME_REPO path must point to a git repository root."
        )
    }
}

/** Signals that --help was requested; [main] should exit(0). */
class HelpRequestedException : Exception("--help")
