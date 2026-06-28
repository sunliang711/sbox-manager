SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

MODULE := github.com/sunliang711/sbox-manager
DIST_DIR ?= dist
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GOOS ?= $(shell go env GOOS 2>/dev/null || uname -s | tr A-Z a-z)
GOARCH ?= $(shell go env GOARCH 2>/dev/null || uname -m)
CGO_ENABLED ?= 1
LDFLAGS := -X '$(MODULE)/internal/version.Version=$(VERSION)' -X '$(MODULE)/internal/version.Commit=$(COMMIT)' -X '$(MODULE)/internal/version.BuildTime=$(BUILD_TIME)'

.PHONY: help fmt lint test build build-linux snapshot package checksums install-local clean

help: ; @printf '%s\n' \
	'Targets:' \
	'  make fmt             Format Go code' \
	'  make lint            Run static checks when tools are available' \
	'  make test            Run Go tests' \
	'  make build           Build sboxctl and sboxsub for GOOS/GOARCH' \
	'  make build-linux     Build Linux amd64 and arm64 binaries' \
	'  make snapshot        Build package for current git state' \
	'  make package         Build release tar.gz for GOOS/GOARCH' \
	'  make checksums       Generate dist/release/checksums.txt' \
	'  make install-local   Install dist/bin binaries to BINDIR' \
	'  make clean           Remove build outputs'

fmt: ; @if [ -f go.mod ]; then go fmt ./...; else echo 'go.mod not found; fmt waits for T01 project bootstrap'; fi

lint: ; @if [ -f go.mod ]; then if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; else echo 'golangci-lint not found; skipping optional lint'; fi; else echo 'go.mod not found; lint waits for T01 project bootstrap'; fi

test: ; @if [ -f go.mod ]; then go test ./...; else echo 'go.mod not found; test waits for T01 project bootstrap'; fi

build:
	@test -d cmd/sboxctl || { echo 'cmd/sboxctl not found; complete T01 project bootstrap first' >&2; exit 1; }
	@test -d cmd/sboxsub || { echo 'cmd/sboxsub not found; complete T01 project bootstrap first' >&2; exit 1; }
	@mkdir -p "$(DIST_DIR)/bin/$(GOOS)_$(GOARCH)"
	@GOOS="$(GOOS)" GOARCH="$(GOARCH)" CGO_ENABLED="$(CGO_ENABLED)" go build -trimpath -ldflags "$(LDFLAGS)" -o "$(DIST_DIR)/bin/$(GOOS)_$(GOARCH)/sboxctl" ./cmd/sboxctl
	@GOOS="$(GOOS)" GOARCH="$(GOARCH)" CGO_ENABLED="$(CGO_ENABLED)" go build -trimpath -ldflags "$(LDFLAGS)" -o "$(DIST_DIR)/bin/$(GOOS)_$(GOARCH)/sboxsub" ./cmd/sboxsub

build-linux:
	@$(MAKE) build GOOS=linux GOARCH=amd64
	@$(MAKE) build GOOS=linux GOARCH=arm64

snapshot: package

package: build
	@set -euo pipefail; \
	pkg="sbox-manager_$(VERSION)_$(GOOS)_$(GOARCH)"; \
	root="$(DIST_DIR)/package/$$pkg"; \
	rm -rf "$$root"; \
	mkdir -p "$$root/bin" "$(DIST_DIR)/release"; \
	cp "$(DIST_DIR)/bin/$(GOOS)_$(GOARCH)/sboxctl" "$$root/bin/sboxctl"; \
	cp "$(DIST_DIR)/bin/$(GOOS)_$(GOARCH)/sboxsub" "$$root/bin/sboxsub"; \
	cp README.md "$$root/README.md"; \
	if [ -f LICENSE ]; then cp LICENSE "$$root/LICENSE"; fi; \
	if [ -d templates ]; then cp -R templates "$$root/templates"; fi; \
	tar -C "$(DIST_DIR)/package" -czf "$(DIST_DIR)/release/$$pkg.tar.gz" "$$pkg"; \
	echo "created $(DIST_DIR)/release/$$pkg.tar.gz"

checksums:
	@set -euo pipefail; \
	test -d "$(DIST_DIR)/release" || { echo 'dist/release not found' >&2; exit 1; }; \
	cd "$(DIST_DIR)/release"; \
	if command -v sha256sum >/dev/null 2>&1; then sha256sum *.tar.gz > checksums.txt; else shasum -a 256 *.tar.gz > checksums.txt; fi

install-local:
	@scripts/install-local.sh --from "$(DIST_DIR)/bin/$(GOOS)_$(GOARCH)" --install-dir "$(BINDIR)" --force

clean: ; @rm -rf "$(DIST_DIR)"
