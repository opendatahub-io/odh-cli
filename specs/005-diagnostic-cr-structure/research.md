# Research: Diagnostic Result CR Structure

**Date**: 2025-12-10
**Feature**: 005-diagnostic-cr-structure

## Overview

This research document consolidates findings on Kubernetes metav1.Condition patterns and best practices for implementing CR-like diagnostic result structures.

## Decision: Use metav1.Condition Pattern

**What was chosen**: Implement DiagnosticResult using Kubernetes metav1.Condition pattern with Type, Status, Reason, Message, and LastTransitionTime fields.

**Rationale**:
- Industry-standard pattern familiar to Kubernetes operators
- Rich semantic model supporting True/False/Unknown states
- Machine-readable (Reason) and human-readable (Message) separation
- Built-in time tracking (LastTransitionTime)
- Extensible via Type field for different validation aspects
- Compatible with kubectl and Kubernetes tooling expectations

**Alternatives considered**:
1. **Custom status enum** (Pass/Fail/Error/Skipped): Rejected because it's odh-cli-specific and doesn't align with Kubernetes conventions
2. **Boolean fields** (passed, failed, errored): Rejected due to lack of semantic richness and inability to express Unknown state
3. **Single status field**: Rejected because it can't represent multiple independent validation requirements

## Condition Field Specifications

### Type Field (required)
- **Format**: CamelCase or domain-qualified (e.g., `check.opendatahub.io/ConfigurationValid`)
- **Naming**: Use adjectives (Ready, Available) or past-tense verbs (Validated, Reconciled)
- **Polarity**: Choose what makes sense for humans (positive or negative)
- **Examples for diagnostics**:
  - `Validated`: Primary check validation condition
  - `Available`: Resource availability check
  - `Compatible`: Version/config compatibility
  - `Authorized`: Permission/access validation

### Status Field (required)
- **Values**: `True`, `False`, `Unknown` (only these three)
- **Semantics**:
  - `True`: Condition is met/resolved (check passed)
  - `False`: Condition not met (check failed)
  - `Unknown`: Insufficient information (check error/skipped)
- **Mapping from current ResultStatus**:
  - Pass → `True`
  - Fail → `False`
  - Error/Skipped → `Unknown`

### Reason Field (required)
- **Format**: One-word CamelCase identifier
- **Purpose**: Machine-readable programmatic reason
- **Must be**: Specific, unique, concise
- **Examples**:
  - Success: `RequirementsMet`, `ResourceAvailable`, `ConfigurationValid`
  - Failure: `ResourceNotFound`, `QuotaExceeded`, `PermissionDenied`
  - Unknown: `APIAccessDenied`, `CheckSkipped`, `InsufficientData`

### Message Field (required)
- **Format**: Human-readable text (can be empty)
- **Purpose**: Detailed context and explanation
- **Content**: Explain "why" not just "what"
- **Length**: Keep concise (<1024 bytes recommended)
- **Examples**:
  - `"All ServiceMesh components are properly configured and available"`
  - `"Failed to find required CRD 'servicemeshcontrolplanes.maistra.io' in cluster"`
  - `"Unable to query cluster resources: Forbidden (403) - check RBAC permissions"`

### LastTransitionTime Field (required)
- **Purpose**: Track when Status last changed
- **Update trigger**: Only when Status changes (True→False, False→Unknown, etc.)
- **Do NOT update**: When only Message or Reason changes
- **Type**: `metav1.Time` for Kubernetes compatibility

## Condition Ordering Decision

**Decision**: Order conditions by check execution sequence (order checks were run)

**Rationale**:
- Provides diagnostic reproducibility
- Matches temporal causality
- Easier to correlate with check execution logs
- No confusion about precedence or priority

**Alternative rejected**: Ordering by severity (failures first) - rejected because Kubernetes doesn't define precedence and it would break consistency with K8s patterns

## CR Structure Design

### Complete DiagnosticResult Structure

```go
package check

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DiagnosticResult represents a single diagnostic check result following
// Kubernetes Custom Resource conventions
type DiagnosticResult struct {
    // Metadata section (Kubernetes ObjectMeta/TypeMeta-alike)
    Metadata DiagnosticMetadata `json:"metadata"`

    // Spec section
    Spec DiagnosticSpec `json:"spec"`

    // Status section
    Status DiagnosticStatus `json:"status"`
}

// DiagnosticMetadata contains metadata about the diagnostic check
type DiagnosticMetadata struct {
    // Group categorizes the diagnostic target (e.g., "components", "services")
    Group string `json:"group"`

    // Kind identifies the specific target (e.g., "kserve", "auth")
    Kind string `json:"kind"`

    // Name identifies the specific check (e.g., "configuration-valid")
    Name string `json:"name"`

    // Annotations store version and other metadata
    Annotations map[string]string `json:"annotations,omitempty"`
}

// DiagnosticSpec contains the check description
type DiagnosticSpec struct {
    // Description explains what the check validates
    Description string `json:"description"`
}

// DiagnosticStatus contains the check results as conditions
type DiagnosticStatus struct {
    // Conditions array of validation requirements
    // Each condition represents a specific aspect being validated
    // Ordered by check execution sequence
    Conditions []DiagnosticCondition `json:"conditions"`
}

// DiagnosticCondition represents a single validation requirement
// Follows Kubernetes metav1.Condition pattern
type DiagnosticCondition struct {
    // Type of condition in CamelCase
    // Examples: "Validated", "Available", "Compatible"
    Type string `json:"type"`

    // Status: "True" (passed), "False" (failed), "Unknown" (error/skipped)
    Status ConditionStatus `json:"status"`

    // Reason: machine-readable CamelCase identifier
    // Examples: "RequirementsMet", "ResourceNotFound", "APIAccessDenied"
    Reason string `json:"reason"`

    // Message: human-readable explanation
    Message string `json:"message"`

    // LastTransitionTime: when Status last changed
    LastTransitionTime metav1.Time `json:"lastTransitionTime"`
}

type ConditionStatus string

const (
    ConditionTrue    ConditionStatus = "True"
    ConditionFalse   ConditionStatus = "False"
    ConditionUnknown ConditionStatus = "Unknown"
)
```

