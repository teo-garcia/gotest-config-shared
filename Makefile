.PHONY: help tools lint lint-sync lint-check format format-check test coverage check clean

GOLANGCI_LINT_VERSION ?= v2.11.0
BIN_DIR := $(CURDIR)/bin
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
LINT_CONFIG := .golangci.yml

# The canonical `go test` flags come from this package's own exported config.
GOTEST_ARGS = $(shell go run ./cmd/gotest-config-path --args)

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

$(GOLANGCI_LINT):
	GOBIN=$(BIN_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

tools: $(GOLANGCI_LINT) ## Install the pinned golangci-lint into ./bin

lint-sync: ## Regenerate .golangci.yml from golangci-config-shared
	go run github.com/teo-garcia/golangci-config-shared/cmd/golangci-config-path --out $(LINT_CONFIG)

lint: lint-check ## Alias for lint-check

lint-check: $(GOLANGCI_LINT) ## Run linters
	$(GOLANGCI_LINT) run --config $(LINT_CONFIG) ./...

format: $(GOLANGCI_LINT) ## Apply formatters
	$(GOLANGCI_LINT) fmt --config $(LINT_CONFIG) ./...

format-check: $(GOLANGCI_LINT) ## Verify formatting without writing
	$(GOLANGCI_LINT) fmt --diff --config $(LINT_CONFIG) ./...

test: ## Run tests
	go test -race -shuffle=on ./...

# This package is its own consumer: it collects coverage with the flags it
# exports, then reports it with the reporter it ships.
coverage: ## Run tests with coverage into coverage/
	@mkdir -p coverage
	go test $(GOTEST_ARGS)
	go run ./cmd/gotest-cover

check: lint-check format-check test ## Single CI gate: lint + format + test

clean: ## Remove generated artifacts
	rm -rf bin coverage
