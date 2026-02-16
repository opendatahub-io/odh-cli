---
name: lint-check
description: Create a new lint check for the odh-cli lint command
---

# Lint Check Creation Skill

This skill streamlines creating new lint checks for `kubectl odh lint`.

## Required Information

Before implementing, gather the following from the user:

### 1. Check Classification
- **Group**: component | service | workload | dependency
- **Kind**: The specific target (e.g., kserve, dashboard, certmanager, ray)
- **Check Type**: Use a constant from `check.CheckType*` when applicable, or a package-level string constant for custom types

### 2. Check Metadata
- **Description**: What does this check validate?
- **Remediation** (optional): How to fix the detected issue?

### 3. Version Applicability
- When does this check apply? Examples:
  - "All versions" → `return true, nil`
  - "Upgrading to 3.x" → `version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)`
  - "3.x and above" → `version.IsVersion3x(target.CurrentVersion) || version.IsVersion3x(target.TargetVersion)`

### 4. Validation Logic
- What should the check actually validate?
- What resources need to be queried?
- What conditions indicate success vs failure?
- Is the failure blocking or advisory?

## Auto-Derived Values

From the gathered information:
- **ID**: `<group>.<kind>.<type>` (e.g., `components.kserve.serverless-removal`)
- **Name**: `<Group> :: <Kind> :: <Description>` (e.g., `Components :: KServe :: Serverless Removal (3.x)`)

## Validation Builders

Use the appropriate builder for each check group. These handle resource fetching, error handling, and annotation population automatically.

### Component — `validate.Component(c, target)`

For checks that validate DSC component configuration. `CheckKind()` must match the DSC spec key (e.g., `"kserve"`, `"dashboard"`, lowercase).

**Chainable methods:**
- `.InState(states...)` — only run when component has one of these management states; otherwise returns a "not configured" pass
- `.WithApplicationsNamespace()` — loads applications namespace from DSCI into `req.ApplicationsNamespace`

**Terminal methods:**
- `.Run(ctx, func(ctx, req *validate.ComponentRequest) error)` — full control over result
- `.Complete(ctx, func(ctx, req) ([]result.Condition, error))` — just return conditions

`ComponentRequest` fields: `Target` (embedded), `Result`, `DSC`, `ManagementState`, `ApplicationsNamespace`.

### Workloads — `validate.Workloads(c, target, resources.X)` / `validate.WorkloadsMetadata(c, target, resources.X)`

For checks that list and validate workload instances. Use `WorkloadsMetadata` when only name/namespace/labels/annotations/finalizers are needed; use `Workloads` when spec/status fields are required.

**Chainable methods:**
- `.Filter(func(item) (bool, error))` — keep only matching items

**Terminal methods:**
- `.Run(ctx, func(ctx, req *validate.WorkloadRequest[T]) error)` — full control
- `.Complete(ctx, func(ctx, req) ([]result.Condition, error))` — just return conditions

`WorkloadRequest[T]` fields: `Target` (embedded), `Result`, `Items []T`.

Auto-populates `ImpactedObjects` from filtered items if the callback does not set them. CRD not found is treated as an empty list (not an error).

### DSCI — `validate.DSCI(c, target)`

For service checks that validate DSCInitialization configuration.

**Terminal method:**
- `.Run(ctx, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error)` — note: callback has no `ctx` parameter

### Operator — `validate.Operator(c, target)`

For dependency checks that validate OLM operator presence.

**Chainable methods:**
- `.WithNames(names...)` — override subscription name matching (default: `c.CheckKind()`)
- `.WithChannels(channels...)` — restrict to specific channels
- `.WithConditionBuilder(func(found bool, version string) result.Condition)` — custom condition logic

**Terminal method:**
- `.Run(ctx)` — no callback; uses the condition builder

## Condition API

Create conditions with `check.NewCondition(conditionType, status, opts...)`:

