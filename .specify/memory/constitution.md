<!--
============================================================================
SYNC IMPACT REPORT
============================================================================
Version Change: 1.20.0 → 1.20.1
Modified Principles:
  - Updated Principle XII: Kubernetes-Native Diagnostic CR Structure
    - Removed condition message truncation requirement from validation
    - Message truncation (if needed) should be applied in table rendering only
    - Preserves full messages in JSON/YAML output
Modified Sections:
  - Quality Gates: Constitution Check Gates - Removed "condition message truncation" from Phase 2
Removed Sections: None
Templates Requiring Updates:
  ✅ .specify/templates/plan-template.md - Generic template, gates auto-populate from constitution
  ✅ .specify/templates/spec-template.md - Generic template, intentionally not constitution-specific
  ✅ .specify/templates/tasks-template.md - Generic template, intentionally not constitution-specific
  ✅ .specify/templates/agent-file-template.md - Generic template, intentionally not constitution-specific
  ✅ .specify/templates/checklist-template.md - Generic template, intentionally not constitution-specific
Follow-up TODOs: None

Rationale for PATCH bump (1.20.0 → 1.20.1):
- Removes message truncation from validation to preserve full messages in structured output
- Truncation belongs in presentation layer (table rendering), not data layer (validation)
- Clarifies that DiagnosticResult should not modify condition messages
- No breaking changes to validation contract, only removes a modification side effect
============================================================================
-->

# odh-cli Constitution

## Core Principles

### I. kubectl Plugin Integration

The CLI MUST function as a native kubectl plugin following kubectl UX patterns. The binary MUST be named `kubectl-odh` and automatically discovered when placed in PATH. The CLI MUST leverage the user's active kubeconfig for cluster authentication without requiring separate configuration.

**Rationale**: Users interacting with Kubernetes expect kubectl-like tools. Following kubectl conventions reduces cognitive load and provides a familiar, consistent experience.

### II. Extensible Command Structure

All commands MUST follow the modular Cobra-based pattern separating command definition (cmd/) from business logic (pkg/cmd/). New commands MUST be independently testable without Cobra dependencies. Each command MUST implement the Command interface for consistent lifecycle management.

**Command Interface Requirements**:
- All command implementations MUST define a `Command` struct (NOT `Options`)
- Constructor MUST be named `NewCommand()` (NOT `NewOptions()`)
- Implementation file MUST be named `<command>.go` (NOT `options.go`)
- Command MUST implement the following methods:
  - `Complete() error` - Populate fields, create clients, parse inputs
  - `Validate() error` - Validate all required fields and constraints
  - `Run(ctx context.Context) error` - Execute the command logic
  - `AddFlags(fs *pflag.FlagSet)` - Register command-specific flags
- Command MUST initialize its own `SharedOptions` internally
- A `Command` interface SHOULD be defined for type safety and testing

**Example Structure**:
```go
// pkg/cmd/lint/lint.go
package lint

import "github.com/spf13/pflag"

type Command struct {
    shared        *lint.SharedOptions
    targetVersion string // lint-specific field
}

func NewCommand(opts CommandOptions) *Command {
    return &Command{
        shared:        opts.Shared,
        targetVersion: opts.TargetVersion,
    }
}

func (c *Command) Complete() error { /* ... */ }
func (c *Command) Validate() error { /* ... */ }
func (c *Command) Run(ctx context.Context) error { /* ... */ }
func (c *Command) AddFlags(fs *pflag.FlagSet) {
    fs.StringVar(&c.targetVersion, "target-version", "", "Target version for upgrade assessment")
    // ... other flags
}
```

**Rationale**: Separation of concerns enables independent testing, code reuse, and maintains a consistent structure as the CLI grows. The Command interface provides a clear contract for all command implementations, making the codebase more maintainable and testable. Using "Command" instead of "Options" better reflects the struct's purpose as a command executor, not just option storage. The AddFlags method centralizes flag registration, making it easier to test and maintain flag definitions. This pattern is standard in kubectl plugins and kubectl itself.

### III. Consistent Output Formats

All commands MUST support table (default), JSON, and YAML output formats via the `-o/--output` flag. Table output MUST be human-readable with consistent formatting. JSON and YAML output MUST be machine-parsable and suitable for scripting.

**JSON and YAML List Format Requirements**:
- JSON and YAML output MUST use Kubernetes List pattern with `kind`, `metadata`, and `items` fields
- List `kind` MUST be `"DiagnosticResultList"`
- List `metadata` MUST contain `clusterVersion` and `targetVersion` fields (optional, omitted if null)
- `items` array MUST contain flat list of `DiagnosticResult` objects in execution order
- Each `DiagnosticResult` MUST include `kind: "DiagnosticResult"` field
- Category information MUST be preserved in each item's `metadata.group` field
- Results MUST be in execution order (sequential, not grouped by category)
- Parallel execution is PROHIBITED to ensure deterministic ordering

**Example Structure**:
```json
{
  "kind": "DiagnosticResultList",
  "metadata": {
    "clusterVersion": "2.17.0",
    "targetVersion": "3.0.0"
  },
  "items": [
    {
      "kind": "DiagnosticResult",
      "metadata": {
        "group": "component",
        "kind": "kserve",
        "name": "serverless-removal",
        "annotations": {...}
      },
      "spec": {
        "description": "..."
      },
      "status": {
        "conditions": [...]
      }
    }
  ]
}
```

**Rationale**: Different consumers need different formats. Humans need readable tables, scripts need structured JSON/YAML. The Kubernetes List pattern provides a familiar structure for kubectl users, enables easy post-processing with jq/yq, maintains category information in metadata.group, and ensures deterministic ordering through sequential execution. This format aligns with `kubectl get -o json` output conventions and is optimized for pipeability and tool composition.

### IV. Flexible Initialization Patterns

All command and struct initialization MUST support BOTH struct-based initialization AND functional options patterns. Struct initialization is PREFERRED for simplicity. Functional options provide advanced configuration capabilities when needed.

**Initialization Requirements**:
- Commands MUST accept a struct for configuration (e.g., `NewCommand(opts CommandOptions)`)
- Commands MAY additionally support functional options (e.g., `NewCommand(WithIO(...), WithConfigFlags(...))`)
- Option types MUST be defined in `*_options.go` files
- Functional option functions MUST be named `With<Property>(value)` and return an option function
- Option functions MUST apply configuration via closure or `ApplyTo(target *T)` method

**Examples**:

