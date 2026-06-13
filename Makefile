.PHONY: build test test-tui-e2e bench update-golden update-golden-tui-e2e vet fmt install build-all clean

BIN := sitatame
PKG := ./...

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
