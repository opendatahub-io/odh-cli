# Quickstart: Diagnostic Result CR Structure

**Feature**: 005-diagnostic-cr-structure
**Audience**: Developers implementing or maintaining diagnostic checks

## Overview

This guide provides quick-start instructions for working with the new DiagnosticResult CR structure.

## Key Concepts

- **DiagnosticResult**: CR-like structure with metadata, spec, and status sections
- **Conditions**: Array of validation requirements, each with Type, Status, Reason, Message
- **Status Values**: "True" (passed), "False" (failed), "Unknown" (error/skipped)
- **Multi-Row Rendering**: One table row per condition for visibility

## Quick Examples

### Creating a DiagnosticResult

```go
import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
)

// Create a passing diagnostic result
result := check.DiagnosticResult{
    Metadata: check.DiagnosticMetadata{
        Group: "components",
        Kind:  "kserve",
        Name:  "configuration-valid",
        Annotations: map[string]string{
            "check.opendatahub.io/source-version": "2.15",
        },
    },
    Spec: check.DiagnosticSpec{
        Description: "Validates KServe component configuration and readiness",
    },
    Status: check.DiagnosticStatus{
        Conditions: []check.DiagnosticCondition{
            {
                Type:               "Validated",
                Status:             check.ConditionTrue,
                Reason:             "RequirementsMet",
                Message:            "All KServe configuration requirements validated",
                LastTransitionTime: metav1.Now(),
            },
        },
    },
}
```

### Creating a Failing Diagnostic

```go
result := check.DiagnosticResult{
    Metadata: check.DiagnosticMetadata{
        Group: "components",
        Kind:  "servicemesh",
        Name:  "operator-installed",
    },
    Spec: check.DiagnosticSpec{
        Description: "Validates ServiceMesh operator installation",
    },
    Status: check.DiagnosticStatus{
        Conditions: []check.DiagnosticCondition{
            {
                Type:               "Validated",
                Status:             check.ConditionFalse,
                Reason:             "OperatorNotFound",
                Message:            "ServiceMesh operator not found in namespace 'openshift-operators'",
                LastTransitionTime: metav1.Now(),
            },
        },
    },
}
```

### Multi-Condition Diagnostic

```go
result := check.DiagnosticResult{
    Metadata: check.DiagnosticMetadata{
        Group: "services",
        Kind:  "auth",
        Name:  "readiness-check",
    },
    Spec: check.DiagnosticSpec{
        Description: "Validates authentication service readiness",
    },
    Status: check.DiagnosticStatus{
        Conditions: []check.DiagnosticCondition{
            {
                Type:               "Available",
                Status:             check.ConditionTrue,
                Reason:             "ResourceFound",
                Message:            "Authentication service deployment found",
                LastTransitionTime: metav1.Now(),
            },
            {
                Type:               "Ready",
                Status:             check.ConditionTrue,
                Reason:             "PodsReady",
                Message:            "All auth service pods are ready (3/3)",
                LastTransitionTime: metav1.Now(),
            },
            {
                Type:               "Configured",
                Status:             check.ConditionTrue,
                Reason:             "ConfigValid",
                Message:            "Authentication provider configuration is valid",
                LastTransitionTime: metav1.Now(),
            },
        },
    },
}
```

## Common Patterns

### Mapping Old Status to Conditions

```go
// Old: ResultStatus.Pass
// New:
condition := check.DiagnosticCondition{
    Type:   "Validated",
    Status: check.ConditionTrue,
    Reason: "RequirementsMet",
    Message: "Check passed successfully",
    LastTransitionTime: metav1.Now(),
}

// Old: ResultStatus.Fail (Critical)
// New:
condition := check.DiagnosticCondition{
    Type:   "Validated",
    Status: check.ConditionFalse,
    Reason: "CriticalIssueFound",
    Message: "Resource not found: XYZ",
    LastTransitionTime: metav1.Now(),
}

// Old: ResultStatus.Fail (Warning)
// New:
condition := check.DiagnosticCondition{
    Type:   "Validated",
    Status: check.ConditionFalse,
    Reason: "WarningIssueFound",
    Message: "Configuration suboptimal but functional",
    LastTransitionTime: metav1.Now(),
}

// Old: ResultStatus.Error
// New:
condition := check.DiagnosticCondition{
    Type:   "Validated",
    Status: check.ConditionUnknown,
    Reason: "CheckExecutionFailed",
    Message: "Unable to query cluster: Forbidden (403)",
    LastTransitionTime: metav1.Now(),
}

// Old: ResultStatus.Skipped
// New:
condition := check.DiagnosticCondition{
    Type:   "Validated",
    Status: check.ConditionUnknown,
    Reason: "CheckSkipped",
    Message: "Check skipped: prerequisite not met",
    LastTransitionTime: metav1.Now(),
}
```

