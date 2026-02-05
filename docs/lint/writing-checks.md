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
    CanApply(target Target) bool
    Validate(ctx context.Context, target Target) (*result.DiagnosticResult, error)
}
```

**Key differences from typical interfaces:**
- `Group()` returns `CheckGroup` type (not string)
- `CanApply()` takes `Target` directly (no `ctx` parameter)
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
    "github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type Check struct {
    base.BaseCheck
}

func NewCheck() *Check {
    return &Check{
        BaseCheck: base.BaseCheck{
            CheckGroup:       check.GroupComponent,
            Kind:             check.ComponentDashboard,
            CheckType:        check.CheckTypeInstalled,
            CheckID:          "components.dashboard.status",
            CheckName:        "Components :: Dashboard :: Status",
            CheckDescription: "Validates dashboard component configuration and availability",
        },
    }
}

// CanApply determines if this check should run (no ctx parameter).
func (c *Check) CanApply(target check.Target) bool {
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

**Benefits:**
- No need to define constants for ID, name, description
- No need to implement `ID()`, `Name()`, `Description()`, `Group()` methods
- `NewResult()` automatically creates results with check metadata
- Public fields `Kind` and `CheckType` can be accessed directly
- ~35% code reduction per check

**When to use:**
- All new checks should use BaseCheck
- Access metadata via public fields: `c.Kind`, `c.CheckType`, `c.CheckGroup`, etc.

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
    registry.MustRegister(dashboard.NewCheck())
    registry.MustRegister(kserve.NewServerlessRemovalCheck())
    registry.MustRegister(modelmesh.NewRemovalCheck())
    // ... more checks

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

The `CanApply` method determines if a lint check is applicable based on version context. Note that `CanApply` takes `Target` directly (no `ctx` parameter).

### Lint Mode vs Upgrade Mode

Lint checks detect their execution mode by comparing versions:

```go
func (c *Check) CanApply(target check.Target) bool {
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
func (c *Check) CanApply(target check.Target) bool {
    return true
}
```

**Check applies only in upgrade mode:**
```go
func (c *Check) CanApply(target check.Target) bool {
    if target.CurrentVersion == nil || target.TargetVersion == nil {
        return false
    }
    return !target.CurrentVersion.EQ(*target.TargetVersion)
}
```

**Check applies when upgrading to specific version:**
```go
func (c *Check) CanApply(target check.Target) bool {
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
// List DataScienceCluster resources
dscList := &unstructured.UnstructuredList{}
dscList.SetGroupVersionKind(resources.DataScienceCluster.GVK())

err := target.Client.List(ctx, dscList)
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
// ✓ CORRECT: Check Dashboard CR
func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    dr := c.NewResult()

    dsc, err := target.Client.GetDataScienceCluster(ctx)
    if err != nil {
        return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
    }
    // Validate Dashboard component within DSC
    return dr, nil
}

// ❌ WRONG: Check Dashboard Deployment directly
func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
    deployment := &appsv1.Deployment{}
    err := target.Client.Get(ctx, client.ObjectKey{
        Namespace: "opendatahub",
        Name:      "odh-dashboard",
    }, deployment)
    // This violates high-level resource principle
}
```

## Cluster-Wide Scope

Lint checks MUST operate cluster-wide and scan all namespaces. Namespace filtering is prohibited.

### Discovering Resources

When discovering workloads or services, scan all namespaces:

```go
// ✓ CORRECT: List across all namespaces
notebooks := &unstructured.UnstructuredList{}
notebooks.SetGroupVersionKind(resources.Notebook.GVK())

err := target.Client.List(ctx, notebooks)  // No namespace restriction

