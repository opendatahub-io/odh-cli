# Setup and Build

This document covers setup, build commands, and test commands for odh-cli development.

For development guidelines and coding conventions, see [development.md](development.md).

## Build Commands

**CRITICAL: ALWAYS use `make` commands. NEVER invoke tools directly.**

```bash
# Build the binary
make build

# Run the doctor command
make run

# Format code (NEVER use gci directly)
make fmt

# Run linter (NEVER use golangci-lint directly)
make lint

# Run linter with auto-fix (ALWAYS try this FIRST before manual fixes)
make lint/fix

# Run vulnerability scanner
make vulncheck

# Run all checks (lint + vulncheck)
make check

# Run tests
make test

# Tidy dependencies
make tidy

# Clean build artifacts
make clean
```

**Why use make commands instead of tools directly:**
- **Consistency**: Ensures everyone uses the same linter configuration and settings
- **Safety**: Prevents accidental changes to critical files (e.g., blank imports)
- **Correctness**: Makefile handles proper tool invocation with correct flags
- **Maintainability**: Tool versions and configuration centralized in one place

**Prohibited commands:**
- ❌ `golangci-lint run` - Use `make lint` instead
- ❌ `gci write` - Use `make fmt` instead
- ❌ `gofmt` - Use `make fmt` instead
- ❌ `goimports` - Use `make fmt` instead

## Building Container Image

```bash
make publish
```

This builds a multi-platform container image (linux/amd64, linux/arm64).

## Test Commands

```bash
# Run all tests with verbose output
go test -v ./...

# Run tests in a specific package
go test -v ./pkg/printer

# Run a specific test
go test -v ./pkg/printer -run TestTablePrinter

# Run tests for all packages
make test
```
