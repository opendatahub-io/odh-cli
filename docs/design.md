# Design: odh-cli

This document describes the architecture and design decisions for the odh-cli kubectl plugin.

For development guidelines, coding conventions, and contribution practices, see [development.md](development.md).

## Overview

CLI tool for ODH (Open Data Hub) and RHOAI (Red Hat OpenShift AI) for interacting with ODH/RHOAI deployments on Kubernetes. The CLI is designed as a kubectl plugin to provide a familiar kubectl-like experience.

## Key Architecture Decisions

### Core Principles
- **Extensible Command Structure**: Modular design allowing easy addition of new commands
- **Consistent Output**: Unified output formats (table, JSON) across all commands
- **kubectl Integration**: Native kubectl plugin providing familiar UX patterns

### Client Strategy
- Uses `controller-runtime/pkg/client` instead of `kubernetes.Interface`
- Better support for ODH and RHOAI custom resources
- Unified interface for standard and custom Kubernetes objects
- Simplifies interaction with Custom Resource Definitions (CRDs)

## Architecture & Design

The `odh` CLI is a standalone Go application that leverages the `client-go` library to communicate with the Kubernetes API server. It is designed to function as a kubectl plugin.

### kubectl Plugin Mechanism

The CLI is named `kubectl-odh`. When the binary is placed in a directory listed in the user's `PATH`, kubectl will automatically discover it, allowing it to be invoked as `kubectl odh`. The CLI relies on the user's active kubeconfig file for cluster authentication, just like kubectl.

### Core Libraries

- **Cobra**: To build a robust command-line interface with commands, subcommands, and flags
- **Viper**: For potential future configuration needs
- **Kubernetes client-go**: The official Go client library for interacting with the Kubernetes API
- **controller-runtime/client**: A higher-level client to simplify interactions with Custom Resources
- **k8s.io/cli-runtime**: Provides standard helpers for building kubectl-like command-line tools, handling common flags and client configuration

### Command Structure

The CLI is structured using Cobra with an extensible subcommand architecture:

```
kubectl odh
├── <command> [-o|--output <format>] [--namespace <ns>] [command-specific flags]
└── version
```

**Common Elements:**
- **odh** (root command): The entry point for the plugin
- **-o, --output** (flag): Specifies the output format. Supported values: `table` (default), `json`, `yaml`
- **--namespace** (flag): Managed via cli-runtime. Specifies the namespace for operations when applicable
- **version**: Displays the CLI version information

**Extensibility:**
New commands can be added by implementing the command pattern with Cobra. Each command can define its own subcommands, flags, and execution logic while leveraging shared components like the output formatters and Kubernetes client.

### Command Implementation Pattern

Commands follow a consistent pattern inspired by `sample-cli-plugin`, separating command definition, options, and execution logic.

#### Standard Command Structure

Each command typically follows this structure:

1. **Initialize**: The root command instantiates a `genericclioptions.ConfigFlags` object from cli-runtime to manage common kubectl flags
2. **Options Struct**: A command-specific options struct (e.g., `CommandOptions`) holds configuration, I/O streams, `ConfigFlags`, and output format
3. **Execution Logic**: A `Run()` method on the options struct implements the command logic
4. **Output Formatting**: Commands use shared printer components to format output consistently

**Typical Run Method Flow:**
1. Gets the target namespace (if applicable) and creates a Kubernetes client from the `ConfigFlags`
2. Executes the command-specific logic
3. Collects and processes results
4. Uses a printer to format results based on the specified output format

## Output Formats

The CLI supports multiple output formats to accommodate different use cases. Commands should implement support for these formats using the shared printer components.

### Table Output (Default)

The table output is designed for human consumption and provides a quick, readable summary. The format adapts to each command's data structure. Icons and colors can be used for clarity where appropriate.

### JSON Output (`-o json`)

The JSON output is designed for scripting and integration with other tools. The structure varies by command but maintains consistency in formatting. Each command defines its own JSON structure based on its specific needs.

### YAML Output (`-o yaml`)

Similar to JSON output, the YAML format provides machine-readable output in YAML syntax, suitable for configuration files and human review.

## Project Structure

A standard Go CLI project structure is used, drawing inspiration from `sample-cli-plugin`.

```
/odh-cli
├── cmd/
│   ├── version/        # Version command
│   └── main.go         # Entry point
├── pkg/
│   ├── printer/        # Shared output formatting
│   └── util/           # Shared utilities (client, discovery, etc.)
├── internal/
│   └── version/        # Internal version information
├── go.mod
├── go.sum
└── Makefile
```

**Key Directories:**
- `cmd/`: Command definitions and entry points
- `pkg/`: Public packages that implement command logic and shared utilities
- `internal/`: Internal packages not intended for external use

New commands can be added under `cmd/` with their implementation logic in `pkg/` following the established patterns.

## Key Implementation Notes

1. **Use cli-runtime**: Leverage `k8s.io/cli-runtime/pkg/genericclioptions` for standard kubectl flag handling
2. **Follow kubectl patterns**: Study existing kubectl plugins for consistent UX patterns
3. **Error handling**: Ensure graceful failure and meaningful error messages when ODH/RHOAI components are not available
4. **Extensibility**: Design commands to be modular and easy to add or modify
5. **Testing**: Include both unit tests and integration tests with fake Kubernetes clients
6. **Shared components**: Maximize code reuse through shared utilities like output formatters and client factories
