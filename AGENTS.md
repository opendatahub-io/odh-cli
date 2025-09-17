# kubectl odh CLI Extension - Implementation Guide

## Overview

Diagnostic CLI tool for ODH installations with hierarchical check system.

## Key Architecture Decisions

### Core Structure
- **Categories**: Top-level diagnostic groups (e.g., "Component Health")
- **Checks**: Individual tests within categories (e.g., "Pod Readiness")
- **Status Priority**: ERROR > WARNING > OK

### Data Model
```go
type Category struct {
    Name    string
    Status  Status
    Message string
    Checks  []Check
}

type Check struct {
    Name    string
    Status  Status
    Message string
}
```

### Client Strategy
- Uses `controller-runtime/pkg/client` instead of `kubernetes.Interface`
- Better for ODH custom resources
- Unified interface for standard and custom Kubernetes objects

## 3. Architecture & Design

The `odh` CLI will be a standalone Go application that leverages the `client-go` library to communicate with the Kubernetes API server. It will be designed to function as a kubectl plugin.

### 3.1. kubectl Plugin Mechanism

The CLI will be named `kubectl-odh`. When the binary is placed in a directory listed in the user's `PATH`, kubectl will automatically discover it, allowing it to be invoked as `kubectl odh`. The CLI will rely on the user's active kubeconfig file for cluster authentication, just like kubectl.

### 3.2. Core Libraries

- **Cobra**: To build a robust command-line interface with commands, subcommands, and flags
- **Viper**: For potential future configuration needs
- **Kubernetes client-go**: The official Go client library for interacting with the Kubernetes API
- **controller-runtime/client**: A higher-level client to simplify interactions with Custom Resources
- **k8s.io/cli-runtime**: Provides standard helpers for building kubectl-like command-line tools, handling common flags and client configuration

### 3.3. Command Structure

The CLI will be structured using Cobra as follows:

```
kubectl odh
└── doctor [-o|--output <format>] [--namespace <ns>]
```

- **odh** (root command): The entry point for the plugin
- **doctor** (subcommand): Executes the diagnostic checks
- **-o, --output** (flag): Specifies the output format. Supported values: `table` (default), `json`
- **--namespace** (flag): Managed via cli-runtime. Specifies the namespace where the ODH operator is installed. Defaults to a common installation namespace like `opendatahub`

### 3.4. The doctor Command Logic

The core of the `doctor` command is a "runner" that executes a series of independent checks. The implementation will follow the pattern from `sample-cli-plugin` by separating command definition, options, and execution logic.

#### Initialize
- The root command will instantiate a `genericclioptions.ConfigFlags` object from cli-runtime to manage common kubectl flags
- The doctor command will be initialized with this configuration

#### Define Options Struct
- A `DoctorOptions` struct will hold the configuration and I/O streams for the command, including `ConfigFlags` and the output format

#### Define Checks
- A `Check` will be an interface or struct that encapsulates a single diagnostic test
- Each check will have a `Name` and an `Execute()` method
- The `Execute()` method takes the Kubernetes client as input and returns a `Result`
- Specific checks will be added in a later phase of development

#### Execute Logic (Run method)
The primary logic will reside in a `Run()` method on the `DoctorOptions` struct.

The `Run` method will:
1. Get the target namespace and create a Kubernetes client from the `ConfigFlags`
2. Instantiate and run the predefined list of checks sequentially
3. Collect the results from each check
4. Use a "printer" to format the collected results based on the specified output format

## 4. Output Formats

### 4.1. Table Output (Default)

The table output is for human consumption and will provide a quick, glanceable summary.

```
CHECK          STATUS     MESSAGE
Check Name 1   ✅ OK       Success message for check 1.
Check Name 2   ❌ ERROR    Error details for check 2.
Check Name 3   ⚠️ WARNING  Warning message for check 3.
```

Icons (✅, ❌, ⚠️) are recommended for clarity.

### 4.2. JSON Output (`-o json`)