**Struct Initialization** (PREFERRED):
```go
// pkg/cmd/lint/lint_options.go
package lint

type CommandOptions struct {
    Shared        *lint.SharedOptions
    TargetVersion string
}

// Usage
cmd := lint.NewCommand(lint.CommandOptions{
    Shared:        sharedOpts,
    TargetVersion: "3.0",
})
```

**Functional Options** (for complex cases):
```go
// pkg/cmd/lint/lint_options.go
package lint

type CommandOption func(*Command)

func WithTargetVersion(version string) CommandOption {
    return func(c *Command) {
        c.targetVersion = version
    }
}

func WithShared(shared *lint.SharedOptions) CommandOption {
    return func(c *Command) {
        c.shared = shared
    }
}

// Usage
cmd := lint.NewCommand(
    lint.WithShared(sharedOpts),
    lint.WithTargetVersion("3.0"),
)
```

**Rationale**: Struct initialization provides simplicity and type safety for common cases. Functional options enable optional parameters, backward-compatible API evolution, and complex configuration scenarios. Supporting both patterns gives developers flexibility: use struct initialization for straightforward cases, functional options for advanced needs. This aligns with Go community practices where simple structs are preferred unless functional options provide clear benefits.

### V. Strict Error Handling

Errors MUST be wrapped using `fmt.Errorf` with `%w` for proper error chain propagation. Context MUST be passed through all operations for cancellation support. First error encountered MUST stop processing and be returned immediately. All constructors MUST validate inputs and return errors when appropriate.

**Rationale**: Proper error handling enables debugging, supports graceful degradation, and provides meaningful error messages to users. Context propagation is essential for timeout and cancellation support.

### VI. Test-First Development

Tests MUST use vanilla Gomega (no Ginkgo). All test data MUST be defined as package-level constants, never inline. Tests MUST use subtests via `t.Run()`. Tests MUST use `t.Context()` for context creation. Both unit tests (isolated components) and integration tests (full command flow) are REQUIRED.

**Testing Infrastructure**:
- Unit tests MUST use fake client from `k8s.io/client-go/dynamic/fake` for Kubernetes client mocking
- Integration tests MUST use k3s-envtest (`github.com/lburgazzoli/k3s-envtest`) for full cluster simulation
- Unit tests MUST NOT require a real cluster or network access
- Integration tests MAY use real cluster resources via k3s-envtest

**Mocking**:
- Test mocks MUST use `github.com/stretchr/testify/mock` framework
- Inline mock structs implementing interfaces are PROHIBITED (except for trivial test-specific cases)
- Reusable mocks MUST be placed in `pkg/util/test/mocks/<package>` for cross-test sharing
- Mock generation via `mockery` is recommended for complex interfaces

**Gomega Assertions**:
- Struct validation MUST use Gomega's struct matchers instead of individual field assertions
- Use `g.Expect(obj).To(HaveField("FieldName", expectedValue))` for single field checks
- Use `g.Expect(obj).To(MatchFields(IgnoreExtras, Fields{...}))` for multiple field checks
- PROHIBITED: Individual field assertions like `g.Expect(obj.Field).To(Equal(value))`
- These matchers provide better error messages showing the full struct context when assertions fail

**Examples**:

**Bad** (individual field assertions):
```go
g.Expect(result.Status).To(Equal(check.StatusPass))
g.Expect(result.Message).To(ContainSubstring("ready"))
g.Expect(result.Severity).To(BeNil())
```

**Good** (struct field matchers):
```go
g.Expect(result).To(HaveField("Status", check.StatusPass))
g.Expect(result).To(HaveField("Severity", BeNil()))
g.Expect(result.Message).To(ContainSubstring("ready"))

// Or for multiple fields:
g.Expect(result).To(MatchFields(IgnoreExtras, Fields{
    "Status":   Equal(check.StatusPass),
    "Severity": BeNil(),
    "Message":  ContainSubstring("ready"),
}))
```

**Rationale**: Test-first ensures correctness, enables refactoring, and serves as living documentation. Package-level constants improve readability and enable test data reuse. Fake clients enable fast, isolated unit tests. k3s-envtest provides realistic integration testing without external cluster dependencies. testify/mock provides a standardized, feature-rich mocking framework with assertion capabilities. Centralized mocks prevent duplication and ensure consistency across tests. Gomega struct matchers provide clearer failure diagnostics by showing the full struct context rather than isolated field values.

### VII. JQ-Based Field Access

All operations on `unstructured.Unstructured` objects MUST use JQ queries via the `pkg/util/jq` package. Direct use of `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured` accessor methods is PROHIBITED. This includes: `NestedField()`, `NestedString()`, `NestedBool()`, `NestedInt64()`, `NestedStringSlice()`, `NestedFieldCopy()`, `SetNestedField()`, `SetNestedStringMap()`, `RemoveNestedField()`, and similar functions.

**Field Operations**:
- Reading fields MUST use `jq.Query(obj, ".path.to.field")`
- Setting fields MUST use JQ-based mutation functions from `pkg/util/jq`
- Complex queries MUST leverage JQ's full query syntax and operators

**Rationale**: JQ provides a consistent, expressive query language that eliminates verbose nested accessor chains, reduces error-prone path construction, aligns internal operations with user-facing JQ integration (table renderer, output formatting), and provides familiar syntax for kubectl users. This enables declarative field access and reuse of query logic across the codebase.

### VIII. Centralized Resource Type Definitions

All GroupVersionKind (GVK) and GroupVersionResource (GVR) references MUST be defined in `pkg/resources/types.go`. Direct construction of GVK/GVR structs in business logic is PROHIBITED. Each Kubernetes resource type MUST have a corresponding exported variable providing GVK and GVR accessors.

**ResourceType Structure**:
- All fields MUST be public (exported): `Group`, `Version`, `Kind`, `Resource`
- This enables reuse in tests and other code without hardcoded strings
- Example: `resources.DataScienceCluster.APIVersion()` instead of `"datasciencecluster.opendatahub.io/v1"`

**Usage Pattern**:
- Accessing GVK MUST use `resources.<ResourceType>.GVK()`
- Accessing GVR MUST use `resources.<ResourceType>.GVR()`
- Accessing API version MUST use `resources.<ResourceType>.APIVersion()`
- Accessing individual fields (for tests) MUST use `resources.<ResourceType>.Kind`, `resources.<ResourceType>.Group`, etc.
- Direct struct construction (e.g., `schema.GroupVersionResource{Group: "apps", ...}`) is PROHIBITED
- Hardcoded strings for apiVersion, kind, etc. in test data is PROHIBITED