```go
check.NewCondition(
    check.ConditionTypeCompatible,
    metav1.ConditionFalse,
    check.WithReason(check.ReasonVersionIncompatible),
    check.WithMessage("Feature X is enabled (state: %s) but removed in RHOAI %s", state, tv),
    check.WithImpact(result.ImpactBlocking),
    check.WithRemediation("Disable feature X before upgrading"),
)
```

**Options:**
- `check.WithReason(reason)` — condition reason (required, panics if empty)
- `check.WithMessage(format, args...)` — printf-style message
- `check.WithImpact(impact)` — override auto-derived impact
- `check.WithRemediation(text)` — actionable fix guidance

**Impact auto-derivation:**
- `Status=True` → `ImpactNone` (requirement met)
- `Status=False` → `ImpactAdvisory` (warning, upgrade CAN proceed)
- `Status=Unknown` → `ImpactAdvisory`

Use `check.WithImpact(result.ImpactBlocking)` explicitly for conditions that block upgrades.

### Standard Constants

**Condition Types:** `ConditionTypeValidated`, `ConditionTypeAvailable`, `ConditionTypeReady`, `ConditionTypeCompatible`, `ConditionTypeConfigured`, `ConditionTypeAuthorized`, `ConditionTypeMigrationRequired`

**Success Reasons:** `ReasonRequirementsMet`, `ReasonResourceFound`, `ReasonResourceAvailable`, `ReasonConfigurationValid`, `ReasonVersionCompatible`, `ReasonPermissionGranted`, `ReasonComponentRenamed`, `ReasonMigrationPending`, `ReasonNoMigrationRequired`

**Failure Reasons:** `ReasonResourceNotFound`, `ReasonResourceUnavailable`, `ReasonConfigurationInvalid`, `ReasonVersionIncompatible`, `ReasonPermissionDenied`, `ReasonQuotaExceeded`, `ReasonDependencyUnavailable`, `ReasonDeprecated`, `ReasonWorkloadsImpacted`, `ReasonFeatureRemoved`, `ReasonConfigurationUnmanaged`

**Unknown Reasons:** `ReasonCheckExecutionFailed`, `ReasonCheckSkipped`, `ReasonAPIAccessDenied`, `ReasonInsufficientData`

**Check Types:** `check.CheckTypeRemoval`, `check.CheckTypeInstalled`, `check.CheckTypeImpactedWorkloads`, `check.CheckTypeConfigMigration`, `check.CheckTypeAcceleratorProfileMigration` — or define a package-level `const checkType = "your-type"` for custom types.

**Annotations:** `check.AnnotationComponentManagementState`, `check.AnnotationCheckTargetVersion`, `check.AnnotationImpactedWorkloadCount`

## Implementation Instructions

After gathering information and receiving user approval:

### Step 0: Check for File Conflicts

**CRITICAL**: Before creating any files, check if they already exist:

1. Check if `pkg/lint/checks/<group>/<kind>/<type>.go` exists
2. Check if `pkg/lint/checks/<group>/<kind>/<type>_test.go` exists

**If any file exists**, ask the user:
- "File `<path>` already exists. What would you like to do?"
  - Overwrite the existing file
  - Choose a different check type name
  - Cancel the operation

**Do NOT proceed with file creation until conflicts are resolved.**

### Step 1: Create Check File

Create `pkg/lint/checks/<group>/<kind>/<type>.go` using the appropriate template for the check group.

#### Component Check Template

