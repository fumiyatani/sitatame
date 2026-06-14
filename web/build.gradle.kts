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
}