**Rationale**: Centralizing GVK/GVR definitions eliminates scattered string literals across the codebase, prevents typos in group/version/resource names, provides a single source of truth for API resource references, and enables easy version migrations. Public fields allow tests to construct unstructured objects using resource type definitions instead of hardcoded strings. This pattern is essential for maintainability when working with dynamic clients and unstructured objects, as it ensures consistency and reduces the risk of runtime errors from malformed resource identifiers.

### IX. High-Level Resource Checks

Diagnostic checks MUST operate exclusively on high-level custom resources representing user-facing abstractions. Checks targeting low-level Kubernetes primitives (Pods, Deployments, StatefulSets, ReplicaSets, Services, ConfigMaps, Secrets, etc.) are PROHIBITED.

**Permitted Check Targets**:
- Component CRs: DataScienceCluster, DSCInitialization
- Workload CRs: Notebook, InferenceService, LLMInferenceService, RayCluster, PyTorchJob, TFJob, DataSciencePipelinesApplication, TrustyAIService, ModelRegistry, etc.
- Service CRs: Custom resources representing OpenShift AI services
- CRDs: CustomResourceDefinition (for validating CRD presence and status)
- OLM resources: ClusterServiceVersion, Subscription (for version detection and operator validation)

**Prohibited Check Targets**:
- Core Kubernetes resources: Pod, Deployment, StatefulSet, ReplicaSet, DaemonSet
- Networking resources: Service, Ingress, Route (unless part of a high-level CR validation)
- Configuration resources: ConfigMap, Secret (unless part of a high-level CR validation)
- Storage resources: PersistentVolume, PersistentVolumeClaim (unless part of a high-level CR validation)

