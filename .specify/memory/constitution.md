<!--
============================================================================
SYNC IMPACT REPORT
============================================================================
Version Change: 1.13.0 → 1.14.0
Modified Principles: None
Added Sections:
  - Development Standards: Code Comments
    - Comments MUST NOT state obvious facts
    - Comments SHOULD explain WHY, not WHAT
    - Required only for non-obvious choices, workarounds, complex logic, public APIs
    - Includes examples of prohibited vs good comments
Modified Sections:
  - Principle VI: Test-First Development: Gomega Assertions
    - Expanded guidance to explicitly prohibit individual field assertions
    - Added MatchFields example for multiple field validation
    - Added clear examples of bad vs good struct testing patterns
Removed Sections: None
Templates Requiring Updates:
  ✅ .specify/templates/plan-template.md - Generic template, gates auto-populate from constitution
  ✅ .specify/templates/spec-template.md - Generic template, intentionally not constitution-specific
  ✅ .specify/templates/tasks-template.md - Generic template, intentionally not constitution-specific
  ✅ .specify/templates/agent-file-template.md - Generic template, intentionally not constitution-specific
  ✅ .specify/templates/checklist-template.md - Generic template, intentionally not constitution-specific
Follow-up TODOs:
  ⚠ Refactor existing checks to follow package isolation pattern:
    - Move pkg/doctor/checks/components/dashboard.go to pkg/doctor/checks/components/dashboard/dashboard.go
    - Move pkg/doctor/checks/components/modelmesh_removal.go to pkg/doctor/checks/components/modelmesh/modelmesh.go
    - Move pkg/doctor/checks/components/kserve_serverless_removal.go to pkg/doctor/checks/components/kserve/kserve.go
    - Update all test files accordingly
    - Update imports across codebase
  ⚠ Move MockCheck from selector_test.go to pkg/util/test/mocks/check.go
  ⚠ Refactor inline mocks to use testify/mock
  ⚠ Refactor pkg/cmd/doctor/ structure per Principle XI

Rationale for MINOR bump (1.11.0 → 1.12.0):
- New package organization guidance added (Diagnostic Check Package Isolation)
- Existing checks violate new guidance (requires refactoring)
- Backward compatible at runtime (no breaking changes to CLI)
- Improves code organization and maintainability
- Aligns with Package Granularity principle (focused packages)
- Prevents check package bloat as more checks are added
- Makes check dependencies and boundaries clearer
============================================================================
-->

# odh-cli Constitution

## Core Principles

### I. kubectl Plugin Integration

The CLI MUST function as a native kubectl plugin following kubectl UX patterns. The binary MUST be named `kubectl-odh` and automatically discovered when placed in PATH. The CLI MUST leverage the user's active kubeconfig for cluster authentication without requiring separate configuration.

**Rationale**: Users interacting with Kubernetes expect kubectl-like tools. Following kubectl conventions reduces cognitive load and provides a familiar, consistent experience.

### II. Extensible Command Structure

All commands MUST follow the modular Cobra-based pattern separating command definition (cmd/) from business logic (pkg/cmd/). New commands MUST be independently testable without Cobra dependencies. Each command MUST implement the Complete/Validate/Run pattern for consistent lifecycle management.

**Rationale**: Separation of concerns enables independent testing, code reuse, and maintains a consistent structure as the CLI grows. This pattern is standard in kubectl plugins and kubectl itself.

### III. Consistent Output Formats

All commands MUST support table (default), JSON, and YAML output formats via the `-o/--output` flag. Table output MUST be human-readable with consistent formatting. JSON and YAML output MUST be machine-parsable and suitable for scripting.

**Rationale**: Different consumers need different formats. Humans need readable tables, scripts need structured JSON/YAML. Consistency across commands reduces learning curve and enables composition.

### IV. Functional Options Pattern

All struct initialization MUST use the functional options pattern with the generic `Option[T]` interface. Configuration MUST be applied via `ApplyTo(target *T)` method. Options MUST be defined in `*_options.go` or `*_option.go` files.

**Rationale**: Provides type-safe, extensible, and composable configuration. This pattern is used in k8s-controller-lib and enables backward-compatible API evolution.

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

### XI. Command Package Isolation

