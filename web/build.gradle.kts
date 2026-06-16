import org.jetbrains.kotlin.gradle.ExperimentalWasmDsl
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    kotlin("multiplatform") version "2.1.10"
    kotlin("plugin.serialization") version "2.1.10"
    id("org.jetbrains.compose") version "1.7.3"
    id("org.jetbrains.kotlin.plugin.compose") version "2.1.10"
}

group = "dev.sitatame.web"
version = "0.2.0"

repositories {
    mavenCentral()
    google()
    maven("https://maven.pkg.jetbrains.space/public/p/compose/dev")
}

// Pin Ktor / serialization versions in one place so jvmMain (server) and
// wasmJsMain (client) stay aligned.
val ktorVersion = "3.0.3"
val kotlinxSerializationVersion = "1.7.3"
val kotlinxCoroutinesVersion = "1.9.0"

kotlin {
    jvmToolchain(21)

    jvm {
        compilations.all {
            compileTaskProvider.configure {
                compilerOptions {
                    jvmTarget.set(JvmTarget.JVM_21)
                }
            }
        }
    }

    @OptIn(ExperimentalWasmDsl::class)
    wasmJs {
        moduleName = "sitatame-web"
        browser {
            commonWebpackConfig {
                outputFileName = "sitatame-web.js"
            }
        }
        binaries.executable()
    }

    sourceSets {
        val commonMain by getting {
            dependencies {
                implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:$kotlinxSerializationVersion")
                implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:$kotlinxCoroutinesVersion")
            }
        }
        val commonTest by getting {
            dependencies {
                implementation(kotlin("test"))
            }
        }

        val jvmMain by getting {
            dependencies {
                // The Compose compiler plugin is applied project-wide, so the
                // JVM compilation also needs the runtime on its classpath even
                // though Compose UI code currently lives in wasmJsMain.
                implementation(compose.runtime)

                // YAML round-trip (existing PoC).
                implementation("org.snakeyaml:snakeyaml-engine:2.9")

                // Ktor server.
                implementation("io.ktor:ktor-server-core:$ktorVersion")
                implementation("io.ktor:ktor-server-netty:$ktorVersion")
                implementation("io.ktor:ktor-server-content-negotiation:$ktorVersion")
                implementation("io.ktor:ktor-server-status-pages:$ktorVersion")
                implementation("io.ktor:ktor-server-cors:$ktorVersion")
                implementation("io.ktor:ktor-server-call-logging:$ktorVersion")
                implementation("io.ktor:ktor-serialization-kotlinx-json:$ktorVersion")

                // Logging backend for Ktor's slf4j calls — keep it minimal.
                implementation("org.slf4j:slf4j-simple:2.0.16")
            }
        }
        val jvmTest by getting {
            dependencies {
                implementation(platform("org.junit:junit-bom:5.11.4"))
                implementation("org.junit.jupiter:junit-jupiter")
                runtimeOnly("org.junit.platform:junit-platform-launcher")
                implementation("io.ktor:ktor-server-test-host:$ktorVersion")
                implementation("io.ktor:ktor-client-content-negotiation:$ktorVersion")
            }
        }

        val wasmJsMain by getting {
            dependencies {
                implementation(compose.runtime)
                implementation(compose.foundation)
                implementation(compose.material3)
                implementation(compose.ui)
                // No HTTP client dependency: the wasmJs target consumes
                // `window.fetch` directly via JS interop (see ApiClient.kt).
                // ktor-client-js does not have a wasmJs variant in 3.0.x; the
                // browser fetch call is small enough that pulling the client in
                // is not worth the friction.
            }
        }
    }
}

// The JUnit Platform engine has to be enabled explicitly for the JVM target's
// test task; the Multiplatform DSL doesn't wire that up automatically.
tasks.named<Test>("jvmTest") {
    useJUnitPlatform()
    testLogging {
        events("passed", "skipped", "failed")
        showStandardStreams = true
    }
}

// `./gradlew :web:jvmFatJar` builds a self-contained single JAR (all JVM
// runtime dependencies bundled) that can be launched without Gradle:
//   java -jar web/build/libs/sitatame-web-<version>-fat.jar [--repo /path] [--base ref]
//
// Duplicate entries in META-INF are handled as follows:
//   - EXCLUDE strategy is used as the default (first-in wins).
//   - META-INF/services/* (Java SPI descriptors) are merged by concatenation so
//     that multiple providers for the same interface are all retained.  Plain
//     EXCLUDE would silently drop every provider after the first, breaking SLF4J
//     bindings, Ktor engine discovery, and any future SPI-registered extension.
//   - Signature files (*.SF, *.DSA, *.RSA, INDEX.LIST) are stripped so the JVM
//     does not reject the jar for having a broken signature.
tasks.register<Jar>("jvmFatJar") {
    group = "build"
    description = "Assembles a self-contained fat JAR with all JVM runtime dependencies bundled."
    archiveClassifier.set("fat")

    manifest {
        attributes("Main-Class" to "dev.sitatame.web.server.ServerKt")
    }

    // EXCLUDE is the default for everything except META-INF/services/*.
    // SPI descriptors are handled separately below via explicit concatenation.
    duplicatesStrategy = DuplicatesStrategy.EXCLUDE

    val jvmMainCompilation = kotlin.jvm().compilations.getByName("main")
    // Exclude SPI descriptors and signature files per source so the top-level
    // exclude does not accidentally suppress the merged-services directory added
    // further below.
    from(jvmMainCompilation.output.allOutputs) {
        exclude(
            "META-INF/*.SF",
            "META-INF/*.DSA",
            "META-INF/*.RSA",
            "META-INF/INDEX.LIST",
            "META-INF/services/*",
        )
    }

    val runtimeCp = configurations.named("jvmRuntimeClasspath")
    dependsOn(runtimeCp)
    from({
        runtimeCp.get()
            .filter { it.name.endsWith(".jar") }
            .map { zipTree(it) }
    }) {
        exclude(
            "META-INF/*.SF",
            "META-INF/*.DSA",
            "META-INF/*.RSA",
            "META-INF/INDEX.LIST",
            "META-INF/services/*",
        )
    }

    // Merge SPI descriptors: the `mergeSpiDescriptors` task below collects
    // every META-INF/services/<interface> from the compiled outputs and all
    // runtime jars, concatenates lines per interface name, and writes the
    // merged files into `build/tmp/mergedServices/META-INF/services/`.
    // This source is added without an exclude so the merged descriptors are
    // included verbatim; no provider is silently dropped.
    val mergedServicesDir = layout.buildDirectory.dir("tmp/mergedServices")
    dependsOn("mergeSpiDescriptors")
    from(mergedServicesDir)
}