```go
package <kind>

import (
    "context"
    "fmt"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "github.com/opendatahub-io/odh-cli/pkg/constants"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
    "github.com/opendatahub-io/odh-cli/pkg/util/client"
    "github.com/opendatahub-io/odh-cli/pkg/util/components"
    "github.com/opendatahub-io/odh-cli/pkg/util/jq"
    "github.com/opendatahub-io/odh-cli/pkg/util/version"
)

type <CheckName>Check struct {
    check.BaseCheck
}

func New<CheckName>Check() *<CheckName>Check {
    return &<CheckName>Check{
        BaseCheck: check.BaseCheck{
            CheckGroup:       check.GroupComponent,
            Kind:             constants.Component<Kind>, // or a string literal like "kueue"
            Type:             check.CheckType<Type>,     // or a package-level const
            CheckID:          "components.<kind>.<type>",
            CheckName:        "Components :: <Kind> :: <Description>",
            CheckDescription: "<description>",
            CheckRemediation: "<remediation>",
        },
    }
}

func (c *<CheckName>Check) CanApply(ctx context.Context, target check.Target) (bool, error) {
    // Version logic based on user input
    return true, nil
}

func (c *<CheckName>Check) Validate(
    ctx context.Context,
    target check.Target,
) (*result.DiagnosticResult, error) {
    return validate.Component(c, target).
        Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
            tv := version.MajorMinorLabel(req.TargetVersion)

            // Use jq.Query to read DSC fields
            // val, err := jq.Query[string](req.DSC, ".spec.components.<kind>.someField")

            // Set condition on req.Result
            req.Result.SetCondition(check.NewCondition(
                check.ConditionTypeCompatible,
                metav1.ConditionTrue,
                check.WithReason(check.ReasonVersionCompatible),
                check.WithMessage("Ready for RHOAI %s upgrade", tv),
            ))

            return nil
        })
}
```

#### Workload Check Template

```go
package <kind>

import (
    "context"
    "fmt"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "github.com/opendatahub-io/odh-cli/pkg/lint/check"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
    "github.com/opendatahub-io/odh-cli/pkg/resources"
    "github.com/opendatahub-io/odh-cli/pkg/util/client"
    "github.com/opendatahub-io/odh-cli/pkg/util/components"
    "github.com/opendatahub-io/odh-cli/pkg/util/version"
)

type <CheckName>Check struct {
    check.BaseCheck
}

func New<CheckName>Check() *<CheckName>Check {
    return &<CheckName>Check{
        BaseCheck: check.BaseCheck{
            CheckGroup:       check.GroupWorkload,
            Kind:             "<kind>",
            Type:             check.CheckTypeImpactedWorkloads,
            CheckID:          "workloads.<kind>.<type>",
            CheckName:        "Workloads :: <Kind> :: <Description>",
            CheckDescription: "<description>",
            CheckRemediation: "<remediation>",
        },
    }
}

func (c *<CheckName>Check) CanApply(ctx context.Context, target check.Target) (bool, error) {
    // Version logic based on user input
    return true, nil
}

func (c *<CheckName>Check) Validate(
    ctx context.Context,
    target check.Target,
) (*result.DiagnosticResult, error) {
    return validate.WorkloadsMetadata(c, target, resources.<ResourceType>).
        Filter(func(item *metav1.PartialObjectMetadata) (bool, error) {
            // Optional: filter items by label, annotation, finalizer, etc.
            return true, nil
        }).
        Complete(ctx, func(_ context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) ([]result.Condition, error) {
            tv := version.MajorMinorLabel(req.TargetVersion)
            count := len(req.Items)

            if count == 0 {
                return []result.Condition{
                    check.NewCondition(
                        check.ConditionTypeCompatible,
                        metav1.ConditionTrue,
                        check.WithReason(check.ReasonVersionCompatible),
                        check.WithMessage("No impacted workloads found - ready for RHOAI %s", tv),
                    ),
                }, nil
            }

            return []result.Condition{
                check.NewCondition(
                    check.ConditionTypeCompatible,
                    metav1.ConditionFalse,
                    check.WithReason(check.ReasonWorkloadsImpacted),
                    check.WithMessage("Found %d impacted workload(s) for RHOAI %s", count, tv),
                    check.WithImpact(result.ImpactBlocking),
                    check.WithRemediation(c.CheckRemediation),
                ),
            }, nil
        })
}
```

#### Service Check Template

