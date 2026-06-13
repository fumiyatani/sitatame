plugins {
    kotlin("jvm") version "2.1.10"
}

group = "dev.sitatame.web"
version = "0.0.1-SNAPSHOT"

repositories {
    mavenCentral()
}

dependencies {
    // snakeyaml-engine 2.x — Node tree API needed for comment / order
    // preservation. Plain snakeyaml's YamlReader/Writer drops comments.
    implementation("org.snakeyaml:snakeyaml-engine:2.9")

    testImplementation(platform("org.junit:junit-bom:5.11.4"))
    testImplementation("org.junit.jupiter:junit-jupiter")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

// JDK 21 matches what CI provisions via actions/setup-java (Temurin 21). If
// the local environment only has JDK 17, Gradle's toolchain auto-provisioning
// will try to download Temurin 21 — set `org.gradle.java.installations.auto-detect`
// or install JDK 21 locally to avoid that.
kotlin {
    jvmToolchain(21)
}

tasks.test {
    useJUnitPlatform()
    testLogging {
        events("passed", "skipped", "failed")
        showStandardStreams = true
    }
}