// `mergeSpiDescriptors` collects META-INF/services/* from all JVM runtime
// dependencies and the compiled jvmMain outputs, concatenates lines per
// interface name (de-duplicating within each file), and writes the merged
// descriptors into build/tmp/mergedServices/META-INF/services/.  The
// `jvmFatJar` task depends on this task and includes the output directory so
// that all SPI providers are retained — plain EXCLUDE would silently drop
// every provider after the first.
tasks.register("mergeSpiDescriptors") {
    group = "build"
    description = "Merges META-INF/services/* SPI descriptors from all JVM runtime jars."

    val jvmMainCompilation = kotlin.jvm().compilations.getByName("main")
    val runtimeCp = configurations.named("jvmRuntimeClasspath")
    dependsOn(runtimeCp)
    dependsOn(jvmMainCompilation.compileAllTaskName)

    val outDir = layout.buildDirectory.dir("tmp/mergedServices/META-INF/services")
    outputs.dir(outDir)

    doLast {
        val servicesMap = mutableMapOf<String, MutableList<String>>()

        fun collectFrom(tree: FileTree) {
            tree.matching { include("META-INF/services/*") }.forEach { f ->
                val lines = f.readLines().filter { it.isNotBlank() }
                servicesMap.getOrPut(f.name) { mutableListOf() }.addAll(lines)
            }
        }

        // From compiled class outputs (jvmMain).
        collectFrom(jvmMainCompilation.output.allOutputs.asFileTree)

        // From all runtime dependency jars.
        runtimeCp.get()
            .filter { it.name.endsWith(".jar") }
            .forEach { jar -> collectFrom(zipTree(jar)) }

        val dest = outDir.get().asFile
        dest.mkdirs()
        // De-duplicate per interface: preserve order but drop repeated class names.
        servicesMap.forEach { (iface, providers) ->
            dest.resolve(iface).writeText(providers.distinct().joinToString("\n") + "\n")
        }
    }
}

// `./gradlew :web:run` runs the Ktor backend. The Kotlin Multiplatform plugin
// does not register the `application` plugin automatically; we wire a JavaExec
// task directly against the JVM compilation outputs to avoid the friction.
// Bundle the Compose Wasm distribution into the JVM server's resources so a
// single `:run` serves BOTH the UI (static "/") and the API (/api/v1/...) on
// one localhost port. Wiring it through jvmProcessResources means the output
// lands under build/ (build/processedResources/jvm/main/static) — src/ stays
// clean, so the 12MB+ of generated wasm/js never needs gitignoring.
//
// Because the JavaExec `run` task's classpath includes the JVM compilation's
// resource output, depending on this task transitively pulls the wasm build,
// giving the desired `:run` -> dist build -> serve chain with no extra step.
val wasmJsBrowserDistribution = tasks.named("wasmJsBrowserDistribution")
tasks.named<Copy>(kotlin.jvm().compilations.getByName("main").processResourcesTaskName) {
    dependsOn(wasmJsBrowserDistribution)
    from(wasmJsBrowserDistribution.map { it.outputs.files }) {
        into("static")
    }
}

tasks.register<JavaExec>("run") {
    group = "application"
    description = "Build the Compose Wasm UI, then run the Ktor server (UI + API on one port)."
    mainClass.set("dev.sitatame.web.server.ServerKt")
    val jvmMainCompilation = kotlin.jvm().compilations.getByName("main")
    classpath = files(
        jvmMainCompilation.output.allOutputs,
        jvmMainCompilation.runtimeDependencyFiles,
    )
    // Forward --args="..." from `./gradlew :web:run --args="--repo /path --base ref"`.
    // The `args` property on JavaExec accepts a vararg/list; the Gradle `--args`
    // flag is a single string that we split on whitespace here.  Quoting and
    // shell escaping inside --args are therefore not supported (single tokens only).
    // For paths with spaces use the env-var path instead: SITATAME_REPO / SITATAME_BASE.
    val runArgs = findProperty("args")?.toString()
    if (!runArgs.isNullOrBlank()) {
        args(runArgs.trim().split(Regex("\\s+")))
    }
    // Also support per-property shorthands for convenience:
    //   ./gradlew :web:run -PrepoPath=/path/to/repo -PbaseRef=origin/develop
    val repoProp = findProperty("repoPath")?.toString()
    val baseProp = findProperty("baseRef")?.toString()
    if (!repoProp.isNullOrBlank()) {
        args("--repo", repoProp.trim())
    }
    if (!baseProp.isNullOrBlank()) {
        args("--base", baseProp.trim())
    }
}