```go
package <kind>

import (
    "context"
    "errors"
    "fmt"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

    "github.com/opendatahub-io/odh-cli/pkg/constants"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
    "github.com/opendatahub-io/odh-cli/pkg/util/jq"
    "github.com/opendatahub-io/odh-cli/pkg/util/version"
)

type <CheckName>Check struct {
    check.BaseCheck
}

func New<CheckName>Check() *<CheckName>Check {
    return &<CheckName>Check{
        BaseCheck: check.BaseCheck{
            CheckGroup:       check.GroupService,
            Kind:             "<kind>",
            Type:             check.CheckTypeRemoval,
            CheckID:          "services.<kind>.<type>",
            CheckName:        "Services :: <Kind> :: <Description>",
            CheckDescription: "<description>",
            CheckRemediation: "<remediation>",
        },
    }
}

func (c *<CheckName>Check) CanApply(_ context.Context, target check.Target) (bool, error) {
    return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *<CheckName>Check) Validate(
    ctx context.Context,
    target check.Target,
) (*result.DiagnosticResult, error) {
    tv := version.MajorMinorLabel(target.TargetVersion)

    return validate.DSCI(c, target).Run(ctx, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
        state, err := jq.Query[string](dsci, ".spec.<kind>.managementState")

        switch {
        case errors.Is(err, jq.ErrNotFound):
            dr.SetCondition(check.NewCondition(
                check.ConditionTypeConfigured,
                metav1.ConditionFalse,
                check.WithReason(check.ReasonResourceNotFound),
                check.WithMessage("<Kind> is not configured in DSCInitialization"),
            ))
        case err != nil:
            return fmt.Errorf("querying <kind> managementState: %w", err)
        case state == constants.ManagementStateManaged:
            dr.SetCondition(check.NewCondition(
                check.ConditionTypeCompatible,
                metav1.ConditionFalse,
                check.WithReason(check.ReasonVersionIncompatible),
                check.WithMessage("<Kind> is enabled (state: %s) but removed in RHOAI %s", state, tv),
                check.WithImpact(result.ImpactBlocking),
                check.WithRemediation(c.CheckRemediation),
            ))
        default:
            dr.SetCondition(check.NewCondition(
                check.ConditionTypeCompatible,
                metav1.ConditionTrue,
                check.WithReason(check.ReasonVersionCompatible),
                check.WithMessage("<Kind> is disabled (state: %s) - ready for RHOAI %s upgrade", state, tv),
            ))
        }

        return nil
    })
}
```

#### Dependency Check Template

Dependency checks typically use `c.NewResult()` directly (no builder) or `validate.Operator()` for OLM checks.

```go
package <kind>

import (
    "context"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "github.com/opendatahub-io/odh-cli/pkg/lint/check"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
    "github.com/opendatahub-io/odh-cli/pkg/util/version"
)

type <CheckName>Check struct {
    check.BaseCheck
}

func New<CheckName>Check() *<CheckName>Check {
    return &<CheckName>Check{
        BaseCheck: check.BaseCheck{
            CheckGroup:       check.GroupDependency,
            Kind:             "<kind>",
            Type:             "<check-type>",
            CheckID:          "dependencies.<kind>.<type>",
            CheckName:        "Dependencies :: <Kind> :: <Description>",
            CheckDescription: "<description>",
        },
    }
}

func (c *<CheckName>Check) CanApply(_ context.Context, target check.Target) (bool, error) {
    return version.IsVersion3x(target.CurrentVersion) || version.IsVersion3x(target.TargetVersion), nil
}

func (c *<CheckName>Check) Validate(
    ctx context.Context,
    target check.Target,
) (*result.DiagnosticResult, error) {
    dr := c.NewResult()
    tv := version.MajorMinorLabel(target.TargetVersion)

    // Perform validation (e.g., version detection, API checks)
    // ...

    dr.SetCondition(check.NewCondition(
        check.ConditionTypeCompatible,
        metav1.ConditionTrue,
        check.WithReason(check.ReasonVersionCompatible),
        check.WithMessage("Dependency meets RHOAI %s requirements", tv),
    ))

    return dr, nil
}
```