// ❌ WRONG: Restrict to specific namespace
err := target.Client.List(ctx, notebooks, &client.ListOptions{
    Namespace: "opendatahub",  // Prohibited
})
```

### Handling Multi-Namespace Results

Process resources from all namespaces:

```go
for _, item := range notebooks.Items {
    namespace, _ := jq.Query(&item, ".metadata.namespace")
    name, _ := jq.Query(&item, ".metadata.name")

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
// ✓ CORRECT: Need spec fields - use List with unstructured
dspas := &unstructured.UnstructuredList{}
dspas.SetGroupVersionKind(resources.DataSciencePipelinesApplication.GVK())

err := target.Client.List(ctx, dspas)
if err != nil {
    return nil, fmt.Errorf("listing DSPAs: %w", err)
}

for _, dspa := range dspas.Items {
    // Need to check .spec.apiServer.managedPipelines.instructLab
    instructLab, _ := jq.Query[bool](&dspa, ".spec.apiServer.managedPipelines.instructLab")
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
| Spec fields | `List()` with unstructured | `*unstructured.UnstructuredList` |
| Status fields | `List()` with unstructured | `*unstructured.UnstructuredList` |
| Mixed (some metadata, some spec) | `List()` full objects | `*unstructured.UnstructuredList` |

## Complete Example

Here's a complete lint check implementation using BaseCheck and the Impact-based API:

```go
// pkg/lint/checks/components/kserve/serverless_removal.go
package kserve

import (
    "context"
    "fmt"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
    "github.com/lburgazzoli/odh-cli/pkg/util/jq"
    "github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type ServerlessRemovalCheck struct {
    base.BaseCheck
}

func NewServerlessRemovalCheck() *ServerlessRemovalCheck {
    return &ServerlessRemovalCheck{
        BaseCheck: base.BaseCheck{
            CheckGroup:       check.GroupComponent,
            Kind:             check.ComponentKServe,
            CheckType:        check.CheckTypeRemoval,
            CheckID:          "components.kserve.serverless-removal",
            CheckName:        "Components :: KServe :: Serverless Removal (3.x)",
            CheckDescription: "Validates that serverless components are removed when upgrading to 3.x",
        },
    }
}

// CanApply - no ctx parameter, takes Target directly.
func (c *ServerlessRemovalCheck) CanApply(target check.Target) bool {
    // Only applies when upgrading to 3.x
    return version.IsVersionAtLeast(target.TargetVersion, 3, 0)
}

// Validate - returns (*result.DiagnosticResult, error).
func (c *ServerlessRemovalCheck) Validate(
    ctx context.Context,
    target check.Target,
) (*result.DiagnosticResult, error) {
    dr := c.NewResult()

    // Get DataScienceCluster using client helper
    dsc, err := target.Client.GetDataScienceCluster(ctx)
    switch {
    case apierrors.IsNotFound(err):
        // Return result for "not found" case (not an error)
        return results.DataScienceClusterNotFound(
            string(c.Group()),
            c.Kind,
            c.CheckType,
            c.Description(),
        ), nil
    case err != nil:
        // Return error for infrastructure failures
        return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
    }

    // Check serverless configuration using JQ
    serverlessState, err := jq.Query[string](dsc, ".spec.components.kserve.serving.managementState")
    if err != nil {
        return nil, fmt.Errorf("querying serverless managementState: %w", err)
    }

    // Add annotations to flattened Annotations map
    dr.Annotations[check.AnnotationComponentManagementState] = serverlessState
    if target.TargetVersion != nil {
        dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
    }

    // Evaluate serverless state
    if serverlessState == "" || serverlessState == check.ManagementStateRemoved {
        // Success: serverless removed or not configured
        results.SetCompatibilitySuccessf(dr,
            "Serverless components are removed (state: %s) - ready for 3.x upgrade",
            serverlessState,
        )
        return dr, nil
    }

    // Failure: serverless still configured (blocking upgrade)
    results.SetCondition(dr, check.NewCondition(
        check.ConditionTypeCompatible,
        metav1.ConditionFalse,
        check.ReasonVersionIncompatible,
        "Serverless components are still configured (state: %s) - must be removed before upgrading to 3.x",
        serverlessState,
        // Impact=Blocking is auto-derived from Status=False
    ))

    return dr, nil
}

// Registration is done explicitly in pkg/lint/command.go:
// registry.MustRegister(kserve.NewServerlessRemovalCheck())
```

## Testing Lint Checks

Write tests using vanilla Gomega and fake clients:

```go
// pkg/lint/checks/components/kserve/serverless_removal_test.go
package kserve

import (
    "testing"

    "github.com/blang/semver/v4"
    . "github.com/onsi/gomega"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"

    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/resources"
    "github.com/lburgazzoli/odh-cli/pkg/util/client"
)

func TestServerlessRemovalCheck(t *testing.T) {
    g := NewWithT(t)

    t.Run("should pass when serverless is removed", func(t *testing.T) {
        dsc := &unstructured.Unstructured{}
        dsc.SetGroupVersionKind(resources.DataScienceCluster.GVK())
        dsc.SetName("default")
        dsc.Object["spec"] = map[string]any{
            "components": map[string]any{
                "kserve": map[string]any{
                    "serving": map[string]any{
                        "managementState": "Removed",
                    },
                },
            },
        }

        fakeClient := fake.NewClientBuilder().WithObjects(dsc).Build()
        currentVer := semver.MustParse("2.17.0")
        targetVer := semver.MustParse("3.0.0")

        // Target uses flattened version fields (*semver.Version)
        target := check.Target{
            Client:         client.NewClientWithClient(fakeClient),
            CurrentVersion: &currentVer,
            TargetVersion:  &targetVer,
        }

        chk := NewServerlessRemovalCheck()
        result, err := chk.Validate(t.Context(), target)

        g.Expect(err).ToNot(HaveOccurred())
        // Result has flattened fields (not Metadata.Group)
        g.Expect(result).To(HaveField("Group", "component"))
        g.Expect(result).To(HaveField("Kind", "kserve"))
        g.Expect(result.Status.Conditions).To(HaveLen(1))
        g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
            "Type":   Equal("Compatible"),
            "Status": Equal(metav1.ConditionTrue),
        }))
    })
}
```

See [../testing.md](../testing.md) for comprehensive testing guidelines.
