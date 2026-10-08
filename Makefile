# kubectl-odh CLI Makefile

# Binary name
BINARY_NAME=bin/kubectl-odh

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
CLI_NAME ?= odh-cli

# Pinned commit SHA from odh-gitops for reproducible builds
ODH_GITOPS_COMMIT ?= 1a55af06b8fe85c8ed63b1eff680477d9bf86be3

# Build flags
LDFLAGS = -X 'github.com/opendatahub-io/odh-cli/internal/version.Version=$(VERSION)' \
          -X 'github.com/opendatahub-io/odh-cli/internal/version.Commit=$(COMMIT)' \
          -X 'github.com/opendatahub-io/odh-cli/internal/version.Date=$(DATE)' \
          -X 'github.com/opendatahub-io/odh-cli/internal/version.CLIName=$(CLI_NAME)' \
          -X 'github.com/opendatahub-io/odh-cli/pkg/deps.gitopsRef=$(ODH_GITOPS_COMMIT)'

# Linter configuration
LINT_TIMEOUT := 10m

# Container registry configuration
CONTAINER_REGISTRY ?= quay.io
CONTAINER_REPO ?= $(CONTAINER_REGISTRY)/rhoai/odh-cli-rhel9
CONTAINER_PLATFORMS ?= linux/amd64,linux/arm64,linux/ppc64le
CONTAINER_TAGS ?= $(VERSION)

# Platform for cross-compilation (defaults to current platform)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# FIPS-related build vars for build recipe
# To build for fips:
# make build CGO_ENABLED=1 GOEXPERIMENT=strictfipsruntime GO_BUILD_TAGS="-tags strictfipsruntime"
CGO_ENABLED ?= 0
GOEXPERIMENT ?=
GO_BUILD_TAGS ?=

## Tools
GOLANGCI_VERSION ?= v2.8.0
GOLANGCI ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
GOVULNCHECK_VERSION ?= latest
GOVULNCHECK ?= go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Fetch dependency manifest from odh-gitops
.PHONY: fetch-deps
fetch-deps:
	@mkdir -p pkg/deps/data
	@echo "Fetching dependency manifest from odh-gitops (commit: $(ODH_GITOPS_COMMIT))..."
	@curl -fsSL "https://raw.githubusercontent.com/opendatahub-io/odh-gitops/$(ODH_GITOPS_COMMIT)/charts/rhai-on-openshift-chart/values.yaml" \
		-o pkg/deps/data/values.yaml
	@curl -fsSL "https://raw.githubusercontent.com/opendatahub-io/odh-gitops/$(ODH_GITOPS_COMMIT)/charts/rhai-on-openshift-chart/Chart.yaml" \
		-o pkg/deps/data/Chart.yaml

# Generate JSON schemas from Go types (creates placeholders first for go:embed)
.PHONY: gen-schemas
gen-schemas:
	@mkdir -p pkg/schema/data && cd pkg/schema/data && touch diagnostic_result_list.json component_list.json component_details.json dependency_status_list.json dependency_info_list.json version_info.json version_info_verbose.json kubernetes_list.json
	@go run tools/gen-schemas/main.go

# Build the binary
.PHONY: build
build: gen-schemas
	CGO_ENABLED=$(CGO_ENABLED) GOEXPERIMENT=$(GOEXPERIMENT) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build $(GO_BUILD_TAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) cmd/main.go

# Install completion (defaults to bash)
.PHONY: install-completion
install-completion: install-completion-bash

# Install bash completion for the current user
.PHONY: install-completion-bash
install-completion-bash: build
	@echo "Installing bash completion..."
	@mkdir -p ~/.local/share/bash-completion/completions
	@"$(BINARY_NAME)" completion bash > ~/.local/share/bash-completion/completions/kubectl-odh
	@echo "Done! Start a new terminal for completions to take effect."

# Install zsh completion for the current user
.PHONY: install-completion-zsh
install-completion-zsh: build
	@echo "Installing zsh completion..."
	@mkdir -p ~/.zsh/completions
	@"$(BINARY_NAME)" completion zsh > ~/.zsh/completions/_kubectl-odh
	@if ! grep -q 'fpath=(~/.zsh/completions' ~/.zshrc 2>/dev/null; then \
		echo 'fpath=(~/.zsh/completions $$fpath)' >> ~/.zshrc; \
		echo "Added fpath to ~/.zshrc"; \
	fi
	@if ! grep -q 'autoload -Uz compinit' ~/.zshrc 2>/dev/null; then \
		echo 'autoload -Uz compinit && compinit' >> ~/.zshrc; \
		echo "Added compinit to ~/.zshrc"; \
	fi
	@echo "Done! Run 'source ~/.zshrc' or start a new terminal."

# Install fish completion for the current user
.PHONY: install-completion-fish
install-completion-fish: build
	@echo "Installing fish completion..."
	@mkdir -p ~/.config/fish/completions
	@"$(BINARY_NAME)" completion fish > ~/.config/fish/completions/kubectl-odh.fish
	@echo "Done! Start a new terminal or run 'source ~/.config/fish/completions/kubectl-odh.fish'"

# Run the doctor command
.PHONY: run
run: gen-schemas
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
lint: gen-schemas
	@$(GOLANGCI) run --config .golangci.yml --timeout $(LINT_TIMEOUT)

# Run linter with auto-fix
.PHONY: lint/fix
lint/fix: gen-schemas
	@$(GOLANGCI) run --config .golangci.yml --timeout $(LINT_TIMEOUT) --fix

# Run vulnerability check
.PHONY: vulncheck
vulncheck: gen-schemas
	@$(GOVULNCHECK) ./...

# Run all checks
.PHONY: check
check: lint

# Run tests
.PHONY: test
test: gen-schemas
	go test -coverprofile=coverage.out ./...

# Storage-migration integration tests. By default each test creates a Kind cluster
# (Docker, or KIND_EXPERIMENTAL_PROVIDER=podman). Set STORAGE_MIGRATION_CLUSTER=external
# to use the cluster of the current kubeconfig instead: this DELETES its DataScienceCluster CRD.
.PHONY: test/integration
test/integration: gen-schemas
	go test -tags integration -count=1 -v -timeout=30m ./tests/integration/storage-migration

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
		--build-arg CLI_NAME=$(CLI_NAME) \
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
	@echo "  build                   - Build the kubectl-odh binary"
	@echo "  install-completion      - Install shell completion (defaults to bash)"
	@echo "  install-completion-bash - Install bash completion for current user"
	@echo "  install-completion-zsh  - Install zsh completion for current user"
	@echo "  install-completion-fish - Install fish completion for current user"
	@echo "  build-image             - Build container image without pushing (creates local manifest)"
	@echo "  publish                 - Build and push container image using Podman manifest"
	@echo "  run                     - Run the doctor command"
	@echo "  tidy                    - Tidy up Go module dependencies"
	@echo "  clean                   - Remove build artifacts and test cache"
	@echo "  fmt                     - Format Go code"
	@echo "  lint                    - Run golangci-lint"
	@echo "  lint/fix                - Run golangci-lint with auto-fix"
	@echo "  vulncheck               - Run vulnerability scanner"
	@echo "  check                   - Run all checks (lint)"
	@echo "  test                    - Run tests"
	@echo "  fetch-deps              - Fetch dependency manifest from odh-gitops"
	@echo "  gen-schemas             - Generate JSON schemas from Go types"
	@echo "  help                    - Show this help message"