### Annotation Keys

**Standard annotations**:
- `check.opendatahub.io/source-version`: Current cluster version
- `check.opendatahub.io/target-version`: Target version for upgrade checks

**Format**: Domain-qualified keys following Kubernetes conventions

## Helper Functions

Recommended helper functions from `k8s.io/apimachinery/pkg/api/meta`:

```go
import "k8s.io/apimachinery/pkg/api/meta"

// Find specific condition
condition := meta.FindStatusCondition(result.Status.Conditions, "Validated")

// Check condition status
isPassing := meta.IsStatusConditionTrue(result.Status.Conditions, "Validated")
isFailing := meta.IsStatusConditionFalse(result.Status.Conditions, "Validated")

// Update or add condition (handles LastTransitionTime automatically)
meta.SetStatusCondition(&result.Status.Conditions, metav1.Condition{
    Type:    "Validated",
    Status:  metav1.ConditionTrue,
    Reason:  "RequirementsMet",
    Message: "All requirements validated successfully",
})
```

## Migration Strategy

### Backward Compatibility

**Current structure** (to be replaced):
```go
type DiagnosticResult struct {
    Status      ResultStatus       // Pass, Fail, Error, Skipped
    Severity    *Severity         // Critical, Warning, Info
    Message     string
    Details     map[string]any
    Remediation string
}
```

**Mapping to new structure**:

| Current Field | New Location | Notes |
|--------------|--------------|-------|
| Status (Pass) | Conditions[].Status = True | Primary condition Type="Validated" |
| Status (Fail) | Conditions[].Status = False | Severity maps to Reason |
| Status (Error) | Conditions[].Status = Unknown | Reason="CheckExecutionFailed" |
| Status (Skipped) | Conditions[].Status = Unknown | Reason="CheckSkipped" |
| Message | Conditions[].Message | Human-readable explanation |
| Details | Retained as-is | Additional diagnostic data |
| Remediation | Retained as-is | Actionable guidance |
| Severity (Critical) | Conditions[].Reason | "CriticalIssueFound" |
| Severity (Warning) | Conditions[].Reason | "WarningIssueFound" |

### Check Interface Impact

**Current interface** (assumed):
```go
type Check interface {
    Run(ctx context.Context, target *CheckTarget) DiagnosticResult
}
```

**No change required**: Check interface returns DiagnosticResult, internal structure changes are transparent to check implementations.

**Check implementations**: Update to return new CR-like structure with conditions array instead of single status.

## Table Rendering Strategy

### Multi-Row Rendering

**Decision**: One row per condition for multi-condition checks

**Format**:
```
GROUP      KIND     NAME                 TYPE        STATUS  REASON                MESSAGE
components kserve   version-compat       Validated   True    RequirementsMet       All version requirements met
components kserve   version-compat       Available   True    ResourceFound         KServe CRD found in cluster
```

**Rendering logic**:
1. Iterate through all DiagnosticResult objects
2. For each result, iterate through Status.Conditions array
3. Emit one table row per condition
4. Include metadata (Group/Kind/Name) and condition details (Type/Status/Reason/Message)

### JSON/YAML Output

Maintain full CR structure in structured output:

```yaml
metadata:
  group: components
  kind: kserve
  name: version-compatibility
  annotations:
    check.opendatahub.io/source-version: "2.15"
    check.opendatahub.io/target-version: "3.0"
spec:
  description: "Validates KServe version compatibility for upgrade"
status:
  conditions:
  - type: Validated
    status: "True"
    reason: RequirementsMet
    message: "KServe v0.11 is compatible with OpenShift AI 3.0"
    lastTransitionTime: "2025-12-10T10:00:00Z"
```

## References

- [Kubernetes API Conventions - Conditions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [metav1.Condition type definition](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Condition)
- [apimachinery/pkg/api/meta helpers](https://pkg.go.dev/k8s.io/apimachinery/pkg/api/meta)
- [Pod Lifecycle - Kubernetes Docs](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/)
- [Deployment Conditions - Kubernetes Docs](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)