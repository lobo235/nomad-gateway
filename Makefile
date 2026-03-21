export PATH := $(HOME)/bin/go/bin:$(PATH)

BINARY  := nomad-gateway
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test cover lint run clean

build:
	go build -trimpath $(LDFLAGS) -o $(BINARY) ./cmd/server

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

lint:
	golangci-lint run ./...

run:
	go run ./cmd/server

clean:
	rm -f $(BINARY) coverage.out
