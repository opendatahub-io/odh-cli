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
* Avoid package name repetition in type names (e.g., use `client.Client`, NOT `client.ClientClient`)

### Code Comments

Comments MUST explain **WHY**, not **WHAT**. Code should be self-documenting through clear naming and structure.

**When comments are REQUIRED:**
- Exported functions, types, and constants (godoc comments)
- Complex algorithms or non-obvious logic
- Business rule explanations
- Workarounds for known issues or limitations
- Security-sensitive code

**When comments are PROHIBITED:**
- Restating what the code obviously does
- Describing language features
- Redundant information already clear from code

**Examples:**

```go
// ❌ BAD: States the obvious
// Set the name field to the provided name
user.Name = name

// ❌ BAD: Describes what, not why
// Loop through all items
for _, item := range items {
    process(item)
}

// ✓ GOOD: Explains why
// Process items sequentially to maintain order dependency between transformations
for _, item := range items {
    process(item)
}

// ✓ GOOD: Explains business rule
// Version detection prioritizes DataScienceCluster over DSCInitialization
// because DSC represents the user's desired state while DSCI is system-level.
version, err := detectVersion(ctx, client)

// ✓ GOOD: Explains workaround
// Use string comparison instead of semantic versioning to avoid
// dependency on semver library. See issue #123.
if versionString > "3.0" {
    // ...
}
```

### Message Constants

User-facing messages MUST be defined as package-level constants, NOT inline strings.

**Rationale:** Constants enable message reuse, consistent wording, easier localization, and prevent typos in error messages.

**Required:**
```go
const (
    msgComponentNotFound     = "component %q not found in cluster"
    msgInvalidVersion        = "invalid version format: %s"
    msgClusterNotReachable   = "unable to connect to cluster: %w"
)

func ValidateComponent(name string) error {
    if !exists(name) {
        return fmt.Errorf(msgComponentNotFound, name)
    }
    return nil
}
```

**Prohibited:**
```go
// ❌ WRONG: Inline string literal
func ValidateComponent(name string) error {
    if !exists(name) {
        return fmt.Errorf("component %q not found in cluster", name)
    }
    return nil
}
```

**Multi-line messages:**
```go
const msgUpgradeBlocked = `Upgrade to version %s is blocked due to:
- Incompatible component configurations
- Deprecated API usage
Run 'kubectl odh lint --target-version %s' for details.`
```

### Command Interface Pattern

All commands must implement the Command interface with four lifecycle methods:

```go
type Command interface {
    AddFlags(fs *pflag.FlagSet)  // Register command-specific flags
    Complete() error              // Initialize runtime state
    Validate() error              // Verify configuration
    Run(ctx context.Context) error // Execute business logic
}
```

**Strict Requirements:**
- Struct MUST be named `Command` (NOT `Options`, `CommandOptions`, or any variant)
- Constructor MUST be named `NewCommand()` (NOT `NewOptions()`, `New()`, or any variant)
- Implementation file MUST be named `<command>.go` (NOT `options.go`, `<command>_options.go`, etc.)
- Commands initialize their own `SharedOptions` internally

**Examples:**

```go
// ✓ CORRECT naming
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

// ❌ WRONG naming
type LintOptions struct {  // Wrong: should be Command
    shared *SharedOptions
}

func NewLintOptions() *LintOptions {  // Wrong: should be NewCommand
    return &LintOptions{}
}
```

