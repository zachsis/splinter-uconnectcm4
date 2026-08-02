# Helm of Hades (hohd) — build / install
#
# Primary flow: clone on the Linux/BLE host that will run hohd, then:
#   make                 # native build -> dist/hohd
#   sudo make install    # -> /usr/local/bin/hohd   (run: sudo hohd --dashboard)
# Optional: `sudo make setcap` lets hohd run without sudo.
# Requires Go >= 1.24.

BINARY  := hohd
PKG     := ./cmd/hohd
DIST    := dist
PREFIX  ?= /usr/local
DESTDIR ?=
GOARCH  ?= arm64
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.DEFAULT_GOAL := build
.PHONY: build install uninstall setcap build-cross deploy check-uconsole-host test fmt vet check clean

## build: native build for THIS machine (run on the device that will run hohd)
build:
	go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY) $(PKG)

## install: install the native binary to $(PREFIX)/bin (needs sudo for /usr/local)
install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 0755 $(DIST)/$(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)
	@echo "installed $(DESTDIR)$(PREFIX)/bin/$(BINARY) — run: sudo $(BINARY) --dashboard"
	@echo "(optional: 'sudo make setcap' to run $(BINARY) without sudo)"

## uninstall: remove the installed binary
uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)

## setcap: grant CAP_NET_RAW+CAP_NET_ADMIN so hohd runs without sudo (needs sudo)
setcap:
	@command -v setcap >/dev/null || { echo "setcap not found — install it first (Debian/uConsole: sudo apt install libcap2-bin)"; exit 2; }
	setcap 'cap_net_raw,cap_net_admin+eip' $(DESTDIR)$(PREFIX)/bin/$(BINARY)

## --- maintainer: cross-compile from another machine (optional) ---

## build-cross: static cross-compiled binary (override GOARCH, default arm64)
build-cross:
	GOOS=linux GOARCH=$(GOARCH) CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-$(GOARCH) $(PKG)

## deploy: cross-build then scp to a host you set (UCONSOLE_HOST=user@host)
deploy: check-uconsole-host build-cross
	scp $(DIST)/$(BINARY)-$(GOARCH) $(UCONSOLE_HOST):~/

check-uconsole-host:
	@test -n "$(UCONSOLE_HOST)" || { echo "set UCONSOLE_HOST=user@host, e.g. UCONSOLE_HOST=pi@10.0.0.5 make deploy"; exit 2; }

## --- checks ---

test:
	go test ./...

fmt:
	gofmt -l -w .

vet:
	go vet ./...

check: fmt vet test

clean:
	rm -rf $(DIST)
