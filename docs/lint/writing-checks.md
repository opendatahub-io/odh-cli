# Writing Lint Checks

**Scope:** This guide explains how to write diagnostic checks for the **lint command**.

The lint command (`kubectl odh lint`) validates OpenShift AI cluster configuration using a check framework. This guide covers how to implement checks specifically for the lint command.

For lint architecture, see [architecture.md](architecture.md). For general development practices, see [../development.md](../development.md).

## Check Interface

All lint checks implement the `Check` interface:

```go
type Check interface {
    ID() string
    Name() string
    Description() string
    Group() CheckGroup
    CheckKind() string
    CheckType() string
    CanApply(ctx context.Context, target Target) bool
    Validate(ctx context.Context, target Target) (*result.DiagnosticResult, error)
}
```

**Key differences from typical interfaces:**
- `Group()` returns `CheckGroup` type (not string)
- `CheckKind()` returns the kind of resource being checked (e.g., "kserve", "codeflare"). Used by validation builders to construct diagnostic results
- `CheckType()` returns the type of check (e.g., "removal", "deprecation"). Used by validation builders to construct diagnostic results
- `CanApply()` takes context and target
- `Validate()` returns `(*result.DiagnosticResult, error)` - error for infrastructure failures

### Implementing a Lint Check

Create a new package under `pkg/lint/checks/<category>/<checkname>/`:

```go
// pkg/lint/checks/components/dashboard/dashboard.go
package dashboard

import (
    "context"

    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
)

type Check struct {
    base.BaseCheck
}

func NewCheck() *Check {
    return &Check{
        BaseCheck: base.BaseCheck{
            CheckGroup:       check.GroupComponent,
            Kind:             check.ComponentDashboard,
            Type:             check.CheckTypeInstalled,
            CheckID:          "components.dashboard.status",
            CheckName:        "Components :: Dashboard :: Status",
            CheckDescription: "Validates dashboard component configuration and availability",
            CheckRemediation: "",
        },
    }
}

// CanApply determines if this check should run.
func (c *Check) CanApply(_ context.Context, _ check.Target) bool {
    // Check applies to all versions
    return true
}

// Validate executes the check and returns (result, error).
// Return error for infrastructure failures (API errors, etc.).
// Return result with failing conditions for validation failures.
func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    dr := c.NewResult()
    // Implementation - add conditions to dr
    return dr, nil
}
```

**Registration:** Checks are explicitly registered in `pkg/lint/command.go`:

```go
// In NewCommand()
registry.MustRegister(dashboard.NewCheck())
```

### Using BaseCheck

**BaseCheck** eliminates boilerplate by providing common check metadata through composition:

```go
type BaseCheck struct {
    CheckGroup       check.CheckGroup
    Kind             string
    Type             string
    CheckID          string
    CheckName        string
    CheckDescription string
    CheckRemediation string
}
```

**Methods provided by BaseCheck:**
- `ID()`, `Name()`, `Description()`, `Group()` - standard Check interface methods
- `CheckKind()`, `CheckType()` - returns `Kind` and `Type` fields respectively
- `Remediation()` - returns remediation guidance
- `NewResult()` - creates a DiagnosticResult initialized with check metadata

**Benefits:**
- No need to define constants for ID, name, description
- No need to implement `ID()`, `Name()`, `Description()`, `Group()`, `CheckKind()`, `CheckType()` methods
- `NewResult()` automatically creates results with check metadata
- Public fields `Kind` and `Type` can be accessed directly
- ~35% code reduction per check

**When to use:**
- All new checks should use BaseCheck
- Access metadata via public fields: `c.Kind`, `c.Type`, `c.CheckGroup`, etc.

## Registration Pattern

Lint checks are explicitly registered in `pkg/lint/command.go` within the `NewCommand()` constructor:

