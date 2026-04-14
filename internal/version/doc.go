// Package version provides version information for the odh-cli binary.
//
// # Version Information Flow
//
// Version information (Version, Commit, Date) is embedded into the binary at
// compile time using Go's -ldflags mechanism, which overwrites the default
// values in version.go.
//
// # Build Methods
//
// ## 1. Direct Go Build (Development)
//
//	go build cmd/main.go
//	./main version
//
// Version Source: Go source code defaults (version.go)
//
//	var (
//	    Version = "dev"
//	    Commit = "unknown"
//	    Date = "unknown"
//	)
//
// Output example: kubectl-odh version dev (commit: unknown, built: unknown)
//
// Use Case: Quick local development builds without git metadata.
//
// ## 2. Direct Container Build (Not Recommended)
//
//	podman build -t test .
//	podman run --rm test version
//
// Version Source: Dockerfile ARG defaults
//
// Flow:
//
//  1. Dockerfile ARGs use defaults (no --build-arg provided):
//     ARG VERSION=dev        # Falls back to "dev"
//     ARG COMMIT=unknown     # Falls back to "unknown"
//     ARG DATE=unknown       # Falls back to "unknown"
//
//  2. Dockerfile passes defaults to inner build:
//     RUN make build VERSION=${VERSION}  # VERSION=dev
//
//  3. Inner make build embeds via ldflags with default values
//
// Output example: kubectl-odh version X.Y.Z (commit: unknown, built: unknown)
//
// Use Case: Testing Dockerfile syntax. Not recommended for production - always
// use make build-image instead.
//
// ## 3. Makefile Build (Recommended for Local Development)
//
//	make build
//	./bin/kubectl-odh version
//
// Version Source: Git repository (computed by Makefile)
//
// Flow:
//
//  1. Makefile computes from git:
//     VERSION ?= $(shell git describe --tags --always --dirty)
//     COMMIT ?= $(shell git rev-parse --short HEAD)
//     DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
//
//  2. Makefile embeds via ldflags:
//     go build -ldflags "-X 'internal/version.Version=$(VERSION)' ..."
//
//  3. ldflags overwrites Go source defaults at compile time
//
// Output example: kubectl-odh version X.Y.Z (commit: 7d65e7f, built: 2026-03-17T07:49:56Z)
//
// Use Case: Local development with proper version tracking.
//
// ## 4. Container Build via Makefile (Production) from Github Action
//
//	make build-image
//	podman run --rm localhost/odh-cli:VERSION version
//
// Version Source: Git repository (passed as Docker build args)
//
// Flow:
//
//  1. Makefile computes from git (same as method 3)
//
//  2. Makefile passes to Docker build:
//     podman build \
//     --build-arg VERSION=$(VERSION) \
//     --build-arg COMMIT=$(COMMIT) \
//     --build-arg DATE=$(DATE)
//
//  3. Dockerfile receives and passes to inner build:
//     ARG VERSION=dev
//     RUN make build VERSION=${VERSION}
//
//  4. Inner make build embeds via ldflags (same as method 3)
//
// Output example: kubectl-odh version X.Y.Z (commit: 7d65e7f, built: 2026-03-17T07:49:56Z)
//
// Use Case: Production container images with proper version tracking.
//
// # Summary Table
//
//	| Build Method        | Version       | Commit    | Date      | Use Case                     |
//	|---------------------|---------------|-----------|-----------|------------------------------|
//	| go build            | dev           | unknown   | unknown   | Quick local dev              |
//	| podman build .      | dev           | unknown   | unknown   | Testing only                 |
//	| make build          | Git tag/SHA   | Git SHA   | Timestamp | Local dev with metadata      |
//	| make build-image    | Git tag/SHA   | Git SHA   | Timestamp | Production (recommended)     |
//
// # Defaults Purpose
//
// Go source defaults (version.go):
//   - Used only when building without ldflags (direct go build)
//   - Fallback for quick development builds
//
// Dockerfile ARG defaults:
//   - Used only when building without --build-arg (direct podman build)
//   - Allows Dockerfile to build without errors
//   - Not intended for production use
//
// Both sets of defaults serve the same purpose: allow builds to succeed in
// development scenarios where git metadata isn't needed.
//
// # Recommended Practice
//
// Always use Makefile commands:
//   - Local binary: > make build
//   - Container image: > make build-image
//
// These ensure proper version information is embedded from git metadata.
package version
