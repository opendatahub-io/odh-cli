# Development Guide: odh-cli

This document provides coding conventions, testing guidelines, and contribution practices for developing the odh-cli kubectl plugin.

For architectural information and design decisions, see [design.md](design.md).

## Table of Contents

1. [Setup and Build](#setup-and-build)
2. [Coding Conventions](#coding-conventions)
3. [Testing Guidelines](#testing-guidelines)
4. [Extensibility](#extensibility)
5. [Code Review Guidelines](#code-review-guidelines)

## Setup and Build

### Build Commands

```bash
# Build the binary
make build

# Run the doctor command
make run

# Format code
make fmt

# Run linter
make lint

# Run linter with auto-fix
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

### Test Commands

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

## Coding Conventions

### Functional Options Pattern

All struct initialization uses the functional options pattern for flexible, extensible configuration. This project adopts the generic `Option[T]` interface pattern from [k8s-controller-lib](https://github.com/lburgazzoli/k8s-controller-lib/blob/main/pkg/util/option.go) for type-safe, extensible configuration.

**Define the Option Interface:**

The `pkg/util/option.go` package provides the generic infrastructure:

```go
// Option is a generic interface for applying configuration to a target.
type Option[T any] interface {
    ApplyTo(target *T)
}

// FunctionalOption wraps a function to implement the Option interface.
type FunctionalOption[T any] func(*T)

func (f FunctionalOption[T]) ApplyTo(target *T) {
    f(target)
}
```

**Define Type-Specific Options:**

```go
// Type alias for convenience
type Option = util.Option[Renderer]

// Function-based option using FunctionalOption
func WithWriter(w io.Writer) Option {
    return util.FunctionalOption[Renderer](func(r *Renderer) {
        r.writer = w
    })
}

func WithHeaders(headers ...string) Option {
    return util.FunctionalOption[Renderer](func(r *Renderer) {
        r.headers = headers
    })
}
```

**Apply Options:**

```go
func NewRenderer(opts ...Option) *Renderer {
    r := &Renderer{
        writer:     os.Stdout,
        formatters: make(map[string]ColumnFormatter),
    }

    // Apply options using the interface method
    for _, opt := range opts {
        opt.ApplyTo(r)
    }

    return r
}
```

**Guidelines:**
- Use the generic `Option[T]` interface for type safety
- Wrap option functions with `util.FunctionalOption[T]` to implement the interface
- Keep options simple and focused on a single configuration aspect
- Place all options and related methods in `*_options.go` files (or `*_option.go` for consistency)
- Use descriptive names that clearly indicate what is being configured
- This pattern allows for both function-based and struct-based options implementing the same interface

**Usage:**
```go
// Function-based (flexible, composable)
renderer := table.NewRenderer(
    table.WithWriter(os.Stdout),
    table.WithHeaders("CHECK", "STATUS", "MESSAGE"),
)
```

**Benefits:**
- Type-safe configuration using generics
- Extensible: can have both function-based and struct-based options
- Consistent with k8s-controller-lib patterns
- Clear separation between option definition and application

### Error Handling Conventions

* Errors are wrapped using `fmt.Errorf` with `%w` for proper error chain propagation
* Context is passed through operations for cancellation support
* First error encountered stops processing and is returned immediately
* All constructors validate inputs and return errors when appropriate
* Use `errors.As()` to extract typed errors from error chains
* Use `errors.Is()` to check for specific underlying errors

### Function Signatures

* Each parameter must have its own type declaration
* Never group parameters with the same type
* Use multiline formatting for functions with many parameters:

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

### Package Organization

```
odh-cli/
├── cmd/
│   ├── main.go          # Entry point
│   └── version/         # Version command
├── pkg/
│   ├── <command>/       # Command-specific logic
│   │   ├── types.go     # Command-specific types
│   │   └── ...          # Additional command logic
│   ├── printer/         # Output formatting
│   │   ├── types.go     # Printer types
│   │   └── table/       # Table rendering
│   └── util/            # Shared utilities
├── internal/
│   └── version/         # Version information
├── go.mod
├── go.sum
└── Makefile
```

### Naming Conventions

* Use camelCase for unexported functions and variables
* Use PascalCase for exported functions and types
* Prefer descriptive names over short abbreviations
* For status constants, use clear, unambiguous names (e.g., `StatusOK`, `StatusError`, `StatusWarning`)

## Testing Guidelines

### Test Framework

* Use vanilla Gomega (not Ginkgo)
* Use dot imports for Gomega: `import . "github.com/onsi/gomega"`
* Use `To`/`ToNot` for `Expect` assertions
* Use `Should`/`ShouldNot` for `Eventually` and `Consistently` assertions
* For error validation: `Expect(err).To(HaveOccurred())` / `Expect(err).ToNot(HaveOccurred())`
* Use subtests (`t.Run`) for organizing related test cases
* Use `t.Context()` instead of `context.Background()` or `context.TODO()` (Go 1.24+)

**Example:**
```go
func TestRenderer(t *testing.T) {
    g := NewWithT(t)
    ctx := t.Context()

    t.Run("should render correctly", func(t *testing.T) {
        result, err := renderer.Process(ctx, nil)
        g.Expect(err).ToNot(HaveOccurred())
        g.Expect(result).To(HaveLen(3))
    })

    t.Run("should eventually become ready", func(t *testing.T) {
        g.Eventually(func() bool {
            return component.IsReady()
        }).Should(BeTrue())
    })

    t.Run("should consistently stay healthy", func(t *testing.T) {
        g.Consistently(func() error {
            return component.HealthCheck()
        }).ShouldNot(HaveOccurred())
    })
}
```

### Test Data Organization

**CRITICAL**: All test data must be defined as package-level constants, never inline within test methods.

**Good:**
```go
const testManifest = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value
`

func TestSomething(t *testing.T) {
    result := parseManifest(testManifest)
    // ...
}
```

**Bad:**
```go
func TestSomething(t *testing.T) {
    manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value
`  // WRONG: inline test data
    result := parseManifest(manifest)
    // ...
}
```

**Rules:**
* ALL test data (YAML, JSON, strings, etc.) must be package-level constants
* Define constants at the top of test files, grouped by test scenario
* Use descriptive names that indicate purpose (e.g., `validCheckResult`, `errorCategoryOutput`)
* Add comments to group related constants (e.g., `// Test constants for check execution`)
* This makes tests more readable and data reusable across tests

### Test Strategy

**Unit Tests**: Test each component in isolation
* Command logic: Test command-specific implementations with mock Kubernetes clients
* Printer: Test table and JSON output formatting
* Utilities: Test shared utility functions and helpers

**Integration Tests**: Test the full command flow
* End-to-end command execution
* Output format switching (table vs JSON)
* Error handling throughout the pipeline

**Test Patterns**:
* Use vanilla Gomega (no Ginkgo)
* Subtests via `t.Run()`
* Use `t.Context()` instead of `context.Background()`
* Mock Kubernetes clients to avoid external dependencies
* Use fake clients from `sigs.k8s.io/controller-runtime/pkg/client/fake` for testing

## Extensibility

### Adding a New Command

Commands follow a consistent pattern that makes them easy to add and maintain. To add a new command:

1. **Create command directory**: `cmd/<commandname>/`
2. **Define command**: Create `command.go` with Cobra command definition
3. **Define options**: Create `options.go` with options struct and `Run()` method
4. **Implement logic**: Create command-specific logic in `pkg/<commandname>/`
5. **Register command**: Add to root command in `cmd/main.go`

**Example Command Structure:**

```go
// cmd/mycommand/command.go
package mycommand

import (
    "github.com/spf13/cobra"
    "k8s.io/cli-runtime/pkg/genericclioptions"
)

func NewMyCommandCmd(streams genericclioptions.IOStreams) *cobra.Command {
    o := NewMyCommandOptions(streams)
    
    cmd := &cobra.Command{
        Use:   "mycommand",
        Short: "Brief description of what the command does",
        RunE: func(cmd *cobra.Command, args []string) error {
            if err := o.Complete(cmd, args); err != nil {
                return err
            }
            if err := o.Validate(); err != nil {
                return err
            }
            return o.Run()
        },
    }
    
    // Add flags
    cmd.Flags().StringVarP(&o.outputFormat, "output", "o", "table", "Output format (table|json)")
    
    return cmd
}
```

```go
// cmd/mycommand/options.go
package mycommand

import (
    "context"
    "k8s.io/cli-runtime/pkg/genericclioptions"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type MyCommandOptions struct {
    configFlags  *genericclioptions.ConfigFlags
    outputFormat string
    streams      genericclioptions.IOStreams
    client       client.Client
    namespace    string
}

func NewMyCommandOptions(streams genericclioptions.IOStreams) *MyCommandOptions {
    return &MyCommandOptions{
        configFlags: genericclioptions.NewConfigFlags(true),
        streams:     streams,
    }
}

func (o *MyCommandOptions) Complete(cmd *cobra.Command, args []string) error {
    // Initialize client, namespace, etc.
    return nil
}

func (o *MyCommandOptions) Validate() error {
    // Validate options
    return nil
}

func (o *MyCommandOptions) Run() error {
    ctx := context.Background()
    
    // Implement command logic
    // Use o.client to interact with Kubernetes
    // Use printer package to format output
    
    return nil
}
```

### Command-Specific Logic

Commands can organize their logic in `pkg/<commandname>/`:

```go
// pkg/mycommand/types.go
package mycommand

type Result struct {
    Name   string
    Status string
    Data   map[string]any
}

// pkg/mycommand/logic.go
package mycommand

func ProcessData(ctx context.Context, client client.Client, namespace string) ([]Result, error) {
    // Command-specific implementation
    return results, nil
}
```

### Adding a New Output Format

To add support for a new output format (e.g., XML, YAML):

1. Add the new format constant to `pkg/printer/types.go`
2. Implement a new printer in `pkg/printer/printer.go`
3. Update the `NewPrinter` factory function
4. Update the output flag validation

**Example:**

```go
// pkg/printer/types.go
const (
    JSON  OutputFormat = "json"
    Table OutputFormat = "table"
    YAML  OutputFormat = "yaml"  // New format
)

// pkg/printer/printer.go
type YAMLPrinter struct {
    out io.Writer
}

func (p *YAMLPrinter) PrintResults(results *doctor.CheckResults) error {
    data, err := yaml.Marshal(results)
    if err != nil {
        return err
    }
    _, err = p.out.Write(data)
    return err
}
```

### Using the Table Renderer with Structs

The table renderer in `pkg/printer/table` supports both slice input (`[]any`) and struct input with automatic field extraction.

#### Basic Struct Usage

```go
type Person struct {
    Name   string
    Age    int
    Status string
}

renderer := table.NewRenderer(
    table.WithHeaders("Name", "Age", "Status"),
)

// Append struct directly
person := Person{Name: "Alice", Age: 30, Status: "active"}
renderer.Append(person)

// Or append multiple
people := []any{person1, person2, person3}
renderer.AppendAll(people)

renderer.Render()
```

#### Field Extraction

The renderer uses [mapstructure](https://github.com/go-viper/mapstructure/v2) to automatically extract struct fields:

- **Case-insensitive matching**: Column names match struct field names case-insensitively
- **Mapstructure tags**: Respects standard mapstructure tags for field mapping
- **Nested fields**: Access nested fields using mapstructure's dot notation in custom formatters

#### Custom Formatters

Column formatters transform values for display:

```go
renderer := table.NewRenderer(
    table.WithHeaders("Name", "Status"),
    table.WithFormatter("Name", func(v any) any {
        return strings.ToUpper(v.(string))
    }),
    table.WithFormatter("Status", func(v any) any {
        status := v.(string)
        if status == "active" {
            return green(status)  // colorize function
        }
        return red(status)
    }),
)
```

#### JQ Formatter

Use `JQFormatter` for complex value extraction and transformation using [jq](https://jqlang.github.io/jq/) syntax:

```go
type Person struct {
    Name     string
    Tags     []string
    Metadata map[string]any
}

renderer := table.NewRenderer(
    table.WithHeaders("Name", "Tags", "Location"),
    
    // Extract and join array
    table.WithFormatter("Tags", table.JQFormatter(". | join(\", \")")),
    
    // Extract nested field with default
    table.WithFormatter("Location", 
        table.JQFormatter(".metadata.location // \"N/A\""),
    ),
)
```

The JQ query is compiled once at setup time. If compilation fails, the renderer will panic (fail-fast behavior).

#### Formatter Composition

Use `ChainFormatters` to build transformation pipelines:

```go
renderer := table.NewRenderer(
    table.WithHeaders("Status", "Location", "Count"),
    
    // Chain: identity + colorization
    table.WithFormatter("Status", 
        table.ChainFormatters(
            table.JQFormatter("."),
            func(v any) any { return colorize(v.(string)) },
        ),
    ),
    
    // Chain: JQ extraction + formatting
    table.WithFormatter("Location", 
        table.ChainFormatters(
            table.JQFormatter(".metadata.location // \"Unknown\""),
            func(v any) any { return fmt.Sprintf("📍 %s", v) },
        ),
    ),
    
    // Chain: extraction + math + formatting
    table.WithFormatter("Count", 
        table.ChainFormatters(
            table.JQFormatter(".items | length"),
            func(v any) any { return fmt.Sprintf("%d items", v) },
        ),
    ),
)
```

The pipeline passes the output of each formatter as input to the next, enabling complex transformations.

#### Complete Example

```go
type CheckResult struct {
    Name     string
    Status   string
    Message  string
    Tags     []string
    Metadata map[string]any
}

renderer := table.NewRenderer(
    table.WithHeaders("Name", "Status", "Message", "Tags", "Priority"),
    
    // Simple formatter
    table.WithFormatter("Name", func(v any) any {
        return strings.ToUpper(v.(string))
    }),
    
    // Chained: identity + colorization
    table.WithFormatter("Status", 
        table.ChainFormatters(
            table.JQFormatter("."),
            func(v any) any {
                status := v.(string)
                switch status {
                case "OK":
                    return green(status)
                case "WARNING":
                    return yellow(status)
                case "ERROR":
                    return red(status)
                default:
                    return status
                }
            },
        ),
    ),
    
    // JQ array join
    table.WithFormatter("Tags", table.JQFormatter(". | join(\", \")")),
    
    // Chained: JQ extraction + formatting
    table.WithFormatter("Priority", 
        table.ChainFormatters(
            table.JQFormatter(".metadata.priority // 0"),
            func(v any) any {
                priority := int(v.(float64))
                return fmt.Sprintf("P%d", priority)
            },
        ),
    ),
)

results := []any{
    CheckResult{
        Name:     "pod-check",
        Status:   "OK",
        Message:  "All pods running",
        Tags:     []string{"core", "critical"},
        Metadata: map[string]any{"priority": 1},
    },
    // ... more results
}

renderer.AppendAll(results)
renderer.Render()
```

## Code Review Guidelines

### Linter Rules

The project uses golangci-lint v2 with a comprehensive configuration (`.golangci.yml`) that enables all linters by default with specific exclusions. Run `make lint` to check your code or `make lint/fix` to automatically fix issues where possible.

**Configuration Highlights:**

* **Enabled**: All linters except those explicitly disabled
* **Disabled linters**: wsl, varnamelen, exhaustruct, ireturn, depguard, err113, paralleltest, funcorder, noinlineerr
* **Test file exclusions**: Many strict linters are disabled for `*_test.go` files to allow for more flexible test code
* **Import ordering**: Uses `gci` formatter to organize imports in sections (standard, default, k8s.io, project, dot)
* **Revive rules**: Enable most revive rules with sensible exclusions for package comments, line length, function length, etc.

**Key Rules:**

* **goconst**: Extract repeated string literals to constants
* **gosec**: No hardcoded secrets (use `//nolint:gosec` only for test data with comment explaining why)
* **staticcheck**: Follow all suggestions
* **Comment formatting**: All comments should be complete sentences ending with periods
* **Error wrapping**: Use `fmt.Errorf` with `%w` for error chains
* **Complexity limits**: cyclop (max 15), gocognit (min 50)

**Running the Linter:**

```bash
# Check for issues
make lint

# Auto-fix issues where possible
make lint/fix

# Run vulnerability scanner
make vulncheck

# Run all checks
make check
```

### Git Commit Conventions

**Commit Message Format:**
```
<type>: <subject>

<body>

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

**Types:**
* `feat`: New feature
* `fix`: Bug fix
* `refactor`: Code refactoring (no functional changes)
* `test`: Adding or updating tests
* `docs`: Documentation changes
* `build`: Build system or dependency changes
* `chore`: Maintenance tasks

**Subject:**
* Use imperative mood ("add feature" not "added feature")
* Don't capitalize first letter
* No period at the end
* Max 72 characters

**Body:**
* Explain what and why (not how)
* Separate from subject with blank line
* Wrap at 72 characters
* Use bullet points for multiple items

**Example:**
```
feat: add pod health check to doctor command

This commit adds a new check that verifies pod readiness status:

- Check all pods in the ODH namespace
- Report WARNING if any pods are not ready
- Report ERROR if pods cannot be listed

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

### Pull Request Checklist

Before submitting a PR:
* [ ] All tests pass (`make test`)
* [ ] All checks pass (`make check` - includes lint and vulncheck)
* [ ] Code formatted (`make fmt`)
* [ ] Dependencies tidied (`make tidy`)
* [ ] New tests added for new features
* [ ] Documentation updated (design.md, development.md, or AGENTS.md as needed)
* [ ] All test data extracted to package-level constants
* [ ] Error handling follows conventions
* [ ] Functional options pattern used for configuration
* [ ] No linter warnings or errors

### Code Style

* **Function signatures**: Each parameter must have its own type declaration (never group parameters with same type)
* **Comments**: Explain *why*, not *what*. Focus on non-obvious behavior, edge cases, and relationships
* **Error wrapping**: Always use `fmt.Errorf` with `%w` for error chains
* **Context propagation**: Pass context through all layers for cancellation support
* **Zero values**: Leverage zero value semantics instead of pointers where appropriate
* **Early returns**: Use early returns to reduce nesting and improve readability

