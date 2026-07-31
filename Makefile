# splinterd — build / cross-compile / deploy
BINARY        := splinterd
PKG           := ./cmd/splinterd
DIST          := dist
UCONSOLE_HOST ?= eris@192.168.68.60
VERSION       := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS       := -X main.version=$(VERSION)
export GOTOOLCHAIN := local

.PHONY: build build-uconsole deploy test fmt vet check clean

## build: native build for the host
build:
	go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY) $(PKG)

## build-uconsole: static aarch64 binary for the uConsole (no cgo)
build-uconsole:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-arm64 $(PKG)

## deploy: build-uconsole then scp to the device (override UCONSOLE_HOST as needed)
deploy: build-uconsole
	scp $(DIST)/$(BINARY)-arm64 $(UCONSOLE_HOST):~/

## test / fmt / vet / check
test:
	go test ./...

fmt:
	gofmt -l -w .

vet:
	go vet ./...

check: fmt vet test

clean:
	rm -rf $(DIST)
