# SPDX-License-Identifier: Apache-2.0

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/tessaly-dev/tessaly/internal/version.Version=$(VERSION)
BIN     := bin/tess

.PHONY: setup ui build dev test test-go test-ui lint check clean

setup: ## Install UI dependencies from the lockfile
	cd web && pnpm install --frozen-lockfile

ui: ## Build the React UI into web/dist
	cd web && pnpm run build

build: ui ## Build bin/tess with the UI embedded
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/tess

dev: build ## Build and run the server on 127.0.0.1:7777
	$(BIN) serve

test: test-go test-ui ## Run Go and UI unit tests

test-go:
	go test -race ./...

test-ui:
	cd web && pnpm test

lint: ## Run Go and UI linters and type checks
	golangci-lint run ./...
	go vet ./...
	cd web && pnpm run typecheck

check: lint test build ## Everything CI runs

clean:
	rm -rf bin
	find web/dist -mindepth 1 ! -name .gitkeep -delete
