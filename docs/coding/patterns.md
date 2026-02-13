# Architectural Patterns

This document covers architectural patterns and design practices used throughout odh-cli.

For core coding conventions, see [conventions.md](conventions.md). For code formatting, see [formatting.md](formatting.md).

## Functional Options Pattern

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

## IOStreams Wrapper

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

## JQ-Based Field Access

All operations on `unstructured.Unstructured` objects must use JQ queries via `pkg/util/jq`.

**Required:**
```go
import "github.com/opendatahub-io/odh-cli/pkg/util/jq"

result, err := jq.Query(obj, ".spec.fieldName")
```

**Prohibited:**
Direct use of unstructured accessor methods is prohibited:
- ❌ `unstructured.NestedString()`
- ❌ `unstructured.NestedField()`
- ❌ `unstructured.SetNestedField()`

**Rationale:** JQ provides consistent, expressive queries that align with user-facing JQ integration and eliminate verbose nested accessor chains.

For lint check examples, see [../lint/writing-checks.md](../lint/writing-checks.md#jq-based-field-access).

## Centralized GVK/GVR Definitions

All GroupVersionKind (GVK) and GroupVersionResource (GVR) references must use definitions from `pkg/resources/types.go`.

**Required:**
```go
import "github.com/opendatahub-io/odh-cli/pkg/resources"

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

For lint check examples, see [../lint/writing-checks.md](../lint/writing-checks.md#centralized-gvkgvr-usage).

## High-Level Resource Operations

When working with OpenShift AI resources, operate on high-level custom resources rather than low-level Kubernetes primitives.

**Preferred:**
- Component CRs (DataScienceCluster, DSCInitialization)
- Workload CRs (Notebook, InferenceService, RayCluster, etc.)
- Service CRs, CRDs, ClusterServiceVersions

**Avoid as Primary Targets:**
- Pod, Deployment, StatefulSet, Service
- ConfigMap, Secret, PersistentVolume

**Rationale:** OpenShift AI users interact with high-level CRs, not low-level primitives. Operations targeting low-level resources don't align with user-facing abstractions.

For lint check requirements, see [../lint/writing-checks.md](../lint/writing-checks.md#high-level-resource-targeting).

## Cluster-Wide Operations

When working with OpenShift AI resources, operations typically span all namespaces rather than being constrained to a single namespace.

**General pattern:**
```go
// List across all namespaces
err := client.List(ctx, objectList)  // No namespace restriction
```

**Rationale:** OpenShift AI is a cluster-wide platform. Operations often require visibility into all namespaces.

For lint command requirements, see [../lint/writing-checks.md](../lint/writing-checks.md#cluster-wide-scope).

## Metadata-Only Resource Retrieval

When you only need resource metadata (name, namespace, labels, annotations) and not spec/status fields, use `PartialObjectMetadata` for efficient retrieval.

**Use `Client.ListMetadata()` for discovery:**
```go
import "github.com/opendatahub-io/odh-cli/pkg/resources"

// Efficiently list only metadata (no spec/status)
notebooks, err := client.ListMetadata(ctx, resources.Notebook)
if err != nil {
    return fmt.Errorf("listing notebooks: %w", err)
}

for _, nb := range notebooks {
    // Access metadata fields
    name := nb.GetName()
    namespace := nb.GetNamespace()
    annotations := nb.GetAnnotations()
}
```

**When to use metadata-only retrieval:**
- Checking for resource existence
- Reading annotations or labels
- Building lists of resource references
- Counting resources by type

**When full object retrieval is required:**
- Accessing `.spec` fields (configuration)
- Accessing `.status` fields (current state)
- Using JQ queries on nested structures

**Creating PartialObjectMetadata for output:**
```go
import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

obj := metav1.PartialObjectMetadata{
    TypeMeta: resources.Notebook.TypeMeta(),
    ObjectMeta: metav1.ObjectMeta{
        Namespace: "my-namespace",
        Name:      "my-notebook",
        Annotations: map[string]string{
            "context-key": "context-value",
        },
    },
}
```

**Benefits:**
- Reduced memory usage (no spec/status data)
- Faster API responses (server-side filtering)
- Cleaner code when full object not needed
