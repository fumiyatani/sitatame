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

// Minimum JDK 17 keeps local dev and CI aligned without auto-provisioning a
// toolchain (sandboxed runs cannot download JDKs). CI provisions Temurin 21
// and that satisfies the >=17 constraint at the bytecode level.
kotlin {
    jvmToolchain(17)
}

tasks.test {
    useJUnitPlatform()
    testLogging {
        events("passed", "skipped", "failed")
        showStandardStreams = true
    }
}