```go
// pkg/lint/command.go
func NewCommand(
    streams genericiooptions.IOStreams,
    configFlags *genericclioptions.ConfigFlags,
    options ...CommandOption,
) *Command {
    registry := check.NewRegistry()

    // Explicitly register all checks
    // Components (11)
    registry.MustRegister(codeflare.NewRemovalCheck())
    registry.MustRegister(dashboard.NewAcceleratorProfileMigrationCheck())
    registry.MustRegister(dashboard.NewHardwareProfileMigrationCheck())
    registry.MustRegister(datasciencepipelines.NewInstructLabRemovalCheck())
    // ... additional component checks

    // Dependencies (4)
    registry.MustRegister(certmanager.NewCheck())
    // ... additional dependency checks

    // Services (1)
    registry.MustRegister(servicemesh.NewRemovalCheck())

    // Workloads (7)
    registry.MustRegister(guardrails.NewOtelMigrationCheck())
    registry.MustRegister(kserveworkloads.NewAcceleratorMigrationCheck())
    registry.MustRegister(notebook.NewImpactedWorkloadsCheck())
    // ... additional workload checks

    return &Command{
        SharedOptions: shared,
        registry:      registry,
    }
}
```

**Benefits of explicit registration:**
- No global state - each command instance has its own registry
- Full test isolation - tests can register only needed checks
- Explicit dependencies - all checks visible in one place
- Deterministic registration order

## CanApply Versioning Logic

The `CanApply` method determines if a lint check is applicable based on version context.

### Lint Mode vs Upgrade Mode

Lint checks detect their execution mode by comparing versions:

```go
func (c *Check) CanApply(_ context.Context, target check.Target) bool {
    // Version fields are *semver.Version
    currentVer := target.CurrentVersion
    targetVer := target.TargetVersion

    // Handle nil versions
    if currentVer == nil || targetVer == nil {
        return false
    }

    // Lint mode: validating current cluster state
    isLintMode := currentVer.EQ(*targetVer)

    // Upgrade mode: assessing upgrade readiness
    isUpgradeMode := !currentVer.EQ(*targetVer)

    // Example: check only applies when upgrading to 3.x
    if isUpgradeMode && targetVer.Major == 3 {
        return true
    }

    return false
}
```

### Common Patterns

**Check applies to all versions:**
```go
func (c *Check) CanApply(_ context.Context, _ check.Target) bool {
    return true
}
```

**Check applies only in upgrade mode:**
```go
func (c *Check) CanApply(_ context.Context, target check.Target) bool {
    if target.CurrentVersion == nil || target.TargetVersion == nil {
        return false
    }
    return !target.CurrentVersion.EQ(*target.TargetVersion)
}
```

**Check applies when upgrading to specific version:**
```go
func (c *Check) CanApply(_ context.Context, target check.Target) bool {
    // Use version helper for clean version checks
    return version.IsVersionAtLeast(target.TargetVersion, 3, 0)
}
```

## DiagnosticResult Construction

### Creating Results

Use `result.New()` to create results with flattened metadata:

```go
import "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"

dr := result.New(
    "component",           // Group (flattened field)
    "dashboard",           // Kind (flattened field)
    "status",              // Name (flattened field)
    "Validates dashboard component configuration and availability", // Description
)
```

When using `BaseCheck`, prefer the convenience method:

```go
func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    dr := c.NewResult()  // Uses check metadata automatically
    // Add conditions...
    return dr, nil
}
```

### Creating Conditions

Use `check.NewCondition()` to create conditions with automatic Impact derivation:

```go
import (
    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Success: Status=True → Impact=None (auto-derived)
condition := check.NewCondition(
    check.ConditionTypeAvailable,
    metav1.ConditionTrue,
    check.ReasonResourceAvailable,
    "Dashboard component is ready",
)

// Failure: Status=False → Impact=Blocking (auto-derived)
condition := check.NewCondition(
    check.ConditionTypeConfigured,
    metav1.ConditionFalse,
    check.ReasonConfigurationInvalid,
    "Required configuration parameter 'replicas' not set",
)

// Override impact: Status=False but non-blocking
condition := check.NewCondition(
    check.ConditionTypeCompatible,
    metav1.ConditionFalse,
    check.ReasonDeprecated,
    "TrainingOperator is deprecated in RHOAI 3.3",
    check.WithImpact(result.ImpactAdvisory),  // Override to advisory
)

// Printf-style formatting
condition := check.NewCondition(
    check.ConditionTypeCompatible,
    metav1.ConditionFalse,
    check.ReasonWorkloadsImpacted,
    "Found %d active PyTorchJobs - workloads use deprecated TrainingOperator",
    activeCount,
    check.WithImpact(result.ImpactAdvisory),
)
```