Each command MUST reside in its own dedicated package under `pkg/cmd/<parent>/<command>`. Commands sharing a parent MUST NOT share the same package. Business logic and option types MUST be isolated per command to enable independent development, testing, and maintenance.

**Package Structure Requirements**:
- Command business logic MUST be in `pkg/cmd/<parent>/<command>/` (e.g., `pkg/cmd/doctor/lint/`, `pkg/cmd/doctor/upgrade/`)
- Each command package MUST contain its own `options.go` file with the command's Options struct
- Shared code between sibling commands MUST be factored into a parent-level package `pkg/cmd/<parent>/shared.go`
- Cobra wrappers (command registration) MUST be in `cmd/<parent>/<command>.go` (e.g., `cmd/doctor/lint.go`, `cmd/doctor/upgrade.go`)
- Multiple Cobra wrappers MAY coexist in `cmd/<parent>/` directory, but each MUST delegate to its own `pkg/cmd/<parent>/<command>/` package

**Example Structure**:
```
pkg/cmd/doctor/
├── shared.go              # Shared options, types, utilities
├── lint/
│   ├── options.go         # LintOptions struct
│   ├── run.go             # Lint command business logic
│   └── options_test.go    # Lint tests
└── upgrade/
    ├── options.go         # UpgradeOptions struct
    ├── run.go             # Upgrade command business logic
    └── options_test.go    # Upgrade tests
```

**Benefits**:
- Clear separation of concerns between commands
- Independent versioning and evolution of command logic
- Reduced merge conflicts when multiple developers work on different commands
- Easier testing with isolated dependencies
- Simpler code navigation and discoverability

**Prohibited**:
- Placing multiple command implementations in a single package (e.g., `pkg/cmd/doctor/` containing both lint and upgrade)
- Mixing command-specific logic in shared utility packages
- Circular dependencies between sibling command packages

**Rationale**: As CLI tools grow, command implementations become complex with unique options, validation logic, and execution flows. Isolating each command in its own package prevents tight coupling, enables parallel development, simplifies testing, and makes the codebase more maintainable. This pattern is used by kubectl, helm, and other mature CLI tools where each subcommand is independently developed. Package-level isolation enforces architectural boundaries and prevents accidental dependencies between commands.

## Development Standards

### Code Organization

Projects MUST follow the standard Go CLI structure:
- `cmd/` - Command definitions and entry points
- `pkg/` - Public packages (command logic, shared utilities)
- `internal/` - Internal packages not for external use

Commands MUST be organized as:
- `cmd/<command>/<command>.go` - Minimal Cobra wrapper
- `pkg/cmd/<command>/<command>.go` - Options struct with Complete/Validate/Run
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
pkg/doctor/
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
- Each diagnostic check MUST be in its own dedicated package under `pkg/doctor/checks/<category>/<check>/`
- Package name MUST match the check domain (e.g., `modelmesh`, `kserve`, `dashboard`)
- Check implementation MUST be in `<check>.go`, tests in `<check>_test.go`
- Shared check logic MUST be in a clearly named domain-specific package under `pkg/doctor/checks/shared/`
- PROHIBITED: Multiple checks in the same package (e.g., `components/modelmesh.go` + `components/kserve.go`)

**Examples**:
- **Good**: `pkg/doctor/checks/components/modelmesh/modelmesh.go` - Isolated check
- **Bad**: `pkg/doctor/checks/components/modelmesh_removal.go` - Multiple checks in same package
- **Good**: `pkg/doctor/checks/shared/validation/fields.go` - Shared validation logic
- **Bad**: `pkg/doctor/checks/shared.go` - Unclear domain

**Naming Pattern**:
- **Good**: `version.Detect()` - package name reflects domain (version), function is action
- **Bad**: `doctor.DetectVersion()` - package is too broad, function name duplicates package purpose
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
- Phase 2 (Implementation): Verify error handling, test coverage (fake client + k3s-envtest), testify/mock for mocking (mocks in pkg/util/test/mocks), JQ-based field access for unstructured objects, centralized GVK/GVR definitions in pkg/resources/types.go, user-facing messages defined as package-level constants (no inline strings), `make check` execution after each implementation, full linting compliance, and one commit per completed task with task ID in commit message

**Version**: 1.14.0 | **Ratified**: 2025-12-05 | **Last Amended**: 2025-12-07