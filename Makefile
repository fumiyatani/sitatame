.PHONY: build test test-tui-e2e bench update-golden update-golden-tui-e2e vet fmt install build-all clean web-fixtures web web-jar intellij intellij-test intellij-verify intellij-run

BIN := sitatame
PKG := ./...
WEB_FIXTURE_DIR := web/fixtures

build:
	go build -o $(BIN) .

# Default unit + integration suite. Does NOT include the teatest scenario
# suite under internal/tui/ or the pty smoke under internal/replay/; those
# are gated behind the `tui_e2e` build tag — see `make test-tui-e2e`.
test:
	go test $(PKG)

# Run every test gated on the `tui_e2e` build tag (teatest scenarios + pty
# smoke). Optional locally; in CI it runs as a non-required job.
# See docs/tui-status.md for the maintenance-mode rationale.
test-tui-e2e:
	go test -tags tui_e2e $(PKG)

bench:
	go test -run=^$$ -bench=. -benchmem ./internal/tui/...

# Update the classic golden snapshots under internal/tui/testdata/.
update-golden:
	go test ./internal/tui/ -update-golden

# Update the scenario goldens under internal/tui/testdata/scenarios/. The
# scenario runner is gated behind `tui_e2e`, so `-update-golden` only
# touches the scenario files when that build tag is on.
update-golden-tui-e2e:
	go test -tags tui_e2e ./internal/tui/ -update-golden

vet:
	go vet $(PKG)

fmt:
	gofmt -w .

install:
	go install ./...

build-all:
	mkdir -p dist
	GOOS=darwin  GOARCH=amd64 go build -o dist/$(BIN)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -o dist/$(BIN)-darwin-arm64 .
	GOOS=linux   GOARCH=amd64 go build -o dist/$(BIN)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -o dist/$(BIN)-linux-arm64 .

clean:
	rm -f $(BIN)
	rm -rf dist

# Regenerate the YAML round-trip fixtures consumed by web/ (Kotlin Web UI PoC).
# See cmd/yamlfixture/main.go for the input set and rationale.
web-fixtures:
	mkdir -p $(WEB_FIXTURE_DIR)
	go run ./cmd/yamlfixture -out $(WEB_FIXTURE_DIR)

# Build the Compose Wasm UI and launch the Web UI on a single localhost port.
# `:run` bundles the wasm dist into the Ktor server's resources, so one process
# serves the UI ("/") and the API ("/api/v1/..."). Open the printed
# SITATAME_WEB_URL line. First build is slow (wasm compile); reruns are cached.
web:
	cd web && ./gradlew :run

# Build a self-contained fat jar (Ktor server + Wasm UI + all JVM deps bundled).
# The resulting jar can be run without Gradle — only JDK 21 and git are required:
#   java -jar web/build/libs/sitatame-web-*-fat.jar --repo /path/to/repo
web-jar:
	cd web && ./gradlew :jvmFatJar --no-daemon
	@echo "→ $(shell ls web/build/libs/sitatame-web-*-fat.jar 2>/dev/null | head -1)"

# Build the IntelliJ Plugin zip. Drop the resulting file into
# `Settings → Plugins → ⚙ → Install Plugin from Disk…` in any 2024.3+
# IntelliJ IDEA / Android Studio to load the latest local build.
intellij:
	cd intellij && ./gradlew buildPlugin
	@echo "→ $(shell ls intellij/build/distributions/sitatame-intellij-*.zip 2>/dev/null | head -1)"

# Run the IntelliJ plugin unit + integration suite.
intellij-test:
	cd intellij && ./gradlew test

# JetBrains Plugin Verifier — checks the plugin against the IDE range
# declared in `intellij/build.gradle.kts`. Required to stay green before
# Marketplace publishing.
intellij-verify:
	cd intellij && ./gradlew verifyPlugin

# Launch a sandbox IDE with the plugin loaded for interactive testing.
# Slower than `make intellij` but iterates without re-installing the zip.
intellij-run:
	cd intellij && ./gradlew runIde