**Impact Auto-Derivation Rules:**
- Status=True → Impact=None (requirement met, no issues)
- Status=False → Impact=Blocking (requirement not met, blocks upgrade)
- Status=Unknown → Impact=Advisory (unable to determine, proceed with caution)

**Overriding Impact:**
Use `check.WithImpact()` functional option when the default impact is not appropriate:
- Deprecation warnings (Status=False but non-blocking)
- Advisory notices (Status=False but upgrade can proceed)

**Validation:**
Conditions are validated at creation time. Invalid combinations will panic:
- Status=True with Impact≠None (invalid - if met, there's no impact)
- Status=False/Unknown with Impact=None (invalid - if not met, there must be impact)

### Condition Status and Impact Semantics

**Status** indicates whether a requirement is met:
- **True**: Requirement is MET (check passing)
- **False**: Requirement is NOT MET (check failing)
- **Unknown**: Unable to determine if requirement is met (error state)

**Impact** indicates the upgrade/operational impact:
- **blocking**: Upgrade cannot proceed (critical issue requiring action)
- **advisory**: Upgrade can proceed with warning (non-critical, user should be aware)
- **none** (empty): No impact (success state, requirement met)

**Valid Combinations:**
- Status=True + Impact=None ✓ (requirement met, no issues)
- Status=False + Impact=Blocking ✓ (requirement not met, critical)
- Status=False + Impact=Advisory ✓ (requirement not met, but non-blocking)
- Status=Unknown + Impact=Advisory ✓ (cannot determine, proceed with caution)

**Invalid Combinations (will panic):**
- Status=True + Impact=Blocking ✗ (if requirement is met, there's no blocking impact)
- Status=False + Impact=None ✗ (if requirement is not met, there must be some impact)

### Adding Annotations

Version information is added via the flattened `Annotations` map:

```go
// Annotations is a map[string]string field on DiagnosticResult
dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()

// Or add directly
dr.Annotations["check.opendatahub.io/source-version"] = target.CurrentVersion.String()
```

**Important:** Annotation keys must use domain-qualified format (`domain.tld/key`).

### Validation

Results are validated via `DiagnosticResult.Validate()`. Validation ensures:
- `Group` is not empty (flattened field)
- `Kind` is not empty (flattened field)
- `Name` is not empty (flattened field)
- `Status.Conditions` contains at least one condition
- All condition `Type` fields are not empty
- All condition `Status` values are "True", "False", or "Unknown"
- All condition `Reason` fields are not empty
- All annotation keys are in `domain.tld/key` format

## JQ-Based Field Access

All operations on unstructured objects in lint checks MUST use JQ queries via `pkg/util/jq`.

### Reading Fields

```go
import "github.com/lburgazzoli/odh-cli/pkg/util/jq"

// Read a field value
result, err := jq.Query(obj, ".spec.components.dashboard.managementState")
if err != nil {
    return fmt.Errorf("failed to query management state: %w", err)
}

managementState, ok := result.(string)
if !ok {
    return fmt.Errorf("management state is not a string")
}
```

### Complex Queries

JQ supports full query syntax:

```go
// Query with default value
result, err := jq.Query(obj, ".metadata.labels.version // \"unknown\"")

// Check if field exists
result, err := jq.Query(obj, ".spec.components | has(\"kserve\")")

// Array operations
result, err := jq.Query(obj, ".status.conditions | map(select(.type == \"Ready\")) | length")
```

### Prohibited

Direct use of `unstructured.Unstructured` accessor methods is **prohibited**:
- ❌ `unstructured.NestedString()`
- ❌ `unstructured.NestedField()`
- ❌ `unstructured.SetNestedField()`
- ❌ `unstructured.NestedFieldCopy()`

**Use JQ instead** for consistent, expressive queries.

## Centralized GVK/GVR Usage

All GroupVersionKind (GVK) and GroupVersionResource (GVR) references in lint checks MUST use definitions from `pkg/resources/types.go`.

### Accessing Resource Types

```go
import "github.com/lburgazzoli/odh-cli/pkg/resources"

// Get GVK
gvk := resources.DataScienceCluster.GVK()

// Get GVR for dynamic client
gvr := resources.DataScienceCluster.GVR()

// Get APIVersion for unstructured objects
apiVersion := resources.DataScienceCluster.APIVersion()
```

### Using in Lint Checks

```go
// List DataScienceCluster resources via client.Reader
dscList, err := target.Client.List(ctx, resources.DataScienceCluster)

// Or use standalone helper for singletons
dsc, err := client.GetDataScienceCluster(ctx, target.Client)
```

### Prohibited

Direct construction of GVK/GVR structs is **prohibited**:

```go
// ❌ WRONG: hardcoded GVK
gvk := schema.GroupVersionKind{
    Group:   "datasciencecluster.opendatahub.io",
    Version: "v1",
    Kind:    "DataScienceCluster",
}

// ✓ CORRECT: use centralized definition
gvk := resources.DataScienceCluster.GVK()
```

## High-Level Resource Targeting

Lint checks MUST operate on high-level custom resources, not low-level Kubernetes primitives.

### Permitted Targets

- **Component CRs**: DataScienceCluster, DSCInitialization
- **Workload CRs**: Notebook, InferenceService, RayCluster, PyTorchJob, etc.
- **Service CRs**: Custom resources representing OpenShift AI services
- **CRDs**: CustomResourceDefinition (for validating CRD presence)
- **OLM resources**: ClusterServiceVersion, Subscription

### Prohibited Targets

Lint checks MUST NOT target these as primary resources:
- ❌ Pod, Deployment, StatefulSet, ReplicaSet, DaemonSet
- ❌ Service, Ingress, Route
- ❌ ConfigMap, Secret
- ❌ PersistentVolume, PersistentVolumeClaim

**Exception:** Low-level resources MAY be queried as supporting evidence during high-level CR validation (e.g., checking if a Dashboard CR's backing Deployment exists), but MUST NOT be the primary target of a lint check.

### Example

```go
// ✓ CORRECT: Use validate.Component() builder (recommended)
func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    return validate.Component(c, target).
        InState(check.ManagementStateManaged).
        Run(ctx, func(ctx context.Context, req *validate.ComponentRequest) error {
            // Validate Dashboard component via req.DSC
            return nil
        })
}

// ✓ CORRECT: Manual approach using standalone helpers
func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    dr := c.NewResult()

    dsc, err := client.GetDataScienceCluster(ctx, target.Client)
    if err != nil {
        return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
    }
    // Validate Dashboard component within DSC
    return dr, nil
}

// ❌ WRONG: Check Dashboard Deployment directly
func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    // target.Client is client.Reader - it does not support arbitrary Get by ObjectKey
    // This also violates high-level resource principle
}
```

## Cluster-Wide Scope

Lint checks MUST operate cluster-wide and scan all namespaces. Namespace filtering is prohibited.

### Discovering Resources

When discovering workloads or services, scan all namespaces:

```go
// ✓ CORRECT: List across all namespaces using client.Reader
notebooks, err := target.Client.List(ctx, resources.Notebook)  // No namespace restriction
if err != nil {
    return nil, fmt.Errorf("listing notebooks: %w", err)
}

// ✓ CORRECT: Or use the workload builder (recommended)
return validate.Workloads(c, target, resources.Notebook).
    Run(ctx, func(ctx context.Context, req *validate.WorkloadRequest[*unstructured.Unstructured]) error {
        for _, nb := range req.Items {
            // Validate notebook configuration
        }
        return nil
    })
```

### Handling Multi-Namespace Results

Process resources from all namespaces:

```go
for _, nb := range notebooks {
    namespace := nb.GetNamespace()
    name := nb.GetName()

    // Validate notebook configuration
    // Report issues with namespace context in condition message
}
```

## Efficient Resource Discovery

When discovering resources in lint checks, choose the appropriate retrieval method based on what data you need.

### Using ListMetadata for Annotation/Label Checks

When you only need metadata (annotations, labels, name, namespace), use `ListMetadata()`:

```go
// ✓ CORRECT: Only need annotations - use ListMetadata
notebooks, err := target.Client.ListMetadata(ctx, resources.Notebook)
if err != nil {
    return nil, fmt.Errorf("listing notebooks: %w", err)
}

for _, nb := range notebooks {
    profileName := kube.GetAnnotation(nb, "notebooks.opendatahub.io/accelerator-name")
    if profileName != "" {
        // Process notebook with accelerator annotation
    }
}
```

### When Full Objects Are Required

Use full object retrieval when you need spec or status fields:

```go
// ✓ CORRECT: Need spec fields - use List with ResourceType
dspas, err := target.Client.List(ctx, resources.DataSciencePipelinesApplication)
if err != nil {
    return nil, fmt.Errorf("listing DSPAs: %w", err)
}

for _, dspa := range dspas {
    // Need to check .spec.apiServer.managedPipelines.instructLab
    instructLab, _ := jq.Query[bool](dspa, ".spec.apiServer.managedPipelines.instructLab")
}
```

### Populating ImpactedObjects

The `DiagnosticResult.ImpactedObjects` field uses `[]metav1.PartialObjectMetadata`. Populate it with resource references and optional context annotations:

```go
func populateImpactedObjects(
    dr *result.DiagnosticResult,
    impactedItems []types.NamespacedName,
) {
    dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(impactedItems))

    for _, item := range impactedItems {
        obj := metav1.PartialObjectMetadata{
            TypeMeta: resources.Notebook.TypeMeta(),
            ObjectMeta: metav1.ObjectMeta{
                Namespace: item.Namespace,
                Name:      item.Name,
                // Optional: Add context via annotations
                Annotations: map[string]string{
                    "status": "impacted",
                },
            },
        }
        dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
    }
}
```

### Decision Guide

| Need | Method | Returns |
|------|--------|---------|
| Name, namespace, labels, annotations | `ListMetadata()` | `[]*metav1.PartialObjectMetadata` |
| Spec fields | `List()` | `[]*unstructured.Unstructured` |
| Status fields | `List()` | `[]*unstructured.Unstructured` |
| Mixed (some metadata, some spec) | `List()` full objects | `[]*unstructured.Unstructured` |

## Validation Builders

The `pkg/lint/checks/shared/validate/` package provides fluent builder APIs that eliminate boilerplate for common check patterns. Using builders is the recommended approach for new checks.

### Component Builder

`validate.Component()` handles DSC fetching, component state filtering, and annotation population automatically. Use for checks that validate a component's configuration in the DataScienceCluster.

```go
import "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"

func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    return validate.Component(c, target).
        InState(check.ManagementStateManaged).
        Run(ctx, validate.Removal("CodeFlare is enabled (state: %s) but will be removed in RHOAI 3.x"))
}
```

**Fluent API:**
- `Component(c, target)` - Creates the builder. Component name is derived from `c.CheckKind()`
- `.InState(states...)` - Only validate when component is in one of the specified management states. If not called, validates for any state
- `.WithApplicationsNamespace()` - Also fetches the applications namespace from DSCI
- `.Run(ctx, fn)` - Fetches DSC, checks state, populates annotations, and calls `fn`

**The builder handles automatically:**
- DSC not found: returns a standard "not found" diagnostic result (not an error)
- DSC fetch error: returns wrapped error
- Component not in required state: returns a "not configured" diagnostic result
- Annotation population: management state and target version are automatically added

**`validate.Removal()` helper:** Returns a `ComponentValidateFn` that sets a compatibility failure condition. The management state is automatically prepended as the first format argument:

```go
validate.Removal("CodeFlare is enabled (state: %s) but will be removed in RHOAI 3.x")
```

**`ComponentRequest` struct** provides pre-fetched data to the validation function:

```go
type ComponentRequest struct {
    Result              *result.DiagnosticResult  // Pre-created with auto-populated annotations
    DSC                 *unstructured.Unstructured // Fetched DataScienceCluster
    ManagementState     string                     // Component's management state
    Client              client.Reader              // Read-only API access
    ApplicationsNamespace string                   // Populated when WithApplicationsNamespace() is used
}
```

### DSCI Builder

`validate.DSCI()` handles DSCInitialization fetching for service checks that read platform configuration from DSCI.

```go
func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    return validate.DSCI(c).Run(ctx, target, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
        managementState, err := jq.Query[string](dsci, ".spec.serviceMesh.managementState")

        switch {
        case errors.Is(err, jq.ErrNotFound):
            results.SetServiceNotConfigured(dr, "ServiceMesh")
        case err != nil:
            return fmt.Errorf("querying servicemesh managementState: %w", err)
        case managementState == check.ManagementStateManaged:
            results.SetCompatibilityFailuref(dr, "ServiceMesh is enabled (state: %s)", managementState)
        default:
            results.SetCompatibilitySuccessf(dr, "ServiceMesh is disabled (state: %s)", managementState)
        }

        return nil
    })
}
```

**Fluent API:**
- `DSCI(c)` - Creates the builder
- `.Run(ctx, target, fn)` - Fetches DSCI, populates annotations, and calls `fn` with the result and DSCI

### Operator Builder

`validate.Operator()` validates OLM operator presence via subscriptions. Use for dependency checks.

```go
func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    return validate.Operator(c, target).
        WithNames("cert-manager", "openshift-cert-manager-operator").
        Run(ctx)
}
```

**Fluent API:**
- `Operator(c, target)` - Creates the builder. Default matches subscription name == `c.CheckKind()`
- `.WithNames(names...)` - Override subscription name matching (OR semantics)
- `.WithChannels(channels...)` - Restrict matching to specific channels (must match both name AND channel)
- `.WithConditionBuilder(builder)` - Override the default Available condition builder
- `.Run(ctx)` - Checks OLM availability, finds subscriptions, and builds the result

### Workload Builder

`validate.Workloads()` and `validate.WorkloadsMetadata()` provide a generic builder for workload-based checks. The builder is parameterized over the item type (`*unstructured.Unstructured` or `*metav1.PartialObjectMetadata`).

```go
// Full objects (need spec/status fields)
func (c *OtelMigrationCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    return validate.Workloads(c, target, resources.GuardrailsOrchestrator).
        Filter(hasDeprecatedOtelFields).
        Complete(ctx, newOtelMigrationCondition)
}

// Metadata-only (only need name/namespace/annotations)
func (c *AcceleratorMigrationCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    return validate.WorkloadsMetadata(c, target, resources.Notebook).
        Filter(hasAcceleratorAnnotation).
        Complete(ctx, newAcceleratorMigrationCondition)
}
```

**Fluent API:**
- `Workloads(c, target, resourceType)` - Lists full unstructured objects
- `WorkloadsMetadata(c, target, resourceType)` - Lists metadata-only objects
- `.Filter(fn)` - Adds a predicate to select matching items. Items where `fn` returns false are excluded
- `.Run(ctx, fn)` - Lists, filters, populates annotations, calls `fn`, and auto-populates `ImpactedObjects` if the callback didn't set them
- `.Complete(ctx, fn)` - Higher-level alternative to `Run` for checks that only need to set conditions. `fn` returns `([]result.Condition, error)` and the builder sets them on the result

**Auto-populated by the builder:**
- Target version annotation
- Impacted workload count annotation
- `ImpactedObjects` (if callback doesn't set them)

## Migration Helper

The `pkg/lint/checks/shared/migration/` package provides a helper for API group migration checks that follow a common pattern: list resources, report count as advisory if found, report no-migration if empty.

```go
import "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/migration"

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    dr := c.NewResult()

    err := migration.ValidateResources(ctx, target, dr, migration.Config{
        ResourceType:           resources.AcceleratorProfile,
        ResourceLabel:          "AcceleratorProfile",
        NoMigrationMessage:     "No AcceleratorProfiles found - no migration needed",
        MigrationPendingMessage: "Found %d AcceleratorProfiles that will be auto-migrated to HardwareProfile API group",
    })
    if err != nil {
        return nil, err
    }

    return dr, nil
}
```

**`migration.Config` fields:**
- `ResourceType` - The Kubernetes resource type to discover
- `ResourceLabel` - Human-readable name used in condition messages
- `NoMigrationMessage` - Message when no resources are found
- `MigrationPendingMessage` - Printf format for the message when resources are found (must contain `%d`)

**The helper handles:**
- Target version annotation population
- Metadata-only listing for efficiency
- CRD-not-found treated as empty list
- Impacted workload count annotation
- ImpactedObjects population

## Complete Example

Here's a complete lint check implementation using the `validate.Component()` builder (the recommended pattern):

```go
// pkg/lint/checks/components/codeflare/codeflare.go
package codeflare

import (
    "context"

    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
    "github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type RemovalCheck struct {
    base.BaseCheck
}

func NewRemovalCheck() *RemovalCheck {
    return &RemovalCheck{
        BaseCheck: base.BaseCheck{
            CheckGroup:       check.GroupComponent,
            Kind:             "codeflare",
            Type:             check.CheckTypeRemoval,
            CheckID:          "components.codeflare.removal",
            CheckName:        "Components :: CodeFlare :: Removal (3.x)",
            CheckDescription: "Validates that CodeFlare is disabled before upgrading from RHOAI 2.x to 3.x (component will be removed)",
        },
    }
}

// CanApply returns whether this check should run for the given target.
func (c *RemovalCheck) CanApply(_ context.Context, target check.Target) bool {
    return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    return validate.Component(c, target).
        InState(check.ManagementStateManaged).
        Run(ctx, validate.Removal("CodeFlare is enabled (state: %s) but will be removed in RHOAI 3.x"))
}

// Registration is done explicitly in pkg/lint/command.go:
// registry.MustRegister(codeflare.NewRemovalCheck())
```

**Note:** For complex checks that don't fit the builder pattern, you can still write checks without builders using `c.NewResult()` and manually fetching resources via `client.GetDataScienceCluster(ctx, target.Client)`.

## Testing Lint Checks

Write tests using vanilla Gomega. Note that `target.Client` is `client.Reader` (an interface), so tests provide a mock or fake implementation:

```go
func TestRemovalCheck(t *testing.T) {
    g := NewWithT(t)

    t.Run("should flag when component is managed", func(t *testing.T) {
        // Build a fake client.Reader with a DSC that has the component managed
        fakeReader := testclient.NewFakeReader(
            testclient.WithDSC(map[string]any{
                "components": map[string]any{
                    "codeflare": map[string]any{
                        "managementState": "Managed",
                    },
                },
            }),
        )

        currentVer := semver.MustParse("2.17.0")
        targetVer := semver.MustParse("3.0.0")

        target := check.Target{
            Client:         fakeReader,
            CurrentVersion: &currentVer,
            TargetVersion:  &targetVer,
        }

        chk := NewRemovalCheck()
        result, err := chk.Validate(t.Context(), target)

        g.Expect(err).ToNot(HaveOccurred())
        g.Expect(result).To(HaveField("Group", "component"))
        g.Expect(result).To(HaveField("Kind", "codeflare"))
        g.Expect(result.Status.Conditions).To(HaveLen(1))
        g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
            "Type":   Equal("Compatible"),
            "Status": Equal(metav1.ConditionFalse),
        }))
    })
}
```

See [../testing.md](../testing.md) for comprehensive testing guidelines.
