# kubectl odh CLI Extension - Implementation Guide

## Overview

CLI tool for ODH (Open Data Hub) and RHOAI (Red Hat OpenShift AI) for interacting with ODH/RHOAI deployments on Kubernetes. The `odh` CLI is implemented as a kubectl plugin.

## Documentation

This project's documentation is organized into two main documents:

### [Design Documentation](docs/design.md)

Covers the architecture and design decisions for the CLI:
- Core architecture and design principles
- kubectl plugin mechanism
- Command structure and extensibility
- Output formats (table, JSON)
- Project structure

### [Development Documentation](docs/development.md)

Covers development guidelines and best practices:
- Setup and build commands
- Coding conventions (functional options, error handling, function signatures)
- Testing guidelines (Gomega, test data organization)
- Extensibility (adding commands, output formats)
- Code review guidelines (git commit conventions, PR checklist)

## Quick Start

```bash
# Build the binary
make build

# Run the CLI
make run

# Run tests
make test

# Run all checks (lint + vulncheck)
make check
```

## Key Features

- **kubectl Integration**: Works as a native kubectl plugin
- **Multiple Output Formats**: Human-readable table output and machine-parsable JSON
- **Extensible Command Structure**: Easy to add new commands and capabilities
- **ODH/RHOAI-Aware**: Uses controller-runtime client for better ODH and RHOAI custom resource support
- **Modular Design**: Clean separation of concerns with reusable components