For OLM-based dependency checks, use the Operator builder instead:

```go
func (c *<CheckName>Check) Validate(
    ctx context.Context,
    target check.Target,
) (*result.DiagnosticResult, error) {
    return validate.Operator(c, target).
        WithNames("operator-name-1", "operator-name-2").
        Run(ctx)
}
```

### Step 2: Register Check

Add to `pkg/lint/command.go` in the `NewCommand()` function:

```go
registry.MustRegister(<kind>.New<CheckName>Check())
```

Add the import if the package is new. Follow the existing registration order: Dependencies → Services → Components → Workloads.

### Step 3: Create Test File

Create `pkg/lint/checks/<group>/<kind>/<type>_test.go`:

```go
package <kind>_test

import (
    "testing"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"

    "github.com/opendatahub-io/odh-cli/pkg/lint/check"
    resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
    "github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
    "github.com/opendatahub-io/odh-cli/pkg/lint/checks/<group>/<kind>"
    "github.com/opendatahub-io/odh-cli/pkg/resources"

    . "github.com/onsi/gomega"
    . "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
    // Include every resource the check lists or fetches.
    // For component checks:
    resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
    // For workload checks, also include the workload resource:
    // resources.<WorkloadType>.GVR(): resources.<WorkloadType>.ListKind(),
}

func Test<CheckName>Check_PassCase(t *testing.T) {
    g := NewWithT(t)
    ctx := t.Context()

    target := testutil.NewTarget(t, testutil.TargetConfig{
        ListKinds:      listKinds,
        Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"<kind>": "Managed"})},
        CurrentVersion: "2.17.0",
        TargetVersion:  "3.0.0",
    })

    chk := <kind>.New<CheckName>Check()
    result, err := chk.Validate(ctx, target)

    g.Expect(err).ToNot(HaveOccurred())
    g.Expect(result.Status.Conditions).To(HaveLen(1))
    g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
        "Type":   Equal(check.ConditionTypeCompatible),
        "Status": Equal(metav1.ConditionTrue),
        "Reason": Equal(check.ReasonVersionCompatible),
    }))
}

func Test<CheckName>Check_FailCase(t *testing.T) {
    g := NewWithT(t)
    ctx := t.Context()

    // Set up objects that trigger failure
    target := testutil.NewTarget(t, testutil.TargetConfig{
        ListKinds:      listKinds,
        Objects:        []*unstructured.Unstructured{/* objects that cause failure */},
        CurrentVersion: "2.17.0",
        TargetVersion:  "3.0.0",
    })

    chk := <kind>.New<CheckName>Check()
    result, err := chk.Validate(ctx, target)

    g.Expect(err).ToNot(HaveOccurred())
    g.Expect(result.Status.Conditions).To(HaveLen(1))
    g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
        "Type":   Equal(check.ConditionTypeCompatible),
        "Status": Equal(metav1.ConditionFalse),
        "Reason": Equal(check.ReasonVersionIncompatible),
    }))
    g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func Test<CheckName>Check_CanApply(t *testing.T) {
    g := NewWithT(t)

    chk := <kind>.New<CheckName>Check()

    // Should not apply when versions are nil
    target := testutil.NewTarget(t, testutil.TargetConfig{ListKinds: listKinds})
    canApply, err := chk.CanApply(t.Context(), target)
    g.Expect(err).ToNot(HaveOccurred())
    g.Expect(canApply).To(BeFalse())

    // Should apply for expected version range
    target = testutil.NewTarget(t, testutil.TargetConfig{
        ListKinds:      listKinds,
        CurrentVersion: "2.17.0",
        TargetVersion:  "3.0.0",
    })
    canApply, err = chk.CanApply(t.Context(), target)
    g.Expect(err).ToNot(HaveOccurred())
    g.Expect(canApply).To(BeTrue())
}

func Test<CheckName>Check_Metadata(t *testing.T) {
    g := NewWithT(t)

    chk := <kind>.New<CheckName>Check()

    g.Expect(chk.ID()).To(Equal("<group>.<kind>.<type>"))
    g.Expect(chk.Name()).ToNot(BeEmpty())
    g.Expect(chk.Group()).To(Equal(check.Group<Group>))
    g.Expect(chk.Description()).ToNot(BeEmpty())
}
```

