BINARY_NAME=tt-wrapper
CMD_DIR=./cmd/tt-wrapper
BIN_DIR=./bin

.PHONY: all build clean test coverage lint run install

all: build

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)

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
	go install $(CMD_DIR)
