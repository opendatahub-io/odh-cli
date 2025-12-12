# Data Model: Diagnostic Result CR Structure

**Feature**: 005-diagnostic-cr-structure
**Date**: 2025-12-10

## Entity Overview

This data model defines the structure for diagnostic check results following Kubernetes Custom Resource conventions.

## Entity Diagram

```
DiagnosticResult
├── Metadata (DiagnosticMetadata)
│   ├── Group: string
│   ├── Kind: string
│   ├── Name: string
│   └── Annotations: map[string]string
├── Spec (DiagnosticSpec)
│   └── Description: string
└── Status (DiagnosticStatus)
    └── Conditions: []DiagnosticCondition
        ├── Type: string
        ├── Status: ConditionStatus
        ├── Reason: string
        ├── Message: string
        └── LastTransitionTime: metav1.Time
```

## Entities

### DiagnosticResult

Primary entity representing a complete diagnostic check result.

**Fields**:

| Field | Type | Required | Description | Constraints |
|-------|------|----------|-------------|-------------|
| Metadata | DiagnosticMetadata | Yes | CR-like metadata identifying the check | Must contain valid Group, Kind, Name |
| Spec | DiagnosticSpec | Yes | Specification describing the check | Must contain non-empty Description |
| Status | DiagnosticStatus | Yes | Current status with conditions | Must contain at least one condition |

**Relationships**:
- Has one DiagnosticMetadata
- Has one DiagnosticSpec
- Has one DiagnosticStatus

**Validation Rules**:
- Status.Conditions array MUST NOT be empty
- Metadata.Group, Metadata.Kind, Metadata.Name MUST NOT be empty strings
- Spec.Description SHOULD be non-empty (warning if empty)

**Lifecycle**:
1. Created: Check execution begins
2. Populated: Metadata and Spec filled from check definition
3. Executed: Conditions added to Status as validation progresses
4. Completed: All conditions have final Status values
5. Rendered: Displayed as table rows or structured output

---

### DiagnosticMetadata

Metadata section identifying the diagnostic target and check.

**Fields**:

| Field | Type | Required | Description | Constraints |
|-------|------|----------|-------------|-------------|
| Group | string | Yes | Diagnostic target category | Non-empty, lowercase, examples: "components", "services", "workloads" |
| Kind | string | Yes | Specific target being checked | Non-empty, examples: "kserve", "auth", "cert-manager" |
| Name | string | Yes | Check identifier | Non-empty, kebab-case, examples: "version-compatibility", "configuration-valid" |
| Annotations | map[string]string | No | Key-value metadata | Keys must be domain-qualified (domain/key format) |

**Identity Rules**:
- Tuple (Group, Kind, Name) uniquely identifies a diagnostic result
- Same Name can exist across different Group/Kind combinations
- Within same Group+Kind, Name must be unique

**Standard Annotation Keys**:
- `check.opendatahub.io/source-version`: Current cluster version
- `check.opendatahub.io/target-version`: Target version for upgrade assessment

**Validation Rules**:
- Group MUST match pattern: `^[a-z][a-z0-9-]*$`
- Kind MUST match pattern: `^[a-z][a-z0-9-]*$`
- Name MUST match pattern: `^[a-z][a-z0-9-]*$`
- Annotation keys MUST match pattern: `^[a-z0-9.-]+/[a-z0-9.-]+$`

---

### DiagnosticSpec

Specification describing what the check validates.

**Fields**:

| Field | Type | Required | Description | Constraints |
|-------|------|----------|-------------|-------------|
| Description | string | Yes | Detailed explanation of check purpose | SHOULD be non-empty, <1024 characters recommended |

**Content Guidelines**:
- Explain WHAT is being validated
- Explain WHY it matters (impact of failure)
- Use clear, user-friendly language
- Avoid implementation details

**Examples**:
- `"Validates KServe component configuration and availability"`
- `"Checks service mesh control plane status and readiness"`
- `"Verifies authentication provider configuration and connectivity"`

---

### DiagnosticStatus

Status section containing condition-based validation results.

**Fields**:

| Field | Type | Required | Description | Constraints |
|-------|------|----------|-------------|-------------|
| Conditions | []DiagnosticCondition | Yes | Array of validation conditions | MUST contain at least one condition, ordered by execution sequence |

**Ordering**:
- Conditions MUST be ordered by check execution sequence (temporal order)
- This provides reproducibility and correlation with execution logs

**Validation Rules**:
- Array MUST NOT be empty (zero conditions is invalid)
- All conditions in array MUST be valid DiagnosticCondition instances

---

### DiagnosticCondition

Individual validation condition following Kubernetes metav1.Condition pattern.

**Fields**:

| Field | Type | Required | Description | Constraints |
|-------|------|----------|-------------|-------------|
| Type | string | Yes | Condition type identifier | Non-empty, CamelCase, examples: "Validated", "Available", "Compatible" |
| Status | ConditionStatus | Yes | Condition status | One of: "True", "False", "Unknown" |
| Reason | string | Yes | Machine-readable reason code | Non-empty, CamelCase, concise, examples: "RequirementsMet", "ResourceNotFound" |
| Message | string | Yes | Human-readable explanation | Can be empty, <1024 bytes recommended |
| LastTransitionTime | metav1.Time | Yes | Timestamp of last status change | Auto-managed, updated only when Status changes |

