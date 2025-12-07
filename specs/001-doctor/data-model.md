# Data Model: Doctor Subcommand

**Feature**: 001-doctor | **Date**: 2025-12-06 | **Phase**: 1

## Overview

This document defines the core entities and their relationships for the doctor subcommand diagnostic system. The model supports pluggable check registration, version-aware validation, and structured diagnostic reporting with actionable remediation guidance.

---

## Entity Definitions

### Check

Represents an individual diagnostic test that validates a specific aspect of cluster configuration.

**Attributes:**
- `ID` (string): Unique identifier for the check (e.g., "dashboard-deployment-exists")
- `Name` (string): Human-readable check name
- `Description` (string): What the check validates
- `Category` (enum): Check category - Component, Service, or Workload
- `ValidateFunc` (function): Validation logic implementation
- `RemediationHint` (string): Actionable guidance for addressing failures
- `ApplicableVersions` (string): Semver constraint for applicable versions (e.g., ">=2.0.0", ">=3.0.0", "*")

**Relationships:**
- Belongs to exactly one CheckCategory
- Registered in CheckRegistry
- Execution produces DiagnosticResult

**Validation Rules:**
- ID must be unique within registry
- ValidateFunc must not be nil
- Category must be one of: Component, Service, Workload
- ApplicableVersions must be valid semver constraint string
- ApplicableVersions must not be empty

**State Transitions:**
N/A (immutable after registration)

**Implementation Notes:**
```go
type Check struct {
    ID                 string
    Name               string
    Description        string
    Category           CheckCategory
    ValidateFunc       ValidationFunc
    RemediationHint    string
    ApplicableVersions string // semver constraint
}

type ValidationFunc func(ctx context.Context, target *CheckTarget) (*DiagnosticResult, error)

type CheckCategory string

const (
    CategoryComponent CheckCategory = "component"
    CategoryService   CheckCategory = "service"
    CategoryWorkload  CheckCategory = "workload"
)
```

---

### CheckRegistry

Collection of available checks organized by category and version, enabling dynamic check registration and selective execution.

**Attributes:**
- `Checks` (map[string]Check): All registered checks indexed by ID
- `Version` (ClusterVersion): Associated cluster version
- `Categories` (map[CheckCategory][]Check): Checks grouped by category

**Relationships:**
- Contains many Checks
- Associated with exactly one ClusterVersion

**Operations:**
- `Register(check Check) error`: Add check to registry
- `Get(id string) (Check, bool)`: Retrieve check by ID
- `ListByCategory(category CheckCategory) []Check`: Get all checks for category
- `ListBySelector(selector string) []Check`: Get checks matching selector pattern
- `ExecuteAll(ctx context.Context) []DiagnosticResult`: Run all registered checks
- `ExecuteSelective(ctx context.Context, selector string) []DiagnosticResult`: Run subset of checks

**Validation Rules:**
- Check IDs must be unique across registry
- Cannot register checks after execution begins
- Selector patterns must be valid (category name, check ID, or glob pattern)

**State Transitions:**
1. **Initialization**: Empty registry created with version
2. **Registration**: Checks added via Register()
3. **Execution**: Checks run via ExecuteAll() or ExecuteSelective()

**Implementation Notes:**
```go
type CheckRegistry struct {
    checks     map[string]Check
    version    ClusterVersion
    categories map[CheckCategory][]Check
}

func NewRegistry(version ClusterVersion) (*CheckRegistry, error)
func (r *CheckRegistry) Register(check Check) error
func (r *CheckRegistry) ExecuteSelective(ctx context.Context, selector string) ([]DiagnosticResult, error)
```

---

### ClusterVersion

Represents the detected OpenShift AI version with source tracking and branch mapping.

**Attributes:**
- `Version` (string): Semantic version string (e.g., "2.10.0", "3.1.0")
- `MajorVersion` (string): Major version for config lookup (e.g., "2.x", "3.x")
- `Source` (VersionSource): How version was detected - DataScienceCluster, DSCInitialization, or OLM
- `Branch` (string): Operator repository branch (e.g., "stable-2.x", "main")
- `DetectedAt` (time.Time): When version was detected
- `Confidence` (enum): Detection confidence - High (from DSC), Medium (from DSCInit), Low (from OLM)

**Relationships:**
- Referenced by CheckRegistry
- Used by ConfigurationBundle loader

**Validation Rules:**
- Version must follow semantic versioning (major.minor.patch)
- Source must be one of: DataScienceCluster, DSCInitialization, OLM
- Branch mapping: 2.x → "stable-2.x", 3.x → "main"

**State Transitions:**
N/A (immutable after detection)

