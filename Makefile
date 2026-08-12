BINARY := kubedrill
PKG := github.com/agarwalvivek29/kubedrill
VERSION ?= 0.0.0-dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(PKG)/internal/buildinfo.Version=$(VERSION) \
           -X $(PKG)/internal/buildinfo.Commit=$(COMMIT) \
           -X $(PKG)/internal/buildinfo.Date=$(DATE)

.PHONY: build test lint tidy clean

build: ## Build the kubedrill binary
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/kubedrill

test: ## Run unit tests with the race detector
	go test -race ./...

lint: ## Static checks (vet now; golangci-lint wired in CI)
	go vet ./...

tidy: ## Sync go.mod/go.sum
	go mod tidy

clean:
	rm -f $(BINARY)
	rm -rf dist/
