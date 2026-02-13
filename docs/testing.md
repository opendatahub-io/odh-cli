# Testing Guidelines

This document covers testing practices, frameworks, and conventions for odh-cli.

For development workflow, see [development.md](development.md). For quality verification, see [quality.md](quality.md).

## Test Framework

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

## Test Data Organization

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

## Test Strategy

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

## Mock Organization

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
    "github.com/opendatahub-io/odh-cli/pkg/lint/version"
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
    mockversion "github.com/opendatahub-io/odh-cli/pkg/util/test/mocks/version"
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

## Gomega Struct Assertions

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

## Use Kubernetes Sets for Deduplication

When collecting unique values (resource names, labels, etc.), use `k8s.io/apimachinery/pkg/util/sets` instead of `map[string]bool`:

```go
import "k8s.io/apimachinery/pkg/util/sets"

// ✓ CORRECT: Use sets
names := sets.New[string]()
for _, vol := range volumes {
    if vol.ConfigMap != nil && vol.ConfigMap.Name != "" {
        names.Insert(vol.ConfigMap.Name)
    }
}
return sets.List(names)

// ❌ WRONG: Manual map and conversion
names := make(map[string]bool)
for _, vol := range volumes {
    if vol.ConfigMap != nil && vol.ConfigMap.Name != "" {
        names[vol.ConfigMap.Name] = true
    }
}
result := make([]string, 0, len(names))
for name := range names {
    result = append(result, name)
}
return result
```

**Benefits:**
- More expressive and idiomatic
- Built-in set operations (Union, Difference, Intersection)
- Sorted output with `sets.List()`

## Use Generics for Type Conversion

When converting unstructured data to typed Kubernetes objects, use generics with the type specified by the caller:

```go
// ✓ CORRECT: Generic function, type specified by caller
func ConvertToTyped[T any](raw any, typeName string) (T, error) {
    var zero T
    if raw == nil {
        return zero, nil
    }

    data, err := json.Marshal(raw)
    if err != nil {
        return zero, fmt.Errorf("marshaling %s: %w", typeName, err)
    }

    var result T
    if err := json.Unmarshal(data, &result); err != nil {
        return zero, fmt.Errorf("unmarshaling %s: %w", typeName, err)
    }

    return result, nil
}

// Usage: Caller specifies slice or single object
volumes, err := ConvertToTyped[[]corev1.Volume](raw, "volumes")
container, err := ConvertToTyped[corev1.Container](raw, "container")

// ❌ WRONG: Separate function for each type
func ConvertToVolumes(raw []any) ([]corev1.Volume, error) { ... }
func ConvertToContainers(raw []any) ([]corev1.Container, error) { ... }
```

**Benefits:**
- Single reusable function for all conversions
- Caller controls whether result is a slice or single value
- Type-safe at compile time
