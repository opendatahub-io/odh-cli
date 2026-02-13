# kubectl-odh CLI Makefile

# Binary name
BINARY_NAME=bin/kubectl-odh

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Build flags
LDFLAGS = -X 'github.com/opendatahub-io/odh-cli/internal/version.Version=$(VERSION)' \
          -X 'github.com/opendatahub-io/odh-cli/internal/version.Commit=$(COMMIT)' \
          -X 'github.com/opendatahub-io/odh-cli/internal/version.Date=$(DATE)'

# Linter configuration
LINT_TIMEOUT := 10m

# Container registry configuration
CONTAINER_REGISTRY ?= quay.io
CONTAINER_REPO ?= $(CONTAINER_REGISTRY)/rhoai/rhoai-upgrade-helpers-rhel9
CONTAINER_PLATFORMS ?= linux/amd64,linux/arm64
CONTAINER_TAGS ?= $(VERSION)

# Platform for cross-compilation (defaults to current platform)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

## Tools
GOLANGCI_VERSION ?= v2.8.0
GOLANGCI ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
GOVULNCHECK_VERSION ?= latest
GOVULNCHECK ?= go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Build the binary
.PHONY: build
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) cmd/main.go

# Run the doctor command
.PHONY: run
run:
	go run -ldflags "$(LDFLAGS)" cmd/main.go doctor

# Tidy up dependencies
.PHONY: tidy
tidy:
	go mod tidy

# Clean build artifacts
.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	go clean -x
	go clean -x -testcache

# Format code
.PHONY: fmt
fmt:
	@$(GOLANGCI) fmt --config .golangci.yml
	go fmt ./...

# Run linter
.PHONY: lint
lint:
	@$(GOLANGCI) run --config .golangci.yml --timeout $(LINT_TIMEOUT)

# Run linter with auto-fix
.PHONY: lint/fix
lint/fix:
	@$(GOLANGCI) run --config .golangci.yml --timeout $(LINT_TIMEOUT) --fix

# Run vulnerability check
.PHONY: vulncheck
vulncheck:
	@$(GOVULNCHECK) ./...

# Run all checks
.PHONY: check
check: lint vulncheck

# Run tests
.PHONY: test
test:
	go test ./...

# Build container image without pushing (creates local manifest)
.PHONY: build-image
build-image:
	@echo "Building container image for platforms: $(CONTAINER_PLATFORMS)"
	@MANIFEST_NAME="localhost/odh-cli:$(VERSION)"; \
	podman build \
		--platform=$(CONTAINER_PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		--manifest=$$MANIFEST_NAME \
		.
	@echo "Container image built successfully: localhost/odh-cli:$(VERSION)"
	@echo "To inspect the manifest: podman manifest inspect localhost/odh-cli:$(VERSION)"
	@echo "To run: podman run --rm localhost/odh-cli:$(VERSION) version"

# Build and push container image using Podman manifest
.PHONY: publish
publish: build-image
	@echo "Pushing container image to $(CONTAINER_REPO):$(CONTAINER_TAGS)"
	@MANIFEST_NAME="localhost/odh-cli:$(VERSION)"; \
	TAGS="$(CONTAINER_TAGS)"; \
	for tag in $${TAGS//,/ }; do \
		podman manifest push $$MANIFEST_NAME docker://$(CONTAINER_REPO):$$tag; \
	done; \
	podman manifest rm $$MANIFEST_NAME 2>/dev/null || true
	@echo "Container image published successfully"

# Help target
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build       - Build the kubectl-odh binary"
	@echo "  build-image - Build container image without pushing (creates local manifest)"
	@echo "  publish     - Build and push container image using Podman manifest"
	@echo "  run         - Run the doctor command"
	@echo "  tidy        - Tidy up Go module dependencies"
	@echo "  clean       - Remove build artifacts and test cache"
	@echo "  fmt         - Format Go code"
	@echo "  lint        - Run golangci-lint"
	@echo "  lint/fix    - Run golangci-lint with auto-fix"
	@echo "  vulncheck   - Run vulnerability scanner"
	@echo "  check       - Run all checks (lint + vulncheck)"
	@echo "  test        - Run tests"
	@echo "  help        - Show this help message"