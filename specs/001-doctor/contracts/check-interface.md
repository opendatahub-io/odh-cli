# Contract: Check Interface

**Feature**: 001-doctor | **Date**: 2025-12-06 | **Type**: Interface Definition

## Overview

Defines the contract for diagnostic checks in the doctor subcommand. All checks must implement this interface to be registered and executed by the CheckRegistry.

---

## Interface Definition

### Go Interface

```go
package check

import (
    "context"
    "time"
)

// Check represents a diagnostic test that validates a specific aspect of cluster configuration
type Check interface {
    // ID returns the unique identifier for this check
    ID() string

    // Name returns the human-readable check name
    Name() string

    // Description returns what this check validates
    Description() string

    // Category returns the check category (component, service, workload)
    Category() CheckCategory

    // ApplicableVersions returns semver constraint for applicable versions
    ApplicableVersions() string

    // Validate executes the check against the provided target
    // Returns DiagnosticResult with status, severity, and remediation guidance
    Validate(ctx context.Context, target *CheckTarget) (*DiagnosticResult, error)

    // RemediationHint returns actionable guidance for addressing failures
    RemediationHint() string
}
```

---

## Contract Guarantees

### Implementer Responsibilities

**ID()**
- MUST return a unique identifier across all checks
- MUST be lowercase with hyphens (kebab-case)
- MUST follow pattern: `{category}-{component}-{aspect}` (e.g., "component-dashboard-deployment-exists")
- MUST be stable across versions (no renaming)

**Name()**
- MUST return a concise, human-readable name (1-5 words)
- MUST use Title Case
- Example: "Dashboard Deployment Exists"

**Description()**
- MUST return a clear description of what the check validates
- MUST be a complete sentence
- SHOULD be 10-30 words
- Example: "Validates that the ODH Dashboard deployment exists and has at least one running replica"

**Category()**
- MUST return one of: CategoryComponent, CategoryService, CategoryWorkload
- MUST be consistent with check ID prefix

**ApplicableVersions()**
- MUST return a semver constraint string (e.g., ">=2.0.0", ">=2.10.0 <3.0.0", "*")
- MUST be non-empty
- MUST be a valid semver constraint expression
- Use "*" to apply to all versions
- Examples:
  - `">=2.0.0"` - Applies to 2.x and above
  - `">=2.10.0 <3.0.0"` - Applies only to 2.10.x and 2.x versions >= 2.10
  - `">=3.0.0"` - Applies only to 3.x
  - `"*"` - Applies to all versions

**Validate(ctx, target)**
- MUST respect context cancellation
- MUST NOT modify cluster state (read-only operations only)
- MUST return DiagnosticResult with all required fields populated
- MUST use `pkg/util/jq.Query()` for unstructured field access (Principle VII)
- MUST wrap errors with `fmt.Errorf("%w")` for error chain propagation
- SHOULD complete within 10 seconds for typical clusters
- MAY return error for infrastructure failures (network, permissions)
- MUST use target.Client for all Kubernetes API access
- MUST use target.ConfigBundle for version-specific validation rules

**RemediationHint()**
- MUST return actionable guidance for users
- MUST include specific kubectl commands or configuration changes when applicable
- SHOULD be 1-5 sentences
- Example: "Ensure ODH Dashboard is enabled in DataScienceCluster by running: kubectl patch datasciencecluster default-dsc..."

---

## DiagnosticResult Contract

The DiagnosticResult returned by Validate() MUST satisfy:

**Required Fields:**
- `CheckID`: MUST match Check.ID()
- `CheckName`: MUST match Check.Name()
- `Status`: MUST be one of Pass, Fail, Error, Skipped
- `Timestamp`: MUST be set to execution time
- `ExecutionTime`: MUST reflect actual check duration

**Conditional Fields:**
- `Severity`: MUST be set if Status=Fail (determined by check implementation based on the specific finding), MUST be nil if Status=Pass or Skipped
- `Message`: MUST be non-empty for Fail or Error status
- `RemediationHint`: MUST be set if Status=Fail, SHOULD match Check.RemediationHint()
- `AffectedResources`: SHOULD be populated for Fail status with specific resources

**Severity Guidelines:**
Checks SHOULD use the following guidelines when setting severity in DiagnosticResult:
- `SeverityCritical`: Blocking issues requiring immediate action (e.g., component not running, missing required resources, version incompatibility)
- `SeverityWarning`: Non-blocking problems needing attention (e.g., suboptimal configuration, low replica count, deprecated settings)
- `SeverityInfo`: Optimization suggestions and best practices (e.g., resource limits not set, unused components enabled)