The JSON output is for scripting and integration with other tools.

```json
{
  "checks": [
    {
      "name": "Check Name 1",
      "status": "OK",
      "message": "Success message for check 1."
    },
    {
      "name": "Check Name 2",
      "status": "ERROR",
      "message": "Error details for check 2."
    },
    {
      "name": "Check Name 3",
      "status": "WARNING",
      "message": "Warning message for check 3."
    }
  ],
  "summary": {
    "ok": 1,
    "warning": 1,
    "error": 1
  }
}
```

## 5. Project Structure

A standard Go CLI project structure is recommended, drawing inspiration from `sample-cli-plugin`.

```
/kubectl-odh
├── cmd/
│   ├── doctor/
│   │   ├── doctor.go   # Defines the 'doctor' subcommand (NewDoctorCmd)
│   │   └── options.go  # Defines DoctorOptions struct and Run logic
│   └── root.go         # Defines the root 'odh' command
├── pkg/
│   ├── doctor/
│   │   ├── runner.go   # Logic to run all checks
│   │   ├── types.go    # Defines the Check interface and Result struct
│   │   └── checks/     # Directory for individual check implementations
│   └── printers/
│       ├── table.go    # Table output formatting
│       └── json.go     # JSON output formatting
├── go.mod
├── go.sum
└── main.go
```

## 6. Development Guidelines

All code for this project must adhere to the following development guidelines.

### Core Principles

#### Focus and Precision
- Address only the specific task at hand
- Make minimal, targeted changes to fulfill requirements
- Avoid scope creep or unnecessary modifications

#### Code Quality
- Follow DRY (Don't Repeat Yourself) principles rigorously
- Extract common patterns into reusable functions
- Prioritize readability and maintainability over cleverness

#### Documentation Philosophy
- Comments should explain **why**, not **what**
- Focus on clarifying non-obvious behavior, edge cases, and relationships between components
- Avoid redundant comments that merely restate the code
- Skip boilerplate docstrings unless they add genuine value

### Language-Specific Guidelines: Go

#### Function Signatures
- Each parameter must have its own type declaration
- Never group parameters with the same type
- Use multiline formatting for functions with many parameters:

```go
func ProcessRequest(
    ctx context.Context,
    userID string,
    requestType int,
    payload []byte,
    timeout time.Duration,
) (*Response, error) {
    // implementation
}
```

#### Testing with Gomega
- Prefer vanilla Gomega assertions over Ginkgo BDD style
- Always use dot imports for Gomega:

```go
import . "github.com/onsi/gomega"
```

#### Error Handling
- Return errors as the last parameter
- Use `fmt.Errorf` with `%w` verb for error wrapping
- Handle errors at the appropriate level of abstraction

#### Naming Conventions
- Use camelCase for unexported functions and variables
- Use PascalCase for exported functions and types
- Prefer descriptive names over short abbreviations

### General Code Style

#### Variable Declarations
- Use short variable names in limited scopes
- Prefer explicit types when they aid readability
- Initialize variables close to their usage

#### Function Design
- Keep functions focused on a single responsibility
- Limit function length to ~20-30 lines when possible
- Use early returns to reduce nesting

### Code Review Checklist

Before submitting code, verify:

- [ ] All tests pass and provide meaningful coverage
- [ ] No duplicate code patterns exist
- [ ] Comments explain complex logic or business rules
- [ ] Function signatures follow language conventions
- [ ] Error handling is appropriate and consistent
- [ ] Code follows established project patterns

## Key Implementation Notes

1. **Use cli-runtime**: Leverage `k8s.io/cli-runtime/pkg/genericclioptions` for standard kubectl flag handling
2. **Follow kubectl patterns**: Study existing kubectl plugins for consistent UX patterns
3. **Error handling**: Ensure graceful failure when ODH components are not installed
4. **Extensibility**: Design the check framework to easily accommodate new diagnostic checks
5. **Testing**: Include both unit tests and integration tests with fake Kubernetes clients