### Standard Condition Types

```go
const (
    // Primary validation condition
    ConditionTypeValidated = "Validated"

    // Resource availability
    ConditionTypeAvailable = "Available"

    // Resource readiness
    ConditionTypeReady = "Ready"

    // Version compatibility
    ConditionTypeCompatible = "Compatible"

    // Configuration validity
    ConditionTypeConfigured = "Configured"

    // Permission/access
    ConditionTypeAuthorized = "Authorized"
)
```

### Standard Reason Values

```go
// Success reasons
const (
    ReasonRequirementsMet      = "RequirementsMet"
    ReasonResourceFound        = "ResourceFound"
    ReasonResourceAvailable    = "ResourceAvailable"
    ReasonConfigurationValid   = "ConfigurationValid"
    ReasonVersionCompatible    = "VersionCompatible"
    ReasonPermissionGranted    = "PermissionGranted"
)

// Failure reasons
const (
    ReasonResourceNotFound      = "ResourceNotFound"
    ReasonResourceUnavailable   = "ResourceUnavailable"
    ReasonConfigurationInvalid  = "ConfigurationInvalid"
    ReasonVersionIncompatible   = "VersionIncompatible"
    ReasonPermissionDenied      = "PermissionDenied"
    ReasonQuotaExceeded         = "QuotaExceeded"
    ReasonDependencyUnavailable = "DependencyUnavailable"
)

// Unknown/Error reasons
const (
    ReasonCheckExecutionFailed = "CheckExecutionFailed"
    ReasonCheckSkipped         = "CheckSkipped"
    ReasonAPIAccessDenied      = "APIAccessDenied"
    ReasonInsufficientData     = "InsufficientData"
)
```

## Check Implementation Pattern

### Basic Check Structure

```go
package mycheck

import (
    "context"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
)

type Check struct {
    // check fields
}

func (c *Check) Run(ctx context.Context, target *check.CheckTarget) check.DiagnosticResult {
    result := check.DiagnosticResult{
        Metadata: check.DiagnosticMetadata{
            Group: "components",
            Kind:  "mycomponent",
            Name:  "my-check",
            Annotations: map[string]string{
                "check.opendatahub.io/source-version": target.CurrentVersion.String(),
            },
        },
        Spec: check.DiagnosticSpec{
            Description: "Validates my component configuration",
        },
        Status: check.DiagnosticStatus{
            Conditions: []check.DiagnosticCondition{},
        },
    }

    // Add version annotation if target version specified
    if target.Version != nil {
        result.Metadata.Annotations["check.opendatahub.io/target-version"] = target.Version.String()
    }

    // Perform validation and add conditions
    if err := c.validate(ctx, target); err != nil {
        result.Status.Conditions = append(result.Status.Conditions, check.DiagnosticCondition{
            Type:               "Validated",
            Status:             check.ConditionFalse,
            Reason:             "ValidationFailed",
            Message:            err.Error(),
            LastTransitionTime: metav1.Now(),
        })
    } else {
        result.Status.Conditions = append(result.Status.Conditions, check.DiagnosticCondition{
            Type:               "Validated",
            Status:             check.ConditionTrue,
            Reason:             "RequirementsMet",
            Message:            "All requirements validated successfully",
            LastTransitionTime: metav1.Now(),
        })
    }

    return result
}
```

### Multi-Condition Check

