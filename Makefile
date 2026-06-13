.PHONY: build test bench update-golden vet fmt install build-all clean web-fixtures

BIN := sitatame
PKG := ./...
WEB_FIXTURE_DIR := web/fixtures

build:
	go build -o $(BIN) .

test:
	go test $(PKG)

bench:
	go test -run=^$$ -bench=. -benchmem ./internal/tui/...

update-golden:
	go test ./internal/tui/ -update-golden

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