**Test helpers:**
- `testutil.NewTarget(t, cfg)` — builds a `check.Target` from fake clients
- `testutil.NewDSC(map[string]string{...})` — creates a DSC with component management states
- `testutil.NewDSCI(namespace)` — creates a DSCI with applications namespace

**`ListKinds` requirement:** Every resource the check lists must be registered in `ListKinds` (maps GVR to list kind string). Use `resources.X.GVR()` and `resources.X.ListKind()`.

**OLM dependency tests** use `operatorfake.NewSimpleClientset()` via `TargetConfig.OLM` instead of `ListKinds`/`Objects`.

### Step 4: Quality Checks

Run:
```bash
make fmt
make lint
make test
```

## Common Pitfalls

1. **Impact defaults to advisory** — `Status=False` auto-derives `ImpactAdvisory`. Use `check.WithImpact(result.ImpactBlocking)` explicitly for conditions that block upgrades.
2. **Impact validation panics** — `Status=False`/`Unknown` without an impact (`ImpactNone`) causes a panic in `NewCondition`. The auto-derivation handles this, but `WithImpact(result.ImpactNone)` on a failing condition will panic.
3. **`CheckKind()` must match DSC spec key** — For component checks, `Kind` must be the lowercase DSC component key (e.g., `"kserve"` not `"KServe"`). The `validate.Component` builder uses this to look up the management state.
4. **`InState()` returns a pass** — When the component is not in any of the specified states, the builder returns a passing "not configured" result, not a failure.
5. **CRD not found = empty list** — Workload builders treat CRD-not-found as an empty list (not an error). Your check should handle the zero-items case.
6. **DSCI callback has no `ctx`** — The `validate.DSCI` callback signature is `func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error` (no context parameter).
7. **Missing `ListKinds` in tests** — Every resource type the check lists must be registered in `ListKinds`, otherwise the fake client returns wrong list kinds. Workload checks that also read DSC in `CanApply` need both the workload GVR and `resources.DataScienceCluster` in `ListKinds`.
8. **Test package suffix** — Use `package <kind>_test` (external test package), not `package <kind>`.

## Critical Rules

1. **MUST check for file conflicts** — Before creating files, verify they don't exist. If they do, ask the user how to proceed
2. **MUST use `check.BaseCheck`** — From `pkg/lint/check`. Never implement ID/Name/Description/Group manually
3. **MUST use validation builders** — `validate.Component`, `validate.Workloads`, `validate.WorkloadsMetadata`, `validate.DSCI`, or `validate.Operator` as appropriate for the check group
4. **MUST use JQ queries** — `jq.Query[T]()` for field access. Never use `unstructured.Nested*()` methods
5. **MUST use `check.NewCondition`** — With appropriate `WithReason`, `WithMessage`, `WithImpact`, `WithRemediation` options
6. **MUST use constants** — From `pkg/lint/check/constants.go` and `pkg/lint/check/condition.go`
7. **MUST use centralized GVK** — From `pkg/resources/types.go`
8. **MUST register explicitly** — In `pkg/lint/command.go`, following the canonical group order
9. **MUST run quality checks** — `make fmt && make lint && make test`

## Reference Documentation

- Architecture: `docs/lint/architecture.md`
- Writing Checks: `docs/lint/writing-checks.md`
- Testing: `docs/testing.md`
- Coding Conventions: `docs/coding/conventions.md`