**Status Semantics:**
- **Pass**: Check completed successfully, no issues found
- **Fail**: Check completed, found configuration issue or misconfiguration
- **Error**: Check could not complete due to infrastructure problem (permissions, network, missing resources)
- **Skipped**: Check not applicable to this cluster (e.g., version mismatch, component not installed)

---

## CheckTarget Contract

The CheckTarget passed to Validate() provides:

**Client Access:**
- `target.Client.Dynamic`: Use for resource queries with dynamic client
- `target.Client.Discovery`: Use for API resource discovery
- `target.Client.APIExtensions`: Use for CRD discovery and inspection

**Context Information:**
- `target.Version`: Detected cluster version (read-only)
- `target.Resource`: Specific resource being checked (for workload checks, nil for component/service checks)

**Execution Context:**
The CheckRegistry populates CheckTarget differently based on check category:

1. **Component/Service Checks** (`target.Resource == nil`):
   - Check implementation queries resources directly using client
   - Example: Dashboard check queries deployment in known namespace

2. **Workload Checks** (`target.Resource != nil`):
   - CheckRegistry discovers workload CRDs, lists instances, and executes check for each instance
   - Example: Notebook check receives specific Notebook CR in `target.Resource`
   - Get namespace via `target.Resource.GetNamespace()`

**Access Patterns:**
```go
// Component/Service check: Get specific resource using helper method
deployment, err := target.Client.Get(ctx, resources.Deployment.GVR(), "odh-dashboard",
    client.InNamespace("opendatahub"))
if err != nil {
    if apierrors.IsNotFound(err) {
        severity := check.SeverityCritical
        return &check.DiagnosticResult{
            CheckID:         c.ID(),
            CheckName:       c.Name(),
            Status:          check.StatusFail,
            Severity:        &severity,
            Message:         "Deployment not found",
            RemediationHint: c.RemediationHint(),
            ExecutionTime:   time.Since(startTime),
            Timestamp:       time.Now(),
        }, nil
    }
    return nil, fmt.Errorf("getting deployment: %w", err)
}

// Alternative: Query resources directly with dynamic client
gvr := resources.Deployment.GVR()  // Centralized GVK/GVR (Principle VIII)
deployment, err := target.Client.Dynamic.Resource(gvr).Namespace("opendatahub").Get(ctx, "odh-dashboard", metav1.GetOptions{})

// Workload check: Inspect provided resource
if target.Resource == nil {
    return nil, fmt.Errorf("workload check requires target resource")
}
namespace := target.Resource.GetNamespace()
name := target.Resource.GetName()

// Access fields with JQ (Principle VII)
replicas, err := jq.Query(deployment, ".status.replicas")

// Check version applicability
if target.Version.MajorVersion != "3.x" {
    return &DiagnosticResult{Status: StatusSkipped, Message: "Check only applies to 3.x"}, nil
}
```

---

## Implementation Patterns

### Component Check (Example)

All checks are implemented in Go code. Here's a complete example:

```go
// pkg/doctor/checks/components/dashboard.go
package components

import (
    "context"
    "fmt"
    "time"

    "github.com/lburgazzoli/odh-cli/pkg/doctor/check"
    "github.com/lburgazzoli/odh-cli/pkg/resources"
    "github.com/lburgazzoli/odh-cli/pkg/util/jq"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DashboardDeploymentCheck struct{}

func NewDashboardDeploymentCheck() *DashboardDeploymentCheck {
    return &DashboardDeploymentCheck{}
}

func (c *DashboardDeploymentCheck) ID() string {
    return "component-dashboard-deployment-exists"
}

func (c *DashboardDeploymentCheck) Name() string {
    return "Dashboard Deployment Exists"
}

func (c *DashboardDeploymentCheck) Description() string {
    return "Validates that the ODH Dashboard deployment exists and has at least one running replica"
}

func (c *DashboardDeploymentCheck) Category() check.CheckCategory {
    return check.CategoryComponent
}

func (c *DashboardDeploymentCheck) ApplicableVersions() string {
    return ">=2.0.0" // Applies to all 2.x and 3.x versions
}

func (c *DashboardDeploymentCheck) RemediationHint() string {
    return `Ensure ODH Dashboard is enabled in DataScienceCluster:
kubectl patch datasciencecluster default-dsc -n opendatahub --type=merge \
  -p '{"spec":{"components":{"dashboard":{"managementState":"Managed"}}}}'`
}

func (c *DashboardDeploymentCheck) Validate(ctx context.Context, target *check.CheckTarget) (*check.DiagnosticResult, error) {
    startTime := time.Now()

    // Expected values hardcoded in check (can be made version-specific if needed)
    const (
        namespace      = "opendatahub"
        deploymentName = "odh-dashboard"
        minReplicas    = 1
    )

    // Query deployment using dynamic client
    deployment, err := target.Client.Dynamic.
        Resource(resources.Deployment.GVR()).
        Namespace(namespace).
        Get(ctx, deploymentName, metav1.GetOptions{})

    if err != nil {
        if apierrors.IsNotFound(err) {
            severity := check.SeverityCritical
            return &check.DiagnosticResult{
                CheckID:         c.ID(),
                CheckName:       c.Name(),
                Status:          check.StatusFail,
                Severity:        &severity,
                Message:         fmt.Sprintf("Deployment %s not found in namespace %s", deploymentName, namespace),
                RemediationHint: c.RemediationHint(),
                ExecutionTime:   time.Since(startTime),
                Timestamp:       time.Now(),
            }, nil
        }
        return nil, fmt.Errorf("querying deployment: %w", err)
    }

    // Use JQ for field access (Principle VII)
    replicas, err := jq.Query(deployment, ".status.replicas")
    if err != nil {
        return nil, fmt.Errorf("querying replicas field: %w", err)
    }

    // Validate replicas
    replicasInt, ok := replicas.(int64)
    if !ok || replicasInt < minReplicas {
        // Use Warning severity for low replicas (less severe than missing deployment)
        severity := check.SeverityWarning
        return &check.DiagnosticResult{
            CheckID:   c.ID(),
            CheckName: c.Name(),
            Status:    check.StatusFail,
            Severity:  &severity,
            Message:   fmt.Sprintf("Dashboard has %d replicas, expected at least %d", replicasInt, minReplicas),
            AffectedResources: []check.ResourceReference{
                {
                    APIVersion: "apps/v1",
                    Kind:       "Deployment",
                    Name:       deploymentName,
                    Namespace:  namespace,
                },
            },
            RemediationHint: c.RemediationHint(),
            ExecutionTime:   time.Since(startTime),
            Timestamp:       time.Now(),
        }, nil
    }

    // Check passed
    return &check.DiagnosticResult{
        CheckID:       c.ID(),
        CheckName:     c.Name(),
        Status:        check.StatusPass,
        Message:       fmt.Sprintf("Dashboard deployment is healthy with %d replicas", replicasInt),
        ExecutionTime: time.Since(startTime),
        Timestamp:     time.Now(),
    }, nil
}

// Register check in init()
func init() {
    check.MustRegisterCheck(NewDashboardDeploymentCheck())
}
```

This pattern demonstrates:
- **Check logic in Go**: All validation logic is type-safe Go code with expected values as constants
- **JQ-based field access**: Uses `jq.Query()` for unstructured object fields (Principle VII)
- **Centralized GVK/GVR**: Uses `resources.Deployment.GVR()` (Principle VIII)
- **Error wrapping**: Uses `fmt.Errorf("%w")` for error chains (Principle V)
- **Version-specific logic**: Can use `target.Version.MajorVersion` to vary behavior if needed
- **Contextual severity**: Different failure modes have different severities (Critical for missing deployment, Warning for low replicas)

---

## Error Handling Requirements

**Network Errors:**
```go
_, err := target.Client.Dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
if err != nil {
    // IsNotFound = expected failure, return Fail status
    if apierrors.IsNotFound(err) {
        return &DiagnosticResult{Status: StatusFail, ...}, nil
    }
    // Other errors = infrastructure problem, return error
    return nil, fmt.Errorf("fetching resource: %w", err)
}
```

**Permission Errors:**
```go
if apierrors.IsForbidden(err) {
    return &DiagnosticResult{
        Status:          StatusError,
        Severity:        &SeverityCritical,
        Message:         "Insufficient permissions to check resources",
        RemediationHint: "Ensure service account has get/list permissions on required resources",
    }, nil
}
```

**Context Cancellation:**
```go
select {
case <-ctx.Done():
    return nil, ctx.Err()
default:
    // Continue with check
}
```