**Implementation Notes:**
```go
type ClusterVersion struct {
    Version      string
    MajorVersion string
    Source       VersionSource
    Branch       string
    DetectedAt   time.Time
    Confidence   VersionConfidence
}

type VersionSource string

const (
    SourceDataScienceCluster VersionSource = "DataScienceCluster"
    SourceDSCInitialization  VersionSource = "DSCInitialization"
    SourceOLM                VersionSource = "OLM"
)

type VersionConfidence string

const (
    ConfidenceHigh   VersionConfidence = "high"
    ConfidenceMedium VersionConfidence = "medium"
    ConfidenceLow    VersionConfidence = "low"
)
```

---

### DiagnosticResult

Represents the outcome of a check execution with status, findings, and remediation guidance.

**Attributes:**
- `CheckID` (string): ID of executed check
- `CheckName` (string): Human-readable check name
- `Status` (ResultStatus): Execution outcome - Pass, Fail, Error, or Skipped
- `Severity` (Severity): Finding severity - Critical, Warning, Info (only if Status=Fail)
- `Message` (string): Description of finding or error
- `AffectedResources` ([]ResourceReference): Resources involved in finding
- `RemediationHint` (string): Actionable guidance for addressing issue
- `ExecutionTime` (time.Duration): How long check took to run
- `Timestamp` (time.Time): When check was executed

**Relationships:**
- Produced by Check execution
- References AffectedResources

**Validation Rules:**
- CheckID must correspond to registered check
- If Status=Fail, Severity must be set
- If Status=Pass or Skipped, Severity should be nil
- Message must not be empty for Fail or Error status

**State Transitions:**
N/A (immutable after creation)

**Implementation Notes:**
```go
type DiagnosticResult struct {
    CheckID           string
    CheckName         string
    Status            ResultStatus
    Severity          *Severity  // nil for Pass/Skipped
    Message           string
    AffectedResources []ResourceReference
    RemediationHint   string
    ExecutionTime     time.Duration
    Timestamp         time.Time
}

type ResultStatus string

const (
    StatusPass    ResultStatus = "pass"
    StatusFail    ResultStatus = "fail"
    StatusError   ResultStatus = "error"
    StatusSkipped ResultStatus = "skipped"
)

type ResourceReference struct {
    APIVersion string
    Kind       string
    Name       string
    Namespace  string
}
```

---

### Severity

Classification of diagnostic findings indicating urgency and impact.

**Attributes:**
- `Level` (enum): Critical, Warning, or Info

**Definitions:**
- **Critical**: Blocking issues requiring immediate action (e.g., component not running, version incompatibility)
- **Warning**: Non-blocking problems needing attention (e.g., deprecated configuration, suboptimal settings)
- **Info**: Optimization suggestions and best practices (e.g., resource limits not set, unused components enabled)

**Relationships:**
- Used by DiagnosticResult (set when Status=Fail based on the specific finding)

**Validation Rules:**
- Level must be one of: Critical, Warning, Info

**Implementation Notes:**
```go
type Severity string

const (
    SeverityCritical Severity = "Critical"
    SeverityWarning  Severity = "Warning"
    SeverityInfo     Severity = "Info"
)

func (s Severity) Validate() error {
    switch s {
    case SeverityCritical, SeverityWarning, SeverityInfo:
        return nil
    default:
        return fmt.Errorf("invalid severity: %s", s)
    }
}
```

---

---

### CheckTarget

Context and resources passed to check validation functions.

**Attributes:**
- `Client` (*client.Client): Unified Kubernetes client (dynamic, discovery, apiextensions)
- `Version` (ClusterVersion): Detected cluster version
- `Resource` (*unstructured.Unstructured): Specific resource being checked (for workload checks, nil for component/service checks)

**Relationships:**
- Passed to Check.ValidateFunc
- References Client, ClusterVersion

**Usage Patterns:**
- **Component/Service checks**: Resource is nil, check implementation queries necessary resources
- **Workload checks**: Resource contains the specific workload instance being validated, namespace available via `Resource.GetNamespace()`

**Implementation Notes:**
```go
type CheckTarget struct {
    Client   *client.Client
    Version  ClusterVersion
    Resource *unstructured.Unstructured // nil for component/service checks
}
```

---

## Entity Relationships Diagram

