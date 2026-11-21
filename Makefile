.PHONY: help tools config-check consumer-check lint lint-sync lint-check format format-check test coverage security verify-release check clean

GOLANGCI_LINT_VERSION ?= v2.15.0
GOVULNCHECK_VERSION ?= v1.7.0
BIN_DIR := $(CURDIR)/bin
CACHE_DIR := $(CURDIR)/.cache
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
GOVULNCHECK := $(BIN_DIR)/govulncheck
LINT_CONFIG := .golangci.yml

export GOCACHE := $(CACHE_DIR)/go-build
export GOLANGCI_LINT_CACHE := $(CACHE_DIR)/golangci-lint

# The canonical `go test` flags come from this package's own exported config.
GOTEST_ARGS = $(shell go run ./cmd/gotest-config-path --args)

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

$(GOLANGCI_LINT):
	GOBIN=$(BIN_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOVULNCHECK):
	GOBIN=$(BIN_DIR) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

tools: $(GOLANGCI_LINT) $(GOVULNCHECK) ## Install pinned validation tools into ./bin

$(LINT_CONFIG): go.mod go.sum
	go tool golangci-config-path --out $(LINT_CONFIG)

lint-sync: ## Regenerate .golangci.yml from golangci-config-shared
	go tool golangci-config-path --out $(LINT_CONFIG)

config-check: $(GOLANGCI_LINT) $(LINT_CONFIG) ## Verify the generated lint config against the golangci-lint schema
	$(GOLANGCI_LINT) config verify --config $(LINT_CONFIG)

consumer-check: ## Verify test collection and all reporters from a clean consumer module
	./scripts/check-consumer.sh

lint: lint-check ## Alias for lint-check

lint-check: $(GOLANGCI_LINT) $(LINT_CONFIG) ## Run linters
	$(GOLANGCI_LINT) run --config $(LINT_CONFIG) ./...

format: $(GOLANGCI_LINT) $(LINT_CONFIG) ## Apply formatters
	$(GOLANGCI_LINT) fmt --config $(LINT_CONFIG) ./...

format-check: $(GOLANGCI_LINT) $(LINT_CONFIG) ## Verify formatting without writing
	$(GOLANGCI_LINT) fmt --diff --config $(LINT_CONFIG) ./...

test: ## Run tests
	go test -race -shuffle=on ./...

# This package is its own consumer: it collects coverage with the flags it
# exports, then reports it with the reporter it ships.
coverage: ## Run tests with coverage into coverage/
	@mkdir -p coverage
	go test $(GOTEST_ARGS)
	go run ./cmd/gotest-cover

security: $(GOVULNCHECK) ## Fail on reachable known vulnerabilities
	$(GOVULNCHECK) ./...

verify-release: ## Verify VERSION is a Go-compatible semantic release tag
	@printf '%s\n' "$(VERSION)" | grep -Eq '^v(0|1)\.[0-9]+\.[0-9]+$$' || \
		{ echo "VERSION must be a v0.x.y or v1.x.y tag" >&2; exit 1; }

check: config-check lint-check format-check coverage consumer-check security ## Single local and CI gate

clean: ## Remove generated artifacts
	rm -rf bin coverage $(CACHE_DIR) $(LINT_CONFIG)