---

## Validation and Testing

**Unit Test Requirements:**
- MUST test with fake dynamic client (`k8s.io/client-go/dynamic/fake`)
- MUST test all status outcomes (Pass, Fail, Error, Skipped)
- MUST verify DiagnosticResult fields are correctly populated
- MUST test context cancellation handling

**Integration Test Requirements:**
- MUST test with k3s-envtest
- MUST verify behavior against real cluster resources
- SHOULD test permission error scenarios

**Example Unit Test:**
```go
func TestComponentCheckValidate(t *testing.T) {
    g := gomega.NewWithT(t)
    ctx := t.Context()

    t.Run("ResourceExists_Pass", func(t *testing.T) {
        // Setup fake client with resource
        fakeClient := fake.NewSimpleDynamicClient(scheme, testDeployment)
        target := &CheckTarget{
            Client: &client.Client{Dynamic: fakeClient},
            Version: ClusterVersion{MajorVersion: "3.x"},
        }

        check := NewComponentCheck(...)
        result, err := check.Validate(ctx, target)

        g.Expect(err).NotTo(gomega.HaveOccurred())
        g.Expect(result.Status).To(gomega.Equal(StatusPass))
    })

    t.Run("ResourceMissing_Fail", func(t *testing.T) {
        // Setup fake client without resource
        fakeClient := fake.NewSimpleDynamicClient(scheme)
        target := &CheckTarget{
            Client: &client.Client{Dynamic: fakeClient},
            Version: ClusterVersion{MajorVersion: "3.x"},
        }

        check := NewComponentCheck(...)
        result, err := check.Validate(ctx, target)

        g.Expect(err).NotTo(gomega.HaveOccurred())
        g.Expect(result.Status).To(gomega.Equal(StatusFail))
        g.Expect(*result.Severity).To(gomega.Equal(SeverityCritical))
        g.Expect(result.RemediationHint).NotTo(gomega.BeEmpty())
    })
}
```

---

## Version Compatibility

Checks MUST declare ApplicableVersions to ensure they only run against compatible cluster versions:

**Version Patterns:**
- `["2.x"]`: Check applies only to OpenShift AI 2.x
- `["3.x"]`: Check applies only to OpenShift AI 3.x
- `["2.x", "3.x"]`: Check applies to both versions

**Version-Specific Behavior:**
```go
func (c *MyCheck) Validate(ctx context.Context, target *CheckTarget) (*DiagnosticResult, error) {
    switch target.Version.MajorVersion {
    case "2.x":
        return c.validate2x(ctx, target)
    case "3.x":
        return c.validate3x(ctx, target)
    default:
        return &DiagnosticResult{
            Status:  StatusSkipped,
            Message: fmt.Sprintf("Check not applicable to version %s", target.Version.Version),
        }, nil
    }
}
```

---

## Performance Requirements

**Execution Time:**
- Individual checks SHOULD complete within 10 seconds
- Checks MAY take longer for large clusters but MUST respect context timeout
- Checks MUST be independently executable (no cross-check dependencies)

**Resource Usage:**
- Checks MUST NOT hold references to large objects after execution
- Checks SHOULD batch list operations when checking multiple resources
- Checks MUST use provided RESTMapper cache, not create new discovery clients

---

## Registration Contract

Checks are registered with CheckRegistry via:

```go
func (r *CheckRegistry) Register(check Check) error {
    // Validation
    if check.ID() == "" {
        return fmt.Errorf("check ID cannot be empty")
    }
    if _, exists := r.checks[check.ID()]; exists {
        return fmt.Errorf("check %s already registered", check.ID())
    }
    if check.ApplicableVersions() == "" {
        return fmt.Errorf("check %s has empty version constraint", check.ID())
    }

    // Validate semver constraint
    if _, err := semver.ParseRange(check.ApplicableVersions()); err != nil {
        return fmt.Errorf("check %s has invalid semver constraint %q: %w",
            check.ID(), check.ApplicableVersions(), err)
    }

    // Register
    r.checks[check.ID()] = check
    r.categories[check.Category()] = append(r.categories[check.Category()], check)

    return nil
}
```

**Registration Rules:**
- Checks MUST be registered before CheckRegistry.ExecuteAll() is called
- Duplicate IDs will cause registration failure
- Checks with empty ApplicableVersions will be rejected
- Checks are immutable after registration