```
┌─────────────────┐
│ ClusterVersion  │
└────────┬────────┘
         │
         │ references
         ▼
┌──────────────────┐      contains      ┌───────────┐
│  CheckRegistry   │──────────────────►│   Check   │
└────────┬─────────┘                    └─────┬─────┘
         │                                    │
         │ executes                           │ validates
         ▼                                    ▼
┌──────────────────┐                  ┌──────────────┐
│ DiagnosticResult │                  │ CheckTarget  │
└────────┬─────────┘                  └──────────────┘
         │
         │ references
         ▼
┌──────────────────────┐
│ AffectedResources    │
└──────────────────────┘

Workload Discovery Flow:
┌──────────────────┐   discovers CRDs   ┌──────────────────┐
│ APIExtensions    │───────────────────►│ []GVR (workload  │
│ Client           │                     │  types found)    │
└──────────────────┘                     └────────┬─────────┘
                                                  │
                                                  │ lists instances
                                                  ▼
                                          ┌──────────────────┐
                                          │ []unstructured   │
                                          │ .Unstructured    │
                                          └──────────────────┘
```

---

## Data Flow

### Lint Command Flow

1. **Version Detection**: Detect ClusterVersion from DataScienceCluster → DSCInitialization → OLM
2. **Registry Initialization**: Create CheckRegistry with detected version
3. **Check Registration**: Register all checks implemented in Go code (components, services, workloads)
4. **Resource Discovery**:
   - Components: Discovery client for API group `components.platform.opendatahub.io`
   - Services: Discovery client for API group `services.platform.opendatahub.io`
   - Workloads: CRD discovery with label `platform.opendatahub.io/part-of`
5. **Check Execution**:
   - **Component/Service checks**: Execute with CheckTarget{Client, Version, Resource: nil}
   - **Workload checks**: For each discovered workload CRD:
     - List all instances of the CRD across namespaces
     - For each instance, execute check with CheckTarget{Client, Version, Resource: instance}
6. **Result Collection**: Collect DiagnosticResults with status, severity, remediation
7. **Output Formatting**: Format results as table, JSON, or YAML

### Upgrade Command Flow

1. **Current Version Detection**: Same as lint
2. **Target Version Parsing**: Parse `--version` flag for target version
3. **Registry Initialization**: Create CheckRegistry with target version
4. **Compatibility Check Registration**: Register upgrade-specific checks (implemented in Go)
5. **Check Execution**: Execute upgrade readiness checks
6. **Result Collection**: Collect DiagnosticResults for upgrade blockers
7. **Output Formatting**: Format upgrade assessment

---

## Persistence and Caching

**No Persistent Storage:**
- Doctor command is read-only, performs no writes to cluster or filesystem
- All data is ephemeral within command execution

**In-Memory Caching:**
- CheckRegistry caches registered checks for command duration
- RESTMapper caches GVK→GVR mappings (10-minute TTL via client-go default)
- WorkloadCRD discovery results cached for command execution

---

## Error Handling

**Check Execution Errors:**
- Network errors (cluster unreachable): Propagate as Error status in DiagnosticResult
- Permission errors: Return Error status with remediation hint about RBAC
- Resource not found: May be Warning or Info depending on check expectation
- Invalid configurations: Return Fail status with Critical/Warning severity

**Version Detection Errors:**
- All sources fail: Return error, cannot proceed without version
- Partial detection: Use best available source, log confidence level

**CRD Discovery Errors:**
- No CRDs with label: Valid scenario, Info-level finding
- CRD in error state: Warning-level finding with status condition details
- Permission denied: Critical-level finding about insufficient access

---

## Extension Points

**Adding New Checks:**
1. Implement check in `pkg/doctor/checks/{category}/{check-name}.go`
2. Implement Check interface (ID, Name, Description, Category, Severity, Validate, etc.)
3. Register check in init() function
4. Write unit and integration tests
5. Test with `go run cmd/main.go doctor lint`

**Adding New Components/Services/Workloads:**
1. Ensure CRD is properly categorized:
   - Components: API group `components.platform.opendatahub.io`
   - Services: API group `services.platform.opendatahub.io`
   - Workloads: Label `platform.opendatahub.io/part-of`
2. Dynamic discovery automatically detects new CRD
3. Implement category-specific check in `pkg/doctor/checks/{category}/`
4. Register check in checks initialization (or make check generic for all CRDs in category)

**Adding New Versions:**
1. Update version detection logic to recognize new version pattern
2. Implement version-specific checks if needed (check applicable versions)
3. Test with `go run cmd/main.go doctor lint`

---

## Compliance with Constitutional Principles

- **Principle II (Extensible Command Structure)**: Check interface enables independent testing without Cobra dependencies
- **Principle IV (Functional Options Pattern)**: CheckRegistry, ConfigurationBundle use Option[T] pattern
- **Principle V (Error Handling)**: All errors wrapped with `fmt.Errorf("%w")`, context propagated
- **Principle VII (JQ-Based Field Access)**: CheckTarget uses unstructured objects, all field access via `pkg/util/jq.Query()`
- **Principle VIII (Centralized GVK/GVR)**: All resource types defined in `pkg/resources/types.go`, accessed via `resources.<Type>.GVR()`
