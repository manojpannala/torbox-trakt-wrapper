BINARY_NAME=tt-wrapper
CMD_DIR=./cmd/tt-wrapper
BIN_DIR=./bin
PKG=github.com/manojpannala/torbox-trakt-wrapper/pkg/config
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE?=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: all build clean test coverage lint run install

all: build

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)

run:
	go run $(CMD_DIR)

test:
	go test -v -race ./...

coverage:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -html=coverage.txt -o coverage.html

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BIN_DIR) dist coverage.txt coverage.html

install:
	go install -ldflags="$(LDFLAGS)" $(CMD_DIR)
