# Extensibility

This document covers how to extend odh-cli by adding new commands, output formats, and features.

For coding conventions, see [coding/conventions.md](coding/conventions.md) and [coding/patterns.md](coding/patterns.md).

## Adding a New Command

Commands follow a consistent pattern separating Cobra wrappers from business logic:

1. **Create Cobra wrapper**: `cmd/<commandname>/<commandname>.go` - minimal Cobra command definition
2. **Create business logic**: `pkg/cmd/<commandname>/<commandname>.go` - Options struct with Complete/Validate/Run
3. **Add supporting code**: `pkg/<commandname>/` - domain-specific logic and utilities
4. **Register command**: Add to parent command (e.g., `cmd/main.go`)

### Directory Structure

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

### Pattern: Cobra Wrapper (cmd/)

The Cobra wrapper in `cmd/` should be minimal - only command metadata and flag bindings:

```go
// cmd/mycommand/mycommand.go
package mycommand

import (
    "os"
    "github.com/spf13/cobra"
    "k8s.io/cli-runtime/pkg/genericclioptions"
    pkgcmd "github.com/opendatahub-io/odh-cli/pkg/cmd/mycommand"
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

### Pattern: Business Logic (pkg/cmd/)

The Options struct in `pkg/cmd/` contains all business logic:

```go
// pkg/cmd/mycommand/mycommand.go
package mycommand

import (
    "context"
    "fmt"
    "k8s.io/cli-runtime/pkg/genericclioptions"
    utilclient "github.com/opendatahub-io/odh-cli/pkg/util/client"
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

### Benefits of This Pattern

- **Separation of Concerns**: Cobra configuration isolated from business logic
- **Testability**: Options struct can be tested without Cobra dependencies
- **Reusability**: Business logic can be called programmatically
- **Consistency**: All commands follow the same structure
- **kubectl Compatibility**: Follows patterns used by kubectl and kubectl plugins

## Command-Specific Logic

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

## Adding a New Output Format

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

## Using the Table Renderer with Structs

The table renderer in `pkg/printer/table` supports both slice input (`[]any`) and struct input with automatic field extraction.

### Basic Struct Usage

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

### Field Extraction

The renderer uses [mapstructure](https://github.com/go-viper/mapstructure/v2) to automatically extract struct fields:

- **Case-insensitive matching**: Column names match struct field names case-insensitively
- **Mapstructure tags**: Respects standard mapstructure tags for field mapping
- **Nested fields**: Access nested fields using mapstructure's dot notation in custom formatters

### Custom Formatters

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

### JQ Formatter

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

### Formatter Composition

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

### Complete Example

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
