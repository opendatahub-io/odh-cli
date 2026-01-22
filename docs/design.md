# Design: odh-cli

This document describes the architecture and design decisions for the odh-cli kubectl plugin.

For development guidelines, coding conventions, and contribution practices, see [development.md](development.md).

## Overview

CLI tool for ODH (Open Data Hub) and RHOAI (Red Hat OpenShift AI) for interacting with ODH/RHOAI deployments on Kubernetes. The CLI is designed as a kubectl plugin to provide a familiar kubectl-like experience.

## Installation and Usage

### Docker Container

The CLI is available as a container image with multi-platform support (linux/amd64, linux/arm64).

**Default Configuration:**
The container sets `KUBECONFIG=/kubeconfig` by default. Mount your local kubeconfig to this path:

```bash
docker run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  quay.io/lburgazzoli/odh-cli:latest lint --target-version 3.3.0
```

**Custom Path:**
Override the KUBECONFIG environment variable if needed:

```bash
docker run --rm -ti \
  -v $KUBECONFIG:/custom/path \
  -e KUBECONFIG=/custom/path \
  quay.io/lburgazzoli/odh-cli:latest lint --target-version 3.3.0
```

**Interactive Debugging:**
The container includes kubectl, oc, and common utilities (wget, curl, tar, gzip, bash) for interactive debugging and troubleshooting:

```bash
# Shell into container for interactive debugging
docker run -it --rm \
  -v $KUBECONFIG:/kubeconfig \
  --entrypoint /bin/bash \
  quay.io/lburgazzoli/odh-cli:latest

# Inside container, use kubectl/oc for troubleshooting
kubectl get pods -n opendatahub
oc get dsci
kubectl-odh lint --target-version 3.3.0
```

**Available Tools:**
- `kubectl` (latest stable) - Standard Kubernetes CLI
- `oc` (latest stable) - OpenShift CLI
- `wget`, `curl` - Download utilities
- `tar`, `gzip` - Archive utilities
- `bash` - Interactive shell

### kubectl Plugin

Install the `kubectl-odh` binary to your PATH for kubectl integration:

```bash
kubectl odh lint --target-version 3.3.0
kubectl odh version
```

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

### Client Throttling Configuration

The CLI configures Kubernetes client throttling appropriate for parallel operations:

**Default Settings:**
- **QPS (Queries Per Second):** 50
- **Burst:** 100

**Rationale:**
- CLI tools with parallel workers benefit from higher throughput than kubectl defaults (QPS=5, Burst=10)
- Settings allow up to 20 concurrent workers to operate efficiently without client-side throttling delays
- Burst capacity handles initial spikes when all workers start simultaneously
- Still respectful of cluster API server resources

**User Configuration:**

Users can override throttling settings if needed:

```bash
# For very large backups or high-capacity clusters
kubectl odh backup --qps 100 --burst 200 --output-dir /tmp/backup

# For conservative usage on shared clusters
kubectl odh lint --qps 20 --burst 40 --target-version 3.0
```

**When to Adjust:**
- **Increase QPS/Burst:** Very large backups (100+ workloads), high-capacity cluster, dedicated cluster
- **Decrease QPS/Burst:** Shared cluster with strict API server limits, low-priority operations, resource-constrained environments

**Industry Context:**

The default settings align with other parallel Kubernetes tools:
- `kubectl` (sequential): QPS=5, Burst=10
- `velero` (parallel backup): QPS=100, Burst=200
- `helm` (parallel deployments): QPS=50+
- **odh-cli** (parallel operations): QPS=50, Burst=100 (conservative default)

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
├── backup [--output-dir <path>] [--dependencies <bool>] [--includes <types>] [--exclude <types>]
├── lint [-o|--output <format>] [--target-version <version>] [--checks <selector>]
└── version
```

**Common Elements:**
- **odh** (root command): The entry point for the plugin
- **backup**: Backs up OpenShift AI workloads and optionally their dependencies
- **lint**: Validates cluster configuration (current state) or upgrade readiness (with --target-version)
- **-o, --output** (flag): Specifies the output format. Supported values: `table` (default), `json`, `yaml`
- **--target-version** (flag): Target version for upgrade assessment
- **--checks** (flag): Filter checks by category, group, or name
- **--dependencies** (flag): Enable/disable dependency resolution for backup (default: `true`)
- **version**: Displays the CLI version information

**Extensibility:**
New commands can be added by implementing the command pattern with Cobra. Each command can define its own subcommands, flags, and execution logic while leveraging shared components like the output formatters and Kubernetes client.

**Note:** The lint command operates cluster-wide and does not support namespace filtering via `--namespace` flag.

### Backup Command

The `backup` command backs up OpenShift AI workloads and optionally their dependencies.

**Dependency Resolution:**

By default, the backup command resolves and backs up all workload dependencies:

```bash
# Full backup with all dependencies (ConfigMaps, PVCs, Secrets)
kubectl odh backup --output-dir /tmp/backup