**Type Field Naming**:
- Use adjectives: "Ready", "Available", "Healthy"
- Use past-tense verbs: "Validated", "Reconciled", "Accepted"
- Avoid present-tense verbs: ~~"Validating"~~, ~~"Reconciling"~~
- Choose polarity that makes sense for humans (positive or negative)

**Status Semantics**:
- `True`: Condition is met/resolved (check passed, requirement satisfied)
- `False`: Condition not met (check failed, requirement not satisfied)
- `Unknown`: Insufficient information (check error, skipped, or pending)

**Reason Guidelines**:
- One-word CamelCase identifier
- Machine-readable, suitable for programmatic filtering
- Specific and unique to the situation
- Success examples: `RequirementsMet`, `ResourceAvailable`, `ConfigurationValid`
- Failure examples: `ResourceNotFound`, `QuotaExceeded`, `PermissionDenied`
- Unknown examples: `CheckSkipped`, `APIAccessDenied`, `InsufficientData`

**Message Guidelines**:
- Human-readable, detailed context
- Explain "why" not just "what"
- Include error details for failures
- Can be empty if Reason is self-explanatory
- Examples:
  - `"All ServiceMesh components properly configured and available"`
  - `"Failed to find CRD 'servicemeshcontrolplanes.maistra.io'"`
  - `"Unable to query resources: Forbidden (403)"`

**LastTransitionTime Rules**:
- MUST be updated when Status changes (True→False, False→Unknown, etc.)
- MUST NOT be updated when only Message or Reason changes
- Preserves historical transition information

---

### ConditionStatus

Enumeration for condition status values.

**Values**:

| Value | Meaning | Usage |
|-------|---------|-------|
| `True` | Condition met/resolved | Check passed, requirement satisfied, resource available |
| `False` | Condition not met | Check failed, requirement not satisfied, resource missing |
| `Unknown` | Cannot determine | Check error, skipped, or insufficient information |

**Type**: String enum (not Go constants for JSON compatibility)

## State Transitions

### Condition Status Lifecycle

```
          Check Start
              ↓
    ┌─────────────────┐
    │ No Conditions   │
    └────────┬────────┘
             │
             ↓
    ┌─────────────────┐
    │ Executing       │ ← Conditions added with Status
    │ Status: Unknown │
    └────────┬────────┘
             │
             ├──→ Success  ──→ Status: True
             ├──→ Failure  ──→ Status: False
             └──→ Error    ──→ Status: Unknown
```

### DiagnosticResult Lifecycle

1. **Initialization**: Empty DiagnosticResult created
2. **Metadata Population**: Group, Kind, Name, Annotations set
3. **Spec Population**: Description filled from check definition
4. **Execution**: Conditions added to Status.Conditions as checks run
5. **Completion**: All conditions have final Status (True/False/Unknown)
6. **Rendering**: Result displayed as table rows or structured output

## Data Volume Assumptions

**Scale targets**:
- 50+ diagnostic checks per execution
- 1-10 conditions per diagnostic check
- 100-500 total conditions per diagnostic run
- 1000+ conditions in large deployments

**Memory assumptions**:
- Average condition size: ~500 bytes (JSON)
- Average diagnostic result: ~2KB
- 100 diagnostics × 2KB = ~200KB
- Target: <100MB for largest diagnostic runs

## Validation Rules Summary

### Structural Validation

- DiagnosticResult MUST have non-nil Metadata, Spec, Status
- Metadata MUST have non-empty Group, Kind, Name
- Spec MUST have Description (non-empty recommended)
- Status MUST have non-empty Conditions array

### Content Validation

- Group, Kind, Name MUST be lowercase with hyphens
- Annotation keys MUST be domain-qualified
- Condition Type MUST be CamelCase
- Condition Status MUST be "True", "False", or "Unknown"
- Condition Reason MUST be non-empty CamelCase
- LastTransitionTime MUST be valid RFC3339 timestamp

### Business Rules

- Conditions array MUST NOT be empty (at least one condition required)
- Same Name can exist across different Group/Kind tuples
- Within same Group+Kind, Name should be unique (recommendation)
- Conditions ordered by execution sequence

## JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["metadata", "spec", "status"],
  "properties": {
    "metadata": {
      "type": "object",
      "required": ["group", "kind", "name"],
      "properties": {
        "group": {
          "type": "string",
          "pattern": "^[a-z][a-z0-9-]*$"
        },
        "kind": {
          "type": "string",
          "pattern": "^[a-z][a-z0-9-]*$"
        },
        "name": {
          "type": "string",
          "pattern": "^[a-z][a-z0-9-]*$"
        },
        "annotations": {
          "type": "object",
          "additionalProperties": {
            "type": "string"
          }
        }
      }
    },
    "spec": {
      "type": "object",
      "required": ["description"],
      "properties": {
        "description": {
          "type": "string"
        }
      }
    },
    "status": {
      "type": "object",
      "required": ["conditions"],
      "properties": {
        "conditions": {
          "type": "array",
          "minItems": 1,
          "items": {
            "type": "object",
            "required": ["type", "status", "reason", "message", "lastTransitionTime"],
            "properties": {
              "type": {
                "type": "string",
                "pattern": "^[A-Z][a-zA-Z0-9]*$"
              },
              "status": {
                "type": "string",
                "enum": ["True", "False", "Unknown"]
              },
              "reason": {
                "type": "string",
                "pattern": "^[A-Z][a-zA-Z0-9]*$"
              },
              "message": {
                "type": "string"
              },
              "lastTransitionTime": {
                "type": "string",
                "format": "date-time"
              }
            }
          }
        }
      }
    }
  }
}
```