```go
func (c *Check) Run(ctx context.Context, target *check.CheckTarget) check.DiagnosticResult {
    result := check.DiagnosticResult{
        Metadata: check.DiagnosticMetadata{
            Group: "services",
            Kind:  "myservice",
            Name:  "comprehensive-check",
        },
        Spec: check.DiagnosticSpec{
            Description: "Comprehensive service health check",
        },
        Status: check.DiagnosticStatus{
            Conditions: []check.DiagnosticCondition{},
        },
    }

    // Check 1: Resource availability
    if available, msg := c.checkAvailability(ctx, target); available {
        result.Status.Conditions = append(result.Status.Conditions, check.DiagnosticCondition{
            Type:               "Available",
            Status:             check.ConditionTrue,
            Reason:             "ResourceFound",
            Message:            msg,
            LastTransitionTime: metav1.Now(),
        })
    } else {
        result.Status.Conditions = append(result.Status.Conditions, check.DiagnosticCondition{
            Type:               "Available",
            Status:             check.ConditionFalse,
            Reason:             "ResourceNotFound",
            Message:            msg,
            LastTransitionTime: metav1.Now(),
        })
    }

    // Check 2: Readiness
    if ready, msg := c.checkReadiness(ctx, target); ready {
        result.Status.Conditions = append(result.Status.Conditions, check.DiagnosticCondition{
            Type:               "Ready",
            Status:             check.ConditionTrue,
            Reason:             "PodsReady",
            Message:            msg,
            LastTransitionTime: metav1.Now(),
        })
    } else {
        result.Status.Conditions = append(result.Status.Conditions, check.DiagnosticCondition{
            Type:               "Ready",
            Status:             check.ConditionFalse,
            Reason:             "PodsNotReady",
            Message:            msg,
            LastTransitionTime: metav1.Now(),
        })
    }

    // Check 3: Configuration
    if valid, msg := c.checkConfiguration(ctx, target); valid {
        result.Status.Conditions = append(result.Status.Conditions, check.DiagnosticCondition{
            Type:               "Configured",
            Status:             check.ConditionTrue,
            Reason:             "ConfigValid",
            Message:            msg,
            LastTransitionTime: metav1.Now(),
        })
    } else {
        result.Status.Conditions = append(result.Status.Conditions, check.DiagnosticCondition{
            Type:               "Configured",
            Status:             check.ConditionFalse,
            Reason:             "ConfigInvalid",
            Message:            msg,
            LastTransitionTime: metav1.Now(),
        })
    }

    return result
}
```

## Testing Guidelines

### Unit Test Example

```go
func TestCheck_Run(t *testing.T) {
    g := NewWithT(t)

    check := &Check{}
    target := &check.CheckTarget{
        // setup target
    }

    result := check.Run(context.Background(), target)

    // Validate metadata
    g.Expect(result.Metadata.Group).To(Equal("components"))
    g.Expect(result.Metadata.Kind).To(Equal("mycomponent"))
    g.Expect(result.Metadata.Name).To(Equal("my-check"))

    // Validate spec
    g.Expect(result.Spec.Description).ToNot(BeEmpty())

    // Validate status
    g.Expect(result.Status.Conditions).ToNot(BeEmpty())
    g.Expect(result.Status.Conditions).To(HaveLen(1))

    // Validate condition
    condition := result.Status.Conditions[0]
    g.Expect(condition.Type).To(Equal("Validated"))
    g.Expect(condition.Status).To(Equal(check.ConditionTrue))
    g.Expect(condition.Reason).To(Equal("RequirementsMet"))
    g.Expect(condition.Message).ToNot(BeEmpty())
}
```

## Table Rendering

Multi-condition diagnostics render as multiple table rows:

```
GROUP      KIND     NAME                 TYPE        STATUS  REASON              MESSAGE
services   auth     readiness-check      Available   True    ResourceFound       Authentication service deployment found
services   auth     readiness-check      Ready       True    PodsReady           All auth service pods are ready (3/3)
services   auth     readiness-check      Configured  True    ConfigValid         Authentication provider configuration is valid
```

## JSON/YAML Output

Structured output maintains full CR structure:

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

## Best Practices

1. **Always include at least one condition**: Empty conditions array is invalid
2. **Use standard condition types**: Validated, Available, Ready, Compatible, Configured, Authorized
3. **Order by execution sequence**: Maintain temporal order for reproducibility
4. **Keep messages concise**: <1024 bytes recommended
5. **Use CamelCase**: For Type and Reason fields
6. **Update LastTransitionTime correctly**: Only when Status changes
7. **Choose clear Reason values**: Machine-readable, specific, unique

## Next Steps

- Review [data-model.md](./data-model.md) for complete entity definitions
- See [research.md](./research.md) for Kubernetes metav1.Condition pattern details
- Check [contracts/diagnostic-result.yaml](./contracts/diagnostic-result.yaml) for OpenAPI schema
## Real-World Example Output

### Table Format (Default)

```bash
$ odh lint --target-version 3.0.0
```

```
Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.0.0

Assessing upgrade readiness: 2.25.0 → 3.0.0

UPGRADE READINESS: 2.25.0 → 3.0.0
=============================================================
┌──────────────────────────────────────────────────────────────────────────────────────────────┐
│ STATUS  GROUP       KIND          CHECK               SEVERITY  MESSAGE                      │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ ✗       component   kserve        serverless-removal  Critical  KServe serverless mode...    │
│ ✗       component   kueue         managed-removal     Critical  Kueue managed option...      │
│ ✗       component   modelmesh     removal             Critical  ModelMesh component...       │
│ ✗       component   codeflare     removal             Critical  CodeFlare is enabled...      │
│ ✗       service     servicemesh   removal             Critical  ServiceMesh is enabled...    │
│ ✓       dependency  cert-manager  installed           Info      cert-manager operator...     │
│ ✓       dependency  kueue         installed           Info      kueue-operator...            │
│ ✓       dependency  servicemesh   installed           Info      Service Mesh Operator...     │
└──────────────────────────────────────────────────────────────────────────────────────────────┘

Summary:
  Total: 8 | Passed: 3 | Failed: 5

⚠️  Recommendation: Address 5 blocking issue(s) before upgrading
```

