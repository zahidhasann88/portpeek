BINARY  := portpeek
VERSION ?= $(or $(shell git describe --tags --dirty 2>/dev/null | sed 's/^v//'),0.1.0-dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOFLAGS ?=

.PHONY: all build test test-race lint fmt vet install clean check

all: check build

build: ## Build the binary for the host platform
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/portpeek

test: ## Run unit tests
	go test ./...

test-race: ## Run unit tests with the race detector
	go test -race ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go sources
	gofmt -s -w cmd internal

lint: ## Run golangci-lint and check formatting
	@test -z "$$(gofmt -l cmd internal)" || (echo "gofmt needed:" && gofmt -l cmd internal && exit 1)
	golangci-lint run ./...

install: ## Install into $$GOBIN (or $$GOPATH/bin)
	go install -ldflags "$(LDFLAGS)" ./cmd/portpeek

check: vet test lint ## Run vet, tests and lint

clean: ## Remove build artifacts
	rm -rf $(BINARY) $(BINARY).exe dist coverage.out
