import org.jetbrains.intellij.platform.gradle.TestFrameworkType

plugins {
    // Kotlin 2.1.x matches the Web UI PoC (PR #65) and what IntelliJ 2024.3
    // bundles. Keep the two lines in sync so a future codec change doesn't
    // accidentally drift between routes.
    kotlin("jvm") version "2.1.10"
    // Required for @Serializable annotation processing used in RescuePayload.
    kotlin("plugin.serialization") version "2.1.10"
    id("org.jetbrains.intellij.platform") version "2.1.0"
}

group = "dev.sitatame"
version = "0.1.0"

repositories {
    mavenCentral()
    intellijPlatform {
        defaultRepositories()
    }
}

// IntelliJ 2024.3 bundles JBR 21. Match that so the compiled plugin loads
// without complaining about class file version mismatch.
kotlin {
    jvmToolchain(21)
}

dependencies {
    // snakeyaml-engine 2.9 — same version the Web UI PoC uses. The Node tree
    // API preserves comments, key order and unknown keys, which the
    // bit-exact round-trip test relies on. Plain snakeyaml drops comments.
    //
    // ClassLoader risk: IntelliJ ships an older snakeyaml.jar internally but
    // NOT snakeyaml-engine. The plugin classloader (PluginClassLoader) prefers
    // bundled libs over platform ones, so a direct dependency here is enough;
    // no shadowJar relocation is needed for snakeyaml-engine specifically.
    // If this turns out to clash in practice we can switch to a relocated
    // shadow JAR in Phase 2.
    implementation("org.snakeyaml:snakeyaml-engine:2.9")

    // kotlinx-serialization-json is used to marshal RescuePayload (the rescue
    // JSON written on Codec.encode failure). We use a DTO class rather than
    // annotating the mutable Review model directly, because Review holds
    // snakeyaml Node references that are not serializable.
    //
    // IntelliJ 2024.3 bundles kotlinx-serialization-core internally.
    // The PluginClassLoader gives plugin classes priority over platform classes,
    // so declaring an explicit implementation dependency here takes precedence.
    // The version is pinned to match the Kotlin 2.1.x compiler plugin above.
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")

    intellijPlatform {
        // Target IntelliJ IDEA Community 2024.3. Build number 243.x. Android
        // Studio 2024.3.x (Jellyfish) is build 243-based too and so is
        // covered by the same target without a separate distribution.
        intellijIdeaCommunity("2024.3")

        // Bundled Git4Idea gives us GitRepositoryManager for repo root and
        // branch detection without shelling out to `git`.
        bundledPlugin("Git4Idea")
        bundledModule("intellij.platform.vcs.dvcs.impl")

        // Test framework + platform tests. Needed for BasePlatformTestCase
        // in the action tests.
        testFramework(TestFrameworkType.Platform)

        // Required by the IntelliJ Platform Gradle Plugin's instrumentCode
        // task, which runs as part of test/buildPlugin.
        instrumentationTools()
    }

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.opentest4j:opentest4j:1.3.0")
}

intellijPlatform {
    pluginConfiguration {
        // sinceBuild matches the IDE we depend on. untilBuild is intentionally
        // wide (251.*) so 2024.3+, 2025.1.x IDEs and the Android Studio
        // releases that track them all load the plugin. We will narrow this
        // in Phase 2 once we know which APIs are actually unstable.
        ideaVersion {
            sinceBuild = "243"
            untilBuild = "251.*"
        }
    }

    // The Plugin Verifier downloads alternate IDE distributions to validate
    // the plugin against. Sandboxed CI may not be able to reach those mirrors,
    // so we list only the primary target. CI handles wider matrix.
    pluginVerification {
        ides {
            recommended()
        }
    }
}

tasks {
    test {
        useJUnit()
        // BasePlatformTestCase needs the IntelliJ test framework on the
        // classpath; the intellijPlatform DSL above wires that in.
        testLogging {
            events("passed", "skipped", "failed")
            showStandardStreams = true
        }
    }

    // Match the Web UI module's Kotlin compiler args so warnings surface
    // identically.
    withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>().configureEach {
        compilerOptions {
            jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_21)
        }
    }
}