# Workloads only (skip all dependencies)
kubectl odh backup --dependencies=false --output-dir /tmp/backup
```

**What gets backed up (with --dependencies=true):**
- ConfigMaps (excluding trusted-ca-bundle cluster CA bundles)
- PersistentVolumeClaims
- Secrets

**Security Note:** When `--dependencies=true`, Secrets are backed up along with other dependencies. Ensure your backup location is secure:
- Use encrypted storage
- Restrict access to backup files
- Use secure transmission channels
- Consider rotating secrets after backup/restore operations

To exclude all dependencies (including Secrets), use `--dependencies=false`.

**Use Cases:**

**With dependencies enabled (default):**
- Full disaster recovery backups
- Migrating workloads between clusters
- Need complete workload state including configurations

**With dependencies disabled:**
- Fast periodic snapshots of workload inventory
- Dependencies managed separately or stored in external systems
- Large clusters where dependency resolution is slow
- Workload definitions only needed

**Performance Considerations:**
- Disabling dependencies reduces API calls by ~80%
- Faster execution for large clusters (50-70% improvement)
- Lower memory usage without dependency resolution
- Use `--dependencies=false` when dependencies are managed separately

**Example:**
```bash
# Full backup with all dependencies (including Secrets)
kubectl odh backup --output-dir /tmp/full-backup --verbose

# Fast workload-only backup
kubectl odh backup --dependencies=false --output-dir /tmp/workloads-only --verbose
```

### Command Implementation Pattern

Commands follow a consistent pattern separating command definition from business logic.

#### Command Lifecycle

Each command implements a four-phase lifecycle:

1. **AddFlags**: Register command-specific flags
2. **Complete**: Initialize runtime state (client, namespace, parsing)
3. **Validate**: Verify all required options are set correctly
4. **Run**: Execute command business logic

Commands use a `Command` struct (not `Options`) with constructor `NewCommand()` (not `NewOptions()`).

**Typical Structure:**
```go
type Command struct {
    shared        *SharedOptions
    targetVersion string
}

func NewCommand(opts CommandOptions) *Command {
    return &Command{
        shared:        opts.Shared,
        targetVersion: opts.TargetVersion,
    }
}

func (c *Command) AddFlags(fs *pflag.FlagSet) { /* register flags */ }
func (c *Command) Complete() error { /* initialize client, parse inputs */ }
func (c *Command) Validate() error { /* validate configuration */ }
func (c *Command) Run(ctx context.Context) error { /* execute business logic */ }
```

See [architecture.md](architecture.md#command-lifecycle) for detailed lifecycle documentation.

## Output Formats

The CLI supports multiple output formats to accommodate different use cases. Commands should implement support for these formats using the shared printer components.

### Table Output (Default)

The table output is designed for human consumption and provides a quick, readable summary. The format adapts to each command's data structure. Icons and colors can be used for clarity where appropriate.

### JSON Output (`-o json`)

The JSON output is designed for scripting and integration with other tools. The structure varies by command but maintains consistency in formatting. Each command defines its own JSON structure based on its specific needs.

### YAML Output (`-o yaml`)

Similar to JSON output, the YAML format provides machine-readable output in YAML syntax, suitable for configuration files and human review.

## Lint Command

The `lint` command validates OpenShift AI cluster configuration and assesses upgrade readiness.

**DiagnosticResult Structure and Check Framework:**
The lint command uses a check framework with DiagnosticResult CR-like structures. For details, see:
- [lint/architecture.md](lint/architecture.md) - Lint command architecture
- [lint/writing-checks.md](lint/writing-checks.md) - Writing lint checks

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