### Table Format with Verbose (Shows Descriptions)

```bash
$ odh lint --target-version 3.0.0 --verbose
```

```
UPGRADE READINESS: 2.25.0 → 3.0.0
=============================================================
┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ STATUS  GROUP       KIND     CHECK       SEVERITY  MESSAGE                          DESCRIPTION                                                │
├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ ✗       component   kserve   serverless  Critical  KServe serverless enabled...     Validates that KServe serverless mode is disabled...      │
│ ✓       dependency  kueue    installed   Info      kueue-operator v1.1.0 installed  Reports the kueue-operator installation status...        │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### JSON Format

```bash
$ odh lint --target-version 3.0.0 -o json
```

```json
{
  "kind": "DiagnosticResultList",
  "metadata": {
    "clusterVersion": "2.25.0",
    "targetVersion": "3.0.0"
  },
  "items": [
    {
      "metadata": {
        "group": "component",
        "kind": "kserve",
        "name": "serverless-removal",
        "annotations": {
          "check.opendatahub.io/target-version": "3.0.0",
          "component.opendatahub.io/kserve-management-state": "Managed",
          "component.opendatahub.io/serving-management-state": "Managed"
        }
      },
      "spec": {
        "description": "Validates that KServe serverless mode is disabled before upgrading from RHOAI 2.x to 3.x (serverless support will be removed)"
      },
      "status": {
        "conditions": [
          {
            "type": "Compatible",
            "status": "False",
            "lastTransitionTime": "2025-12-12T07:38:25Z",
            "reason": "VersionIncompatible",
            "message": "KServe serverless mode is enabled (state: Managed) but will be removed in RHOAI 3.x"
          }
        ]
      }
    },
    {
      "metadata": {
        "group": "dependency",
        "kind": "cert-manager",
        "name": "installed",
        "annotations": {
          "operator.opendatahub.io/name": "cert-manager",
          "operator.opendatahub.io/version": "cert-manager.v1.16.5"
        }
      },
      "spec": {
        "description": "Reports the cert-manager operator installation status and version"
      },
      "status": {
        "conditions": [
          {
            "type": "Available",
            "status": "True",
            "lastTransitionTime": "2025-12-12T07:38:25Z",
            "reason": "ResourceFound",
            "message": "cert-manager operator installed: cert-manager.v1.16.5"
          }
        ]
      }
    }
  ]
}
```

### YAML Format

```bash
$ odh lint --target-version 3.0.0 -o yaml
```

```yaml
kind: DiagnosticResultList
metadata:
  clusterVersion: "2.25.0"
  targetVersion: "3.0.0"
items:
- metadata:
    group: component
    kind: kserve
    name: serverless-removal
    annotations:
      check.opendatahub.io/target-version: "3.0.0"
      component.opendatahub.io/kserve-management-state: Managed
      component.opendatahub.io/serving-management-state: Managed
  spec:
    description: Validates that KServe serverless mode is disabled before upgrading from RHOAI 2.x to 3.x (serverless support will be removed)
  status:
    conditions:
    - type: Compatible
      status: "False"
      lastTransitionTime: "2025-12-12T07:38:25Z"
      reason: VersionIncompatible
      message: KServe serverless mode is enabled (state: Managed) but will be removed in RHOAI 3.x
- metadata:
    group: dependency
    kind: cert-manager
    name: installed
    annotations:
      operator.opendatahub.io/name: cert-manager
      operator.opendatahub.io/version: cert-manager.v1.16.5
  spec:
    description: Reports the cert-manager operator installation status and version
  status:
    conditions:
    - type: Available
      status: "True"
      lastTransitionTime: "2025-12-12T07:38:25Z"
      reason: ResourceFound
      message: "cert-manager operator installed: cert-manager.v1.16.5"
```

### Filtering Examples

Filter by severity:
```bash
$ odh lint --target-version 3.0.0 --severity critical
# Shows only Critical findings
```

Filter by check pattern:
```bash
$ odh lint --checks "components/*"
# Shows only component checks

$ odh lint --checks "*kserve*"
# Shows only checks matching "kserve"
```

Fail on warnings:
```bash
$ odh lint --fail-on-warning
# Exit code 1 if any Warning findings detected
```

