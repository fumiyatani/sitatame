.PHONY: build test vet fmt install build-all clean

BIN := sitatame
PKG := ./...

build:
	go build -o $(BIN) .

test:
	go test $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -w .

install:
	go install ./...

build-all:
	GOOS=darwin  GOARCH=amd64 go build -o dist/$(BIN)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -o dist/$(BIN)-darwin-arm64 .
	GOOS=linux   GOARCH=amd64 go build -o dist/$(BIN)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -o dist/$(BIN)-linux-arm64 .

clean:
	rm -f $(BIN)
	rm -rf dist
