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
- **Kind**: The specific target (e.g., kserve, dashboard, certmanager)
- **Check Type**: removal | deprecation | version-requirement | config-migration | installed | impacted-workloads

### 2. Check Metadata
- **Description**: What does this check validate?
- **Remediation** (optional): How to fix the detected issue?

### 3. Version Applicability
- When does this check apply? Examples:
  - "All versions" → `return true`
  - "Upgrading to 3.x" → `version.IsUpgradeFrom2xTo3x()`
  - "3.3 and above" → `version.IsVersionAtLeast(target.TargetVersion, 3, 3)`

### 4. Validation Logic
- What should the check actually validate?
- What resources need to be queried?
- What conditions indicate success vs failure?

## Auto-Derived Values

From the gathered information:
- **ID**: `<group>.<kind>.<type>` (e.g., `components.kserve.removal`)
- **Name**: `<Group> :: <Kind> :: <Description>` (e.g., `Components :: KServe :: Removal (3.x)`)

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

Create `pkg/lint/checks/<group>/<kind>/<type>.go`:

```go
package <kind>

import (
    "context"
    "fmt"

    apierrors "k8s.io/apimachinery/pkg/api/errors"

    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
    "github.com/lburgazzoli/odh-cli/pkg/util/jq"
    "github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type <CheckName>Check struct {
    base.BaseCheck
}

func New<CheckName>Check() *<CheckName>Check {
    return &<CheckName>Check{
        BaseCheck: base.BaseCheck{
            CheckGroup:       check.Group<Group>,
            Kind:             check.Component<Kind>, // or Service<Kind>, Dependency<Kind>
            CheckType:        check.CheckType<Type>,
            CheckID:          "<group>.<kind>.<type>",
            CheckName:        "<Group> :: <Kind> :: <Description>",
            CheckDescription: "<description>",
            CheckRemediation: "<remediation>", // optional
        },
    }
}

func (c *<CheckName>Check) CanApply(target check.Target) bool {
    // Version logic based on user input
}

func (c *<CheckName>Check) Validate(
    ctx context.Context,
    target check.Target,
) (*result.DiagnosticResult, error) {
    dr := c.NewResult()

    // Implementation using:
    // - target.Client.GetDataScienceCluster(ctx) for component checks
    // - target.Client.GetDSCInitialization(ctx) for service checks
    // - jq.Query[T]() for field access
    // - results.SetCompatibilitySuccessf() / results.SetCompatibilityFailuref()
    // - results.DataScienceClusterNotFound() for not-found handling

    return dr, nil
}
```

### Step 2: Register Check

Add to `pkg/lint/command.go` in the `NewCommand()` function:

```go
registry.MustRegister(<kind>.New<CheckName>Check())
```

And add the import if the package is new.

### Step 3: Create Test File

Create `pkg/lint/checks/<group>/<kind>/<type>_test.go`:

```go
package <kind>

import (
    "testing"

    . "github.com/onsi/gomega"
    "github.com/onsi/gomega/gstruct"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
)

func Test<CheckName>Check(t *testing.T) {
    g := NewWithT(t)

    t.Run("should pass when <success condition>", func(t *testing.T) {
        // Setup test data
        // Create fake client
        // Execute check
        // Assert results
    })

    t.Run("should fail when <failure condition>", func(t *testing.T) {
        // Setup test data
        // Create fake client
        // Execute check
        // Assert results
    })
}
```

### Step 4: Quality Checks

Run:
```bash
make fmt
make lint
make test
```

## Critical Rules

1. **MUST check for file conflicts** - Before creating files, verify they don't exist. If they do, ask the user how to proceed
2. **MUST use BaseCheck** - Never implement ID/Name/Description/Group manually
3. **MUST use JQ queries** - Never use `unstructured.Nested*()` methods
4. **MUST use result helpers** - From `pkg/lint/checks/shared/results/`
5. **MUST use constants** - From `pkg/lint/check/constants.go`
6. **MUST use centralized GVK** - From `pkg/resources/types.go`
7. **MUST register explicitly** - In `pkg/lint/command.go`
8. **MUST run quality checks** - `make fmt && make lint && make test`

## Reference Documentation

- Architecture: `docs/lint/architecture.md`
- Writing Checks: `docs/lint/writing-checks.md`
- Testing: `docs/testing.md`
- Coding Conventions: `docs/coding/conventions.md`