See [lint/architecture.md](lint/architecture.md#command-lifecycle) for detailed lifecycle documentation.

### IOStreams Wrapper

Commands must use the IOStreams wrapper (`pkg/util/iostreams/`) to eliminate repetitive output boilerplate.

**Usage:**
```go
// Before (repetitive)
_, _ = fmt.Fprintf(o.Out, "Detected version: %s\n", version)
_, _ = fmt.Fprintf(o.ErrOut, "Error: %v\n", err)

// After (clean)
o.io.Fprintf("Detected version: %s", version)
o.io.Errorf("Error: %v", err)
```

**Methods:**
- `Fprintf(format string, args ...any)` - Write formatted output to stdout
- `Fprintln(args ...any)` - Write output to stdout with newline
- `Errorf(format string, args ...any)` - Write formatted error to stderr
- `Errorln(args ...any)` - Write error to stderr with newline

### JQ-Based Field Access

All operations on `unstructured.Unstructured` objects must use JQ queries via `pkg/util/jq`.

**Required:**
```go
import "github.com/lburgazzoli/odh-cli/pkg/util/jq"

result, err := jq.Query(obj, ".spec.fieldName")
```

**Prohibited:**
Direct use of unstructured accessor methods is prohibited:
- ❌ `unstructured.NestedString()`
- ❌ `unstructured.NestedField()`
- ❌ `unstructured.SetNestedField()`

**Rationale:** JQ provides consistent, expressive queries that align with user-facing JQ integration and eliminate verbose nested accessor chains.

For lint check examples, see [lint/writing-checks.md](lint/writing-checks.md#jq-based-field-access).

### Centralized GVK/GVR Definitions

All GroupVersionKind (GVK) and GroupVersionResource (GVR) references must use definitions from `pkg/resources/types.go`.

**Required:**
```go
import "github.com/lburgazzoli/odh-cli/pkg/resources"

gvk := resources.DataScienceCluster.GVK()
gvr := resources.DataScienceCluster.GVR()
apiVersion := resources.DataScienceCluster.APIVersion()
```

**Prohibited:**
Direct construction of GVK/GVR structs:
```go
// ❌ WRONG
gvk := schema.GroupVersionKind{
    Group:   "datasciencecluster.opendatahub.io",
    Version: "v1",
    Kind:    "DataScienceCluster",
}
```

**Rationale:** Centralized definitions eliminate string literals across the codebase, prevent typos, and provide a single source of truth for API resource references.

For lint check examples, see [lint/writing-checks.md](lint/writing-checks.md#centralized-gvkgvr-usage).

### High-Level Resource Operations

When working with OpenShift AI resources, operate on high-level custom resources rather than low-level Kubernetes primitives.

**Preferred:**
- Component CRs (DataScienceCluster, DSCInitialization)
- Workload CRs (Notebook, InferenceService, RayCluster, etc.)
- Service CRs, CRDs, ClusterServiceVersions

**Avoid as Primary Targets:**
- Pod, Deployment, StatefulSet, Service
- ConfigMap, Secret, PersistentVolume

**Rationale:** OpenShift AI users interact with high-level CRs, not low-level primitives. Operations targeting low-level resources don't align with user-facing abstractions.

For lint check requirements, see [lint/writing-checks.md](lint/writing-checks.md#high-level-resource-targeting).

### Cluster-Wide Operations

When working with OpenShift AI resources, operations typically span all namespaces rather than being constrained to a single namespace.

**General pattern:**
```go
// List across all namespaces
err := client.List(ctx, objectList)  // No namespace restriction
```

**Rationale:** OpenShift AI is a cluster-wide platform. Operations often require visibility into all namespaces.

For lint command requirements, see [lint/writing-checks.md](lint/writing-checks.md#cluster-wide-scope).

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

### Mock Organization

**Critical Requirement:** Mocks MUST use testify/mock framework and be centralized in `pkg/util/test/mocks/<package>/`.

**Location Pattern:**
```
pkg/util/test/mocks/
├── client/
│   └── mock_client.go       # Mock for pkg/client
├── printer/
│   └── mock_printer.go      # Mock for pkg/printer
└── version/
    └── mock_detector.go     # Mock for pkg/lint/version
```

**Example Mock:**
```go
// pkg/util/test/mocks/version/mock_detector.go
package version

import (
    "context"
    "github.com/stretchr/testify/mock"
    "github.com/lburgazzoli/odh-cli/pkg/lint/version"
)

type MockDetector struct {
    mock.Mock
}

func (m *MockDetector) Detect(ctx context.Context) (*version.ClusterVersion, error) {
    args := m.Called(ctx)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*version.ClusterVersion), args.Error(1)
}
```

**Usage in Tests:**
```go
import (
    mockversion "github.com/lburgazzoli/odh-cli/pkg/util/test/mocks/version"
)

func TestWithMock(t *testing.T) {
    detector := &mockversion.MockDetector{}
    detector.On("Detect", mock.Anything).Return(&version.ClusterVersion{
        Version: "3.0.0",
    }, nil)

    // Test code using detector
    detector.AssertExpectations(t)
}
```

**Prohibited:**
```go
// ❌ WRONG: Inline mock
type mockDetector struct{}

func (m *mockDetector) Detect(ctx context.Context) (*version.ClusterVersion, error) {
    return &version.ClusterVersion{Version: "3.0.0"}, nil
}
```

### Gomega Struct Assertions

**Critical Requirement:** For struct assertions, MUST use `HaveField` or `MatchFields`. Individual field assertions are PROHIBITED.

**Required:**
```go
import . "github.com/onsi/gomega"

// ✓ CORRECT: Use HaveField for single field
g.Expect(result).To(HaveField("Metadata.Group", "components"))
g.Expect(result).To(HaveField("Metadata.Kind", "kserve"))

// ✓ CORRECT: Use MatchFields for multiple fields
g.Expect(result.Metadata).To(MatchFields(IgnoreExtras, Fields{
    "Group": Equal("components"),
    "Kind":  Equal("kserve"),
    "Name":  Equal("serverless-removal"),
}))

// ✓ CORRECT: Nested struct matching
g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
    "Type":   Equal("ServerlessRemoved"),
    "Status": Equal(metav1.ConditionTrue),
    "Reason": Equal("ServerlessRemoved"),
}))
```

**Prohibited:**
```go
// ❌ WRONG: Individual field assertions
g.Expect(result.Metadata.Group).To(Equal("components"))
g.Expect(result.Metadata.Kind).To(Equal("kserve"))
g.Expect(result.Metadata.Name).To(Equal("serverless-removal"))
```

**Rationale:** Struct matchers provide clearer test output on failure, showing exactly which fields don't match in a single assertion rather than stopping at the first failed field.

## Extensibility

### Adding a New Command

Commands follow a consistent pattern separating Cobra wrappers from business logic:

1. **Create Cobra wrapper**: `cmd/<commandname>/<commandname>.go` - minimal Cobra command definition
2. **Create business logic**: `pkg/cmd/<commandname>/<commandname>.go` - Options struct with Complete/Validate/Run
3. **Add supporting code**: `pkg/<commandname>/` - domain-specific logic and utilities
4. **Register command**: Add to parent command (e.g., `cmd/main.go`)

#### Directory Structure

```
cmd/
└── mycommand/
    └── mycommand.go          # Cobra wrapper only
pkg/
├── cmd/
│   └── mycommand/
│       └── mycommand.go      # Options struct + Complete/Validate/Run
└── mycommand/                # Domain logic (optional)
    ├── types.go
    └── utilities.go
```

#### Pattern: Cobra Wrapper (cmd/)

The Cobra wrapper in `cmd/` should be minimal - only command metadata and flag bindings:

```go
// cmd/mycommand/mycommand.go
package mycommand

import (
    "os"
    "github.com/spf13/cobra"
    "k8s.io/cli-runtime/pkg/genericclioptions"
    pkgcmd "github.com/lburgazzoli/odh-cli/pkg/cmd/mycommand"
)

const (
    cmdName  = "mycommand"
    cmdShort = "Brief description"
    cmdLong  = `Detailed description...`
)

func AddCommand(parent *cobra.Command, flags *genericclioptions.ConfigFlags) {
    o := pkgcmd.NewMyCommandOptions(
        genericclioptions.IOStreams{
            In:     os.Stdin,
            Out:    os.Stdout,
            ErrOut: os.Stderr,
        },
        flags,
    )

    cmd := &cobra.Command{
        Use:   cmdName,
        Short: cmdShort,
        Long:  cmdLong,
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

    // Bind flags to Options struct fields
    cmd.Flags().StringVarP(&o.OutputFormat, "output", "o", "table", "Output format")

    parent.AddCommand(cmd)
}
```

#### Pattern: Business Logic (pkg/cmd/)

The Options struct in `pkg/cmd/` contains all business logic:

```go
// pkg/cmd/mycommand/mycommand.go
package mycommand

import (
    "context"
    "fmt"
    "k8s.io/cli-runtime/pkg/genericclioptions"
    utilclient "github.com/lburgazzoli/odh-cli/pkg/util/client"
)

type MyCommandOptions struct {
    configFlags  *genericclioptions.ConfigFlags
    streams      genericclioptions.IOStreams
    
    // Public fields for flag binding
    OutputFormat string
    
    // Private fields for runtime state
    client    *utilclient.Client
    namespace string
}

func NewMyCommandOptions(
    streams genericclioptions.IOStreams,
    configFlags *genericclioptions.ConfigFlags,
) *MyCommandOptions {
    return &MyCommandOptions{
        configFlags: configFlags,
        streams:     streams,
    }
}

// Complete initializes runtime state (client, namespace, etc.)
func (o *MyCommandOptions) Complete(cmd *cobra.Command, args []string) error {
    var err error
    
    o.client, err = utilclient.NewClient(o.configFlags)
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }
    
    // Extract namespace if needed
    if o.configFlags.Namespace != nil && *o.configFlags.Namespace != "" {
        o.namespace = *o.configFlags.Namespace
    }
    
    return nil
}

// Validate checks that all required options are set correctly
func (o *MyCommandOptions) Validate() error {
    validFormats := []string{"table", "json", "yaml"}
    for _, format := range validFormats {
        if o.OutputFormat == format {
            return nil
        }
    }
    return fmt.Errorf("unsupported output format: %s", o.OutputFormat)
}

// Run executes the command business logic
func (o *MyCommandOptions) Run() error {
    ctx := context.Background()
    
    // Implement command logic using o.client, o.streams, etc.
    // Call domain-specific functions from pkg/mycommand/
    
    return nil
}
```

#### Benefits of This Pattern

- **Separation of Concerns**: Cobra configuration isolated from business logic
- **Testability**: Options struct can be tested without Cobra dependencies
- **Reusability**: Business logic can be called programmatically
- **Consistency**: All commands follow the same structure
- **kubectl Compatibility**: Follows patterns used by kubectl and kubectl plugins

### Command-Specific Logic

Commands can organize domain-specific logic in `pkg/<commandname>/`:

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

func ProcessData(ctx context.Context, client *utilclient.Client, namespace string) ([]Result, error) {
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

## Continuous Quality Verification

**Critical Requirement:** MUST run `make check` after EVERY implementation. This is NOT optional.

Quality verification is a mandatory part of the development workflow, not a pre-commit step. All code changes must pass quality gates before being considered complete.

### Development Workflow

1. **Make code changes**
2. **Run `make lint-fix`** - Auto-fix formatting and simple issues
3. **Run `make lint`** - Check for remaining linting issues
4. **Manual fixes** - Address issues that can't be auto-fixed
5. **Run `make check`** - Complete quality verification (lint + vulncheck + tests)

**make check includes:**
- `make lint` - golangci-lint with all enabled linters
- `make vulncheck` - Security vulnerability scanning
- `make test` - All unit and integration tests

### Lint-Fix-First

**Always run auto-fix before manual fixes:**

```bash
# ✓ CORRECT workflow
make lint-fix    # Auto-fix first
make lint        # Check what remains
# manually fix remaining issues
make check       # Final verification

# ❌ WRONG workflow
make lint        # Check issues
# manually fix all issues without trying auto-fix
make check
```

**Rationale:** `lint-fix` automatically resolves 80%+ of common issues (formatting, imports, simple patterns). Manual fixes should only address issues that require human judgment.

### Quality Gates

All of these MUST pass before code is considered complete:

**Linting:**
```bash
make lint
```

**Vulnerability Check:**
```bash
make vulncheck
```

**Tests:**
```bash
make test
```

**Complete Check (all of the above):**
```bash
make check
```

### When to Run

- After **every** implementation (function, method, test)
- Before **every** commit
- After resolving merge conflicts
- When resuming work on a branch

**NOT optional.** Quality verification is part of implementation, not a separate step.

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

**Task-Based Commits:**

When implementing tasks from `specs/*/tasks.md`, commit granularity follows task boundaries:

- **One commit per task**: Each task gets exactly one commit
- **Task ID in subject**: Use format `T###: <description>` where ### is the task number
- **Grouped tasks**: Multiple related tasks can be `T###, T###: <description>`

**Task Commit Examples:**
```
T001: implement Check interface for serverless removal validation

Adds the Check interface implementation that validates serverless
components are removed when upgrading to 3.x.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

```
T005, T006: add tests for serverless check and version detection

Groups two related testing tasks into a single commit.

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