**Exception**: Low-level resources MAY be queried as supporting evidence during high-level CR validation (e.g., checking if a Dashboard CR's backing Deployment exists), but MUST NOT be the primary target of a check.

**Rationale**: OpenShift AI users interact with high-level custom resources (Notebooks, InferenceServices, etc.), not low-level Kubernetes primitives. Diagnostic checks targeting low-level resources produce noise and false positives because they don't align with user-facing abstractions. Enforcing high-level checks ensures diagnostics remain relevant to OpenShift AI semantics, reduces operational complexity, and prevents checks from duplicating Kubernetes' own self-healing mechanisms. This principle aligns with OpenShift AI's operator-managed architecture where low-level resources are implementation details managed by controllers.

### X. Cluster-Wide Diagnostic Scope

The doctor command MUST operate cluster-wide and scan all namespaces. Namespace filtering via `--namespace` or `-n` flags is PROHIBITED for diagnostic commands.

**Scope Requirements**:
- Component checks MUST examine cluster-scoped resources (DataScienceCluster, DSCInitialization, CRDs, ClusterServiceVersions)
- Service checks MUST examine cluster-scoped or all-namespace resources
- Workload checks MUST discover and validate workloads across ALL namespaces
- Discovery operations MUST NOT be constrained by namespace boundaries

**Prohibited**:
- Adding `--namespace` or `-n` flags to doctor subcommands
- Implementing namespace-based filtering in check execution logic
- Skipping namespaces during workload discovery

**Exception**: The kubeconfig context's namespace MAY be used for context display purposes only, but MUST NOT limit diagnostic scope.

**Rationale**: OpenShift AI is a cluster-wide platform with components, services, and workloads distributed across multiple namespaces. Comprehensive cluster health assessment requires visibility into all namespaces to detect misconfigurations, permission issues, and cross-namespace dependencies. Namespace filtering would create blind spots and incomplete diagnostics. A cluster administrator running diagnostics needs to see the full picture, not a partial view. This aligns with kubectl's cluster-scoped commands (e.g., `kubectl get nodes`, `kubectl get pv`) which don't support namespace filtering because they inherently operate cluster-wide.

### XI. Lint Command Architecture

The `kubectl odh lint` command provides cluster diagnostics supporting both current-state validation (linting) and upgrade-readiness assessment (when `--target-version` flag is provided).

**Command Structure**:
- Top-level `lint` command in `pkg/cmd/lint/`
- Lint command accepts optional `--target-version` flag to specify target version
- When `--target-version` is omitted or equals current version: validates current cluster state (lint mode)
- When `--target-version` differs from current version: assesses upgrade readiness to target version (upgrade mode)
- Checks detect their execution mode by comparing `target.CurrentVersion` with `target.Version`
- Same checks used for both modes; checks adapt behavior based on version context

**Package Structure**:
```
pkg/cmd/lint/
├── shared_options.go      # SharedOptions (IOStreams, ConfigFlags, output settings)
├── lint.go                # Command struct with Complete/Validate/Run/AddFlags
├── lint_options.go        # CommandOptions struct and functional options
└── lint_test.go           # Command tests
cmd/
└── lint.go                # Lint command registration (Cobra wrapper)
```

**Lint Command Behavior**:
```bash
# Lint current cluster state
kubectl odh lint

# Assess upgrade readiness to version 3.0
kubectl odh lint --target-version 3.0

# Same command, different context
# Checks adapt based on current vs target version comparison
```

**Check Implementation Pattern**:
```go
func (c *Check) Run(ctx context.Context, target *check.CheckTarget) check.CheckResult {
    // Checks detect mode by version comparison
    isLintMode := target.Version.Version == target.CurrentVersion.Version
    isUpgradeMode := target.Version.Version != target.CurrentVersion.Version

    if isUpgradeMode {
        // Validate upgrade-specific concerns (breaking changes, deprecations)
        return checkUpgradeReadiness(target)
    } else {
        // Validate current state (configuration, availability)
        return checkCurrentState(target)
    }
}
```

**Benefits**:
- Direct access: lint is promoted to top-level command for easier access
- Simpler mental model: lint validates state, `--target-version` changes context
- Clear flag naming: `--target-version` explicitly communicates purpose (vs ambiguous `--version`)
- Check code reuse: same checks handle both lint and upgrade scenarios
- Consistent output format across both modes
- Reduced typing overhead for most commonly used diagnostic functionality

**Prohibited**:
- Creating separate `upgrade` command (use `--target-version` flag on `lint` instead)
- Creating intermediate `doctor` parent command (lint is top-level)
- Using ambiguous `--version` flag (use explicit `--target-version` instead)
- Duplicate checks for lint vs upgrade (checks MUST adapt to version context)
- Hard-coding lint-only or upgrade-only logic (checks SHOULD be version-aware)

**Rationale**: Promoting lint to top-level simplifies the CLI surface and reduces typing for the primary diagnostic command. The `--target-version` flag name is more explicit than `--version`, making the command's intent clearer. Users understand "lint validates cluster state" as a single concept; the `--target-version` flag simply changes the validation context (current vs target). This approach aligns with kubectl's philosophy of direct command access and clear flag naming. Checks that adapt to version context are more maintainable than separate check implementations.

### XII. Kubernetes-Native Diagnostic CR Structure

Diagnostic check results MUST follow Kubernetes Custom Resource structure with metadata, spec, and status sections. This ensures consistency with Kubernetes CR conventions and enables familiar organization, granular condition reporting, and version tracking capabilities.

**CR Structure Requirements**:
- Diagnostic results MUST have a `Metadata` section with Group, Kind, Name, and Annotations
- Diagnostic results MUST have a `Spec` section containing the check description
- Diagnostic results MUST have a `Status` section containing a Conditions array
- Table rendering MUST display one row per condition for multi-condition checks

**Metadata Section**:
- `Group`: Categorizes the diagnostic target (e.g., "components", "services", "workloads")
- `Kind`: Identifies the specific target being checked (e.g., "kserve", "auth", "cert-manager")
- `Name`: Identifies the specific check being performed (e.g., "configuration-valid", "version-compatibility")
- `Annotations`: Key-value pairs for metadata (source/target versions, etc.)

**Group Field**:
- Categorizes diagnostic targets into logical groups
- Enables filtering and organizing diagnostics by category
- Examples: "components", "services", "infrastructure", "workloads"

**Kind Field**:
- Identifies the specific target being checked (component, service, etc.)
- Examples: "kserve", "auth", "cert-manager", "dashboard", "model-registry"
- Uses lowercase or component name conventions
- Represents the target entity within the group

**Name Field**:
- Identifies the specific diagnostic check being performed
- Uses descriptive kebab-case naming
- Examples: "component-status", "version-compatibility", "configuration-valid", "upgrade-readiness"
- Should clearly indicate what the check validates
- Must be unique within its group and kind

**Annotations**:
- Store version information using domain-qualified keys
- Source version annotation MUST use key: `check.opendatahub.io/source-version`
- Target version annotation MUST use key: `check.opendatahub.io/target-version`
- Additional annotations MAY be added for check-specific metadata
- Annotation keys MUST follow Kubernetes domain-qualified naming (domain/key format)

**Spec Section**:
- MUST contain a `Description` field explaining what the check validates
- Description MUST be detailed and user-friendly
- MUST clarify why the check matters (impact of failure)
- MUST be clear enough for operators to understand without reading source code

**Status Section**:
- MUST contain a `Conditions` array of metav1.Condition-alike structs
- Each condition represents a specific validation requirement
- Conditions enable granular reporting vs. single pass/fail result
- Each condition MUST include: Type, Status, Reason, Message, LastTransitionTime (following metav1.Condition pattern)

**Table Rendering**:
- When a diagnostic check has multiple conditions, table output MUST display one row per condition
- Each row MUST show Group, Kind, Name, and condition-specific details
- Single-condition checks display as one row; multi-condition checks expand to multiple rows
- This enables at-a-glance visibility of all validation requirements

**Example Diagnostic CR Structure**:
```go
type DiagnosticResult struct {
    // Metadata section (Kubernetes ObjectMeta/TypeMeta-alike)
    Metadata struct {
        Group       string            // "components"
        Kind        string            // "kserve"
        Name        string            // "configuration-valid"
        Annotations map[string]string // {"check.opendatahub.io/source-version": "2.15", ...}
    }

    // Spec section
    Spec struct {
        Description string // "Validates KServe component configuration and readiness for production use"
    }

    // Status section
    Status struct {
        Conditions []Condition // Array of validation conditions
    }
}

type Condition struct {
    Type               string    // Condition type (e.g., "ConfigurationValid", "ResourcesAvailable")
    Status             string    // "True", "False", "Unknown"
    Reason             string    // Machine-readable reason code
    Message            string    // Human-readable message
    LastTransitionTime time.Time // When condition last changed
}
```

**Rationale**: Kubernetes users are already familiar with CR structure (metadata/spec/status) and metav1.Condition patterns. Using this structure reduces cognitive load, enables intuitive filtering and organization, aligns with kubectl plugin integration (Principle I), and provides granular condition reporting for better troubleshooting. The Conditions array enables reporting multiple validation requirements per check instead of a single pass/fail, making diagnostics more actionable. Table expansion for multi-condition checks provides clear visibility without nested data structures. Domain-qualified annotation keys prevent naming conflicts and follow Kubernetes best practices. This structure enables future enhancements like diagnostic resource discovery, selective check execution by group/kind, condition-based filtering, and programmatic result processing that mirrors Kubernetes API patterns.

**Check Registration Pattern**:

All diagnostic checks MUST register themselves with the global registry using the init() function pattern. Command entrypoints MUST use blank imports to trigger check registration.

**Registration Requirements**:
- Each check implementation MUST call `registry.Instance().Register()` in its `init()` function
- Check packages MUST be imported in the command entrypoint file using blank imports (`import _ "package"`)
- Blank import statements MUST include explanatory comments documenting their purpose
- The global registry pattern ensures checks are automatically discovered without manual registration

**Example Check Registration**:
```go
// pkg/lint/checks/components/kserve/kserve.go
package kserve

import "github.com/lburgazzoli/odh-cli/pkg/lint/check"

func init() {
    registry.Instance().Register(&Check{})
}

type Check struct{}
// ... check implementation
```

**Example Command Entrypoint**:
```go
// cmd/lint/lint.go
package lint

import (
    // Import check packages to trigger init() auto-registration.
    // These blank imports are REQUIRED for checks to register with the global registry.
    // Do NOT remove these imports - they appear unused but are essential for runtime check discovery.
    _ "github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/kserve"
    _ "github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/modelmesh"
    _ "github.com/lburgazzoli/odh-cli/pkg/lint/checks/services/servicemesh"
    // ... other check packages
)
```

**Prohibited**:
- Manual check registration in command code (e.g., `registry.Register(kserve.NewCheck())`)
- Conditional registration based on runtime flags or environment
- Lazy registration patterns that delay check discovery

**Rationale**: The init() + blank import pattern provides automatic check discovery without manual registration boilerplate, ensures all checks are available at runtime, follows Go's plugin-like extension pattern, and prevents runtime errors from forgotten registration calls. Blank imports with explanatory comments prevent accidental removal by linters or developers who don't understand their purpose. This pattern is used by database drivers (database/sql), image formats (image), and other Go plugin systems.

**DiagnosticResult Validation Requirements**:

All DiagnosticResult instances MUST pass validation before being returned from checks. The validation MUST enforce strict structural requirements to ensure output consistency and prevent malformed diagnostics.

**Validation Rules**:

*Metadata Validation*:
- `Metadata.Group` MUST NOT be empty
- `Metadata.Kind` MUST NOT be empty
- `Metadata.Name` MUST NOT be empty
- `Metadata.Annotations` keys MUST follow domain-qualified format: `domain/key`
  - Domain MUST contain at least one dot (e.g., `opendatahub.io`, `example.com`)
  - Key MUST be non-empty
  - Examples: `check.opendatahub.io/source-version` (valid), `version` (invalid - no domain/key separator)

*Status Validation*:
- `Status.Conditions` array MUST contain at least one condition
- Empty conditions array is INVALID

*Condition Validation*:
- `Condition.Type` MUST NOT be empty
- `Condition.Status` MUST be one of: `"True"`, `"False"`, `"Unknown"` (matches metav1.ConditionStatus)
- `Condition.Reason` MUST NOT be empty

**Validation Error Messages**:
```go
const (
    errMsgGroupEmpty              = "metadata.group must not be empty"
    errMsgKindEmpty               = "metadata.kind must not be empty"
    errMsgNameEmpty               = "metadata.name must not be empty"
    errMsgConditionsEmpty         = "status.conditions must contain at least one condition"
    errMsgConditionTypeEmpty      = "condition with empty type found"
    errMsgConditionReasonEmpty    = "condition %q has empty reason"
    errMsgConditionInvalidStatus  = "condition %q has invalid status (must be True, False, or Unknown)"
    errMsgAnnotationInvalidFormat = "annotation key %q must be in domain/key format (e.g., openshiftai.io/version)"
)
```

**Example Validation Implementation**:
```go
func (r *DiagnosticResult) Validate() error {
    // Validate metadata
    if err := r.Metadata.Validate(); err != nil {
        return err
    }

    // Validate conditions
    if len(r.Status.Conditions) == 0 {
        return errors.New(errMsgConditionsEmpty)
    }

    // Validate each condition
    for i := range r.Status.Conditions {
        condition := &r.Status.Conditions[i]

        if condition.Type == "" {
            return errors.New(errMsgConditionTypeEmpty)
        }
        if condition.Status != metav1.ConditionTrue &&
           condition.Status != metav1.ConditionFalse &&
           condition.Status != metav1.ConditionUnknown {
            return fmt.Errorf(errMsgConditionInvalidStatus, condition.Type)
        }
        if condition.Reason == "" {
            return fmt.Errorf(errMsgConditionReasonEmpty, condition.Type)
        }
    }

    return nil
}
```

**Rationale**: Strict validation prevents malformed diagnostics from propagating through the system, ensures consistent output structure across all checks, catches developer errors at check execution time rather than during output rendering, and provides clear error messages for debugging. Annotation key validation enforces Kubernetes conventions and prevents naming conflicts. Message truncation (if needed) should be applied during table rendering for display purposes only, not during validation, to preserve full messages in JSON/YAML output.

## Development Standards

### Code Organization

Projects MUST follow the standard Go CLI structure:
- `cmd/` - Command definitions and entry points
- `pkg/` - Public packages (command logic, shared utilities)
- `internal/` - Internal packages not for external use

Commands MUST be organized as:
- `cmd/<command>.go` - Minimal Cobra wrapper (for top-level commands like lint)
- `cmd/<parent>/<command>.go` - For nested commands (if parent command exists)
- `pkg/cmd/<command>/<command>.go` - Command struct with Complete/Validate/Run/AddFlags
- `pkg/<command>/` - Domain-specific logic (optional)

### Function Signatures

Each parameter MUST have its own type declaration. Parameters MUST NOT be grouped even if they share the same type. Functions with many parameters MUST use multiline formatting.

**Rationale**: Explicit type declarations improve code clarity and prevent subtle bugs from parameter reordering.

### Naming Conventions

Use camelCase for unexported functions and variables. Use PascalCase for exported functions and types. Prefer descriptive names over abbreviations.

**Package Name Repetition**: Functions, types, and constants MUST NOT repeat the package name unless absolutely necessary for clarity. When code is organized in focused packages, the package name provides context, making repetition redundant and verbose.

**Constants**:
- **Good**: `kserve.checkID` (package provides context)
- **Bad**: `kserve.kserveServerlessRemovalCheckID` (redundant "kserve" prefix)

**Structs/Types**:
- **Good**: `dashboard.Check` (package-qualified: `dashboard.Check`)
- **Bad**: `dashboard.DashboardCheck` (redundant "Dashboard" prefix)
- **Good**: `kserve.ServerlessRemovalCheck` (package-qualified: `kserve.ServerlessRemovalCheck`)
- **Bad**: `kserve.KServeServerlessRemovalCheck` (redundant "KServe" prefix)
- **Good**: `modelmesh.RemovalCheck` (package-qualified: `modelmesh.RemovalCheck`)
- **Bad**: `modelmesh.ModelmeshRemovalCheck` (redundant "Modelmesh" prefix)

**Rationale**: Package-qualified names already include the package name (e.g., `kserve.checkID`), making additional prefixes redundant. This follows Go's philosophy of concise, readable code and aligns with standard library practices (e.g., `http.Client`, not `http.HTTPClient`).

### Code Comments

Comments MUST NOT state the obvious or describe what the code clearly does. Comments SHOULD explain WHY something is done, not WHAT is being done. Comments are REQUIRED only for:
- Non-obvious algorithmic choices or optimizations
- Workarounds for bugs or limitations in dependencies
- Complex business logic requiring domain knowledge
- Public APIs (package-level, exported functions/types per Go documentation conventions)
- Security-sensitive code sections
- Performance-critical sections

**Prohibited Comments** (obvious, redundant):
```go
// Get the DataScienceCluster singleton
dsc, err := target.Client.GetDataScienceCluster(ctx)

// Check if serviceMesh is enabled (Managed or Unmanaged)
if managementStateStr == "Managed" || managementStateStr == "Unmanaged" {
```

**Good Comments** (explain WHY or non-obvious context):
```go
// ServiceMesh is deprecated in 3.x but Unmanaged state must still be checked
// because users may have manually deployed service mesh operators
if managementStateStr == "Managed" || managementStateStr == "Unmanaged" {

// Parse versions once before loop to avoid duplicate parsing for each check
var currentVer, targetVer *semver.Version

// Workaround for https://github.com/kubernetes/kubernetes/issues/12345
// Direct field access fails for CRDs with structural schema validation
result, err := jq.Query(obj, ".spec.field")
```

**Exception**: godoc comments on exported identifiers are REQUIRED per Go conventions and are not considered "obvious" comments.

**Rationale**: Code should be self-documenting through clear naming and structure. Obvious comments create noise, become outdated as code changes, and reduce readability. The best comment is no comment when the code speaks for itself. When comments are needed, they should provide context that cannot be expressed in code alone—the reasoning, constraints, and trade-offs that led to the implementation.

### Commit Granularity

Each completed task from `specs/*/tasks.md` MUST result in exactly one commit, OR a group of strictly related tasks MAY be committed together when they are tightly coupled and cannot be reasonably separated. Commits MUST NOT bundle multiple unrelated tasks together. Each commit message MUST reference the task ID(s) in the format: `T###: <description>` for single tasks or `T###, T###: <description>` for grouped tasks (e.g., `T042: Implement workload resource limits check` or `T072, T073: Implement severity filtering and exit code flags`).

**Commit Message Format**:
- First line (single task): `T###: <imperative verb> <what>`
- First line (grouped tasks): `T###, T###: <imperative verb> <what>`
- Example: `T042: Implement workload resource limits check`
- Example: `T056: Add --version flag parsing for target version`
- Example: `T072, T073: Implement severity filtering and exit code flags`
- Body (optional): Additional context, breaking changes, migration notes
- Body (grouped tasks): SHOULD explain why tasks are grouped and their relationship

**Exceptions**:
- Automated changes (e.g., `make fmt`, `make tidy`) MAY be committed separately without task IDs
- Constitution amendments MUST use format: `docs: update constitution to v<version> - <summary>`
- Emergency hotfixes MAY omit task IDs if not planned in tasks.md

**Rationale**: Granular commits create a clear audit trail linking implementation to planned work. Each commit becomes a logical, reviewable, and revertable unit of work. This practice improves code review efficiency, simplifies debugging via git bisect, enables selective cherry-picking, and makes project history easier to understand. One commit per task enforces the discipline of completing tasks fully before moving on, preventing half-finished work from polluting the repository.

### Message Constants

User-facing messages MUST be defined as package-level constants or grouped in a dedicated constants file. Inline string literals and string concatenations for messages are PROHIBITED in business logic.

**Message Types Requiring Constants**:
- Remediation hints in diagnostic checks
- Error messages returned to users
- Help text and descriptions
- Validation error messages
- Success/failure status messages
- Warning and informational messages

**Constant Naming**:
- Use descriptive names in SCREAMING_SNAKE_CASE for exported constants
- Use camelCase with descriptive prefixes for unexported constants (e.g., `remediationInsufficientPermissions`)
- Group related messages in a `const` block or dedicated file (e.g., `messages.go`, `remediation.go`)

**String Formatting**:
- Use multi-line string literals (backticks) for long messages spanning multiple lines
- Use string concatenation with `+` for messages that need to be split for readability but are conceptually single-line
- Preserve formatting (newlines, indentation) in multi-line strings when appropriate for user display

**Allowed Inline Strings**:
- Dynamic values via `fmt.Sprintf()` using constant templates (e.g., `fmt.Sprintf(msgTemplateNotFound, resourceName)`)
- Log messages intended for debugging (not user-facing)
- Test assertions and test data

**Examples**:

**Good** (multi-line string literal):
```go
const (
    remediationInsufficientPermissions = `Insufficient permissions to access cluster resources.
Ensure your ServiceAccount or user has the required RBAC permissions.
Required permissions: get, list on the resource types being checked.
Contact your cluster administrator to grant access.`

    remediationTimeout = `Request timed out. Check network connectivity to the cluster API server.
Verify the cluster is responsive and not overloaded.`
)

// Usage
if apierrors.IsForbidden(err) {
    remediation = remediationInsufficientPermissions
}
```

**Good** (concatenation for single-line):
```go
const (
    remediationInsufficientPermissions = "Insufficient permissions to access cluster resources. " +
        "Ensure your ServiceAccount or user has the required RBAC permissions. " +
        "Required permissions: get, list on the resource types being checked. " +
        "Contact your cluster administrator to grant access."
)
```

**Bad** (inline):
```go
// PROHIBITED: inline string concatenation
if apierrors.IsForbidden(err) {
    remediation = "Insufficient permissions to access cluster resources. " +
        "Ensure your ServiceAccount or user has the required RBAC permissions. " +
        "Required permissions: get, list on the resource types being checked. " +
        "Contact your cluster administrator to grant access."
}
```

**Rationale**: Defining messages as constants enables message reuse across the codebase, ensures consistency in user-facing text, simplifies updates (change once, apply everywhere), makes messages testable and verifiable, facilitates future localization/internationalization, and makes code review easier by separating logic from text. Inline strings scattered throughout code are hard to find, maintain, and test.

### Mock Organization

Test mocks MUST be organized in reusable modules to prevent duplication across test files. Mocks MUST use the `github.com/stretchr/testify/mock` framework.

**Mock Location**:
- Reusable mocks MUST be placed in `pkg/util/test/mocks/<package>/` directory
- Example: `pkg/util/test/mocks/check/check.go` for mocking the `check.Check` interface
- Mock package name MUST be `mocks` (not `mocks_<package>`)

**Mock Requirements**:
- MUST use `testify/mock` framework (`github.com/stretchr/testify/mock`)
- MUST embed `mock.Mock` in mock structs
- MUST implement all interface methods with `m.Called(args...)` pattern
- MUST provide constructor functions (e.g., `NewMockCheck()`)
- MAY use `mockery` tool for automatic mock generation

**Prohibited**:
- Inline mock struct definitions in test files (except trivial test-specific cases)
- Duplicating mock implementations across multiple test files
- Hand-written mocks for complex interfaces when `mockery` can generate them

**Examples**:

**Good** (centralized mock):
```go
// pkg/util/test/mocks/check/check.go
package mocks

import "github.com/stretchr/testify/mock"

type MockCheck struct {
    mock.Mock
}

func NewMockCheck() *MockCheck {
    return &MockCheck{}
}

func (m *MockCheck) ID() string {
    args := m.Called()
    return args.String(0)
}

// ... other interface methods

// Usage in tests
import "github.com/lburgazzoli/odh-cli/pkg/util/test/mocks/check"

mockCheck := mocks.NewMockCheck()
mockCheck.On("ID").Return("test.check")
```

**Bad** (inline mock):
```go
// pkg/doctor/check/selector_test.go
// PROHIBITED: inline mock struct
type MockCheck struct {
    id       string
    category check.CheckCategory
}

func (m *MockCheck) ID() string { return m.id }
// ...
```

**Rationale**: Centralizing mocks in reusable modules eliminates code duplication, ensures consistency across tests, simplifies mock updates when interfaces change, and provides a single source of truth for test doubles. The testify/mock framework provides assertion capabilities (call verification, argument matching), reduces boilerplate, and is the Go community standard for mocking.

### Package Granularity

Packages MUST be fine-grained and organized by specific domain or functionality. Package names MUST be concise nouns representing their purpose. Overly broad packages with multiple unrelated responsibilities are PROHIBITED.

**Package Organization**:
- Prefer narrow, focused packages over large, multi-purpose packages
- Package name MUST reflect a single domain concept (e.g., `check`, `version`, `discovery`)
- Functions/types MUST be accessed as `package.Thing`, not `package.DoPackageThing`
- Avoid package names that are just collections of utilities (e.g., `utils`, `helpers`, `common`)

**Good Package Structure**:
```
pkg/lint/
    check/          # Check framework and execution (Check interface, CheckTarget)
        registry/   # Check registry management
    version/        # Version detection logic
    discovery/      # Resource discovery
    checks/
        components/ # Component-specific checks
            dashboard/       # Each check in its own package
                dashboard.go
                dashboard_test.go
            modelmesh/
                modelmesh.go
                modelmesh_test.go
        services/   # Service-specific checks
            servicemesh/
                servicemesh.go
                servicemesh_test.go
        shared/     # Shared check logic (if needed)
            validation/  # Clear domain-specific shared logic
```

**Diagnostic Check Package Isolation**:
- Each diagnostic check MUST be in its own dedicated package under `pkg/lint/checks/<category>/<check>/`
- Package name MUST match the check domain (e.g., `modelmesh`, `kserve`, `dashboard`)
- Check implementation MUST be in `<check>.go`, tests in `<check>_test.go`
- Shared check logic MUST be in a clearly named domain-specific package under `pkg/lint/checks/shared/`
- PROHIBITED: Multiple checks in the same package (e.g., `components/modelmesh.go` + `components/kserve.go`)

**Examples**:
- **Good**: `pkg/lint/checks/components/modelmesh/modelmesh.go` - Isolated check
- **Bad**: `pkg/lint/checks/components/modelmesh_removal.go` - Multiple checks in same package
- **Good**: `pkg/lint/checks/shared/validation/fields.go` - Shared validation logic
- **Bad**: `pkg/lint/checks/shared.go` - Unclear domain

**Naming Pattern**:
- **Good**: `version.Detect()` - package name reflects domain (version), function is action
- **Bad**: `lint.DetectVersion()` - package is too broad, function name duplicates package purpose
- **Good**: `discovery.DiscoverComponentsAndServices()` - clear domain separation
- **Bad**: `util.DiscoverComponentsAndServices()` - "util" is meaningless
- **Good**: `registry.Add()`, `registry.Get()`, `registry.List()`, `registry.Instance()` - focused registry package
- **Bad**: `check.RegisterCheck()`, `check.GetCheck()` - registry concerns mixed into check package

**When to Split Packages**:
- Package has multiple unrelated types or functions
- Package name requires qualifiers like "and" or multiple concepts (e.g., `foobar` for unrelated foo and bar)
- Import cycles occur due to overly broad packages
- Package exceeds ~1000 lines of code (excluding tests and generated code)

**Rationale**: Fine-grained packages improve code discoverability, reduce coupling, prevent import cycles, make dependencies explicit, and align with Go's philosophy of small, focused packages. Package names as domain concepts (not actions) make code read naturally (e.g., `check.Execute()` reads as "execute check", not "check execute check"). This pattern is standard in well-designed Go projects like Kubernetes, Docker, and Prometheus.

###IOStreams Wrapper

Commands MUST use an IOStreams wrapper utility to eliminate repetitive formatting boilerplate. The wrapper MUST provide convenience methods that automatically format and write output. Direct use of `fmt.Fprintf(o.Out, ...)` for command output is DISCOURAGED when the wrapper provides equivalent functionality.

**IOStreams Wrapper Requirements**:
- Wrapper MUST be implemented in `pkg/util/iostreams/`
- Wrapper MUST provide methods: `Fprintf(format string, args ...any)`, `Fprintln(args ...any)`, `Errorf(format string, args ...any)`, `Errorln(args ...any)`
- Methods MUST automatically select the correct output stream (Out vs ErrOut)
- Formatting methods (Fprintf, Errorf) MUST use `fmt.Sprintf` when args are provided
- Non-formatting methods (Fprintln, Errorln) MUST use `fmt.Fprintln` directly
- SharedOptions SHOULD embed or contain an instance of the IOStreams wrapper

**Example Implementation**:
```go
// pkg/util/iostreams/iostreams.go
package iostreams

import (
    "fmt"
    "io"
)

type IOStreams struct {
    In     io.Reader
    Out    io.Writer
    ErrOut io.Writer
}

// Fprintf writes formatted output to Out
func (s *IOStreams) Fprintf(format string, args ...any) {
    if len(args) > 0 {
        _, _ = fmt.Fprintln(s.Out, fmt.Sprintf(format, args...))
    } else {
        _, _ = fmt.Fprintln(s.Out, format)
    }
}

// Fprintln writes output to Out with newline
func (s *IOStreams) Fprintln(args ...any) {
    _, _ = fmt.Fprintln(s.Out, args...)
}

// Errorf writes formatted error output to ErrOut
func (s *IOStreams) Errorf(format string, args ...any) {
    if len(args) > 0 {
        _, _ = fmt.Fprintln(s.ErrOut, fmt.Sprintf(format, args...))
    } else {
        _, _ = fmt.Fprintln(s.ErrOut, format)
    }
}

// Errorln writes error output to ErrOut with newline
func (s *IOStreams) Errorln(args ...any) {
    _, _ = fmt.Fprintln(s.ErrOut, args...)
}
```

**Usage**:
```go
// Before (repetitive)
_, _ = fmt.Fprintf(o.Out, "Detected version: %s\n", version)
_, _ = fmt.Fprintf(o.ErrOut, "Error: %v\n", err)

// After (clean)
o.io.Fprintf("Detected version: %s", version)
o.io.Errorf("Error: %v", err)
```

**Rationale**: Repetitive `fmt.Fprintf(o.Out, ...)` patterns create visual noise and make code harder to read. An IOStreams wrapper centralizes output handling, reduces boilerplate, automatically manages newlines, and provides a cleaner API for command output. This pattern aligns with DRY principles and improves code maintainability. The automatic selection of output streams (Out vs ErrOut) prevents mistakes where errors are written to stdout instead of stderr.

### Deprecated API Avoidance

Code MUST NOT use deprecated methods, functions, or packages unless no non-deprecated alternative exists. When using deprecated APIs is unavoidable, a comment MUST explain why and reference the tracking issue or upstream bug preventing migration.

**Deprecation Detection**:
- Use IDE deprecation warnings (GoLand, VSCode with gopls)
- Check godoc comments for `// Deprecated:` markers
- Review dependency changelogs when upgrading

**When Deprecated API is Unavoidable**:
- Add a comment explaining why the deprecated API must be used
- Reference the upstream issue or limitation preventing migration
- Create a TODO with a tracking issue for future migration
- Document the required conditions for migrating away from the deprecated API

**Examples**:

**Good** (avoids deprecated API):
```go
// Use current API instead of deprecated NestedString
value, err := jq.Query(obj, ".spec.field")
```

**Acceptable** (deprecated but unavoidable):
```go
// TODO(issue-123): Migrate to client.List() when streaming support is added
// Deprecated client.ListWatch() required because client.List() lacks streaming
resources, err := client.ListWatch(ctx, gvr)
```

**Bad** (deprecated without justification):
```go
// PROHIBITED: using deprecated API without explanation
value, _ := unstructured.NestedString(obj.Object, "spec", "field")
```

**Rationale**: Deprecated APIs exist for a reason—they have known issues, security vulnerabilities, or better alternatives. Using deprecated code increases technical debt, creates maintenance burden, and may break in future releases. Avoiding deprecated APIs ensures the codebase stays modern, secure, and maintainable. When deprecated APIs are unavoidable, documenting the reason and creating a migration plan ensures the debt is tracked and addressed systematically.

## Development Workflow

### Lint-Fix-First

Before manually resolving linting issues, you MUST run `make lint-fix` to automatically fix trivial issues. Manual fixes SHOULD only be applied to issues that cannot be auto-fixed.

**Workflow Requirements**:
- ALWAYS run `make lint-fix` (or equivalent) as the first step when addressing linting failures
- Run `make lint` after auto-fix to identify remaining issues
- Manually fix only the issues that auto-fix could not resolve
- Commit auto-fixed changes separately from manual fixes when appropriate

**Auto-Fixable Issues** (handled by lint-fix):
- Import organization (gci)
- Code formatting (gofmt, gofumpt)
- Unused imports
- Simplifiable code patterns (gosimple)
- Some inefficient assignments (ineffassign)

**Manual-Fix-Required Issues**:
- Logic errors
- Security vulnerabilities
- Complex code smells
- API usage violations
- Custom linter failures

**Example Workflow**:
```bash
# Step 1: Run tests and discover linting failures
make check
# Output: 47 linting issues found

# Step 2: Auto-fix trivial issues FIRST
make lint-fix
# Output: Fixed 42 issues automatically

# Step 3: Check remaining issues
make lint
# Output: 5 issues remaining

# Step 4: Manually fix remaining 5 issues
# ... edit code ...

# Step 5: Verify all issues resolved
make check
# Output: All checks passed
```

**Rationale**: Auto-fixers resolve trivial formatting and import issues instantly, saving developer time and mental effort. Running lint-fix first prevents wasted effort manually fixing issues that could be automated. This workflow enforces consistent code style, reduces code review noise, and allows developers to focus on substantive issues that require human judgment. The separation of auto-fixed and manual changes improves commit clarity and makes code review more efficient.

## Quality Gates

### Continuous Quality Verification

After EVERY implementation (task, feature, or fix), you MUST run `make check` and fix all issues before proceeding. This is NOT optional and applies to ALL code changes, including:
- New features
- Bug fixes
- Refactoring
- Test additions
- Documentation updates that include code examples

**Rationale**: Catching linting and security issues immediately prevents technical debt accumulation and ensures the codebase remains consistently high quality. Running checks only at PR time leads to batch fixing and makes it harder to identify which change introduced issues.

### Linting

All code MUST pass `make lint` using golangci-lint v2 with the project's `.golangci.yml` configuration. All linters are enabled by default except: wsl, varnamelen, exhaustruct, ireturn, depguard, err113, paralleltest, funcorder, noinlineerr.

### Vulnerability Scanning

All code MUST pass `make vulncheck` using govulncheck to detect known security vulnerabilities in dependencies and code.

### Testing

All code MUST pass `make test`. New features MUST include both unit and integration tests. Test coverage SHOULD increase or remain stable with new code.

### Formatting

All code MUST be formatted with `make fmt`. Imports MUST be organized using `gci` in sections: standard, default, k8s.io, project, dot.

### Dependencies

Dependencies MUST be kept tidy via `make tidy`. New dependencies MUST pass `make vulncheck` security scanning.

## Governance

This constitution supersedes all other development practices. All pull requests MUST be reviewed for constitutional compliance. Amendments require documentation of rationale, approval from maintainers, and a migration plan if breaking changes are introduced.

Constitutional violations MUST be justified in the implementation plan's Complexity Tracking table, documenting why the simpler alternative was insufficient.

**Constitution Check Gates**:
- Phase 0 (Research): Verify approach aligns with kubectl plugin integration, output format consistency, high-level resource check targets (Principle IX - no low-level Kubernetes primitives), and cluster-wide diagnostic scope (Principle X - no namespace filtering)
- Phase 1 (Design): Verify command structure follows Complete/Validate/Run pattern, functional options, and fine-grained package organization (focused packages with concise domain names, avoid package bloat)
- Phase 2 (Implementation): Verify error handling, test coverage (fake client + k3s-envtest), testify/mock for mocking (mocks in pkg/util/test/mocks), JQ-based field access for unstructured objects, centralized GVK/GVR definitions in pkg/resources/types.go, Kubernetes-native diagnostic CR structure (Principle XII - Metadata with Group/Kind/Name/Annotations, Spec with Description, Status with Conditions array, table rendering one row per condition), check registration pattern (init() functions + blank imports with explanatory comments), DiagnosticResult validation compliance (required fields, annotation key format, empty conditions array check), user-facing messages defined as package-level constants (no inline strings), `make check` execution after each implementation, full linting compliance, and one commit per completed task with task ID in commit message

**Version**: 1.20.1 | **Ratified**: 2025-12-05 | **Last Amended**: 2025-12-12