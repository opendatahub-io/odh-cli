# Migrate Command Implementation Plan

## Overview

Implement a `migrate` subcommand for odh-cli that performs cluster migrations, specifically migrating from OpenShift AI's built-in Kueue to the Red Hat Build of Kueue (RHBOK) operator. The command will mirror the `lint` command architecture but add capabilities for cluster modification, dry-run mode, preparation, and user confirmations.

## Requirements Summary

### Flags
- `--dry-run`: Show what would be done without making changes
- `--prepare`: Run pre-flight checks AND backup existing Kueue resources
- `-y` / `--yes`: Skip confirmation prompts (default: ask for each step)
- Standard flags: `--output`, `--timeout`, `--verbose` (reuse from lint)

### RHBOK Migration Steps
1. Install Red Hat Build of Kueue Operator from OperatorHub
2. Update DataScienceCluster: set `spec.components.kueue.managementState` to `Unmanaged`
3. Verify ClusterQueue and LocalQueue resources are preserved

### User Experience
- Default: Ask confirmation before each major step
- With `--yes`: Execute all steps automatically
- With `--dry-run`: Show planned actions without executing
- With `--prepare`: Run validation + backup resources, don't execute migration

## Implementation Phases

### Phase 1: Core Action Infrastructure

Create the action system analogous to lint's check system:

#### 1.1 Action Interface and Types
**File:** `pkg/migrate/action/action.go`
```go
type Action interface {
    ID() string
    Name() string
    Description() string
    Group() ActionGroup
    CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool
    Validate(ctx context.Context, target *ActionTarget) (*result.ActionResult, error)
    Execute(ctx context.Context, target *ActionTarget) (*result.ActionResult, error)
}

type ActionTarget struct {
    Client         *client.Client
    CurrentVersion *version.ClusterVersion
    TargetVersion  *version.ClusterVersion
    DryRun         bool
    BackupPath     string
    SkipConfirm    bool
    IO             iostreams.Interface
}
```

**Key differences from Check interface:**
- `Execute()` performs cluster modifications (in addition to `Validate()`)
- ActionTarget includes DryRun, BackupPath, SkipConfirm, and IO for interaction
- `Validate()` is for pre-flight checks (non-destructive)

#### 1.2 Action Registry
**File:** `pkg/migrate/action/registry.go`
```go
type ActionRegistry struct {
    actions map[string]Action
}
```
Pattern matching via `ListByPattern()`, similar to CheckRegistry

**File:** `pkg/migrate/action/global.go`
Global registry with `MustRegisterAction()` for auto-registration

#### 1.3 Action Executor
**File:** `pkg/migrate/action/executor.go`
```go
type Executor struct {
    registry *ActionRegistry
}

func (e *Executor) ExecuteSelective(ctx, target, pattern, group) ([]ActionExecution, error)
```

**Critical:** Sequential execution only (no parallelism) for deterministic output and state management, mirroring lint's executor at `pkg/lint/check/executor.go:60-93`

#### 1.4 Action Results
**File:** `pkg/migrate/action/result/result.go`
```go
type ActionResult struct {
    Metadata struct {
        Group, Kind, Name string
        Annotations map[string]string
    }
    Spec struct {
        Description string
        DryRun bool
    }
    Status struct {
        Steps      []ActionStep
        Completed  bool
        Error      string
    }
}

type ActionStep struct {
    Name        string
    Description string
    Status      StepStatus  // Pending, Running, Completed, Failed, Skipped
    Message     string
    Timestamp   time.Time
}
```

### Phase 2: Command Framework

#### 2.1 Command Entry Point
**File:** `cmd/migrate/migrate.go`

Cobra command wrapper with:
- Blank imports for action auto-registration (e.g., `_ "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"`)
- Three-phase execution: Complete → Validate → Run
- Pattern from `cmd/lint/lint.go:83-140`

#### 2.2 Command Implementation
**File:** `pkg/cmd/migrate/migrate.go`

```go
type Command struct {
    *SharedOptions
    DryRun      bool
    Prepare     bool
    Yes         bool
    BackupPath  string
    MigrationID string
}

func (c *Command) AddFlags(fs *pflag.FlagSet)
func (c *Command) Complete() error
func (c *Command) Validate() error
func (c *Command) Run(ctx context.Context) error
```

**Run() flow:**
1. Detect current version via `version.Detect()`
2. Create ActionTarget with flags
3. Get global registry
4. If `--prepare`: run `runPrepareMode()` (validate + backup)
5. Else: run `runMigrationMode()` (execute actions)

#### 2.3 Shared Options
**File:** `pkg/cmd/migrate/shared_options.go`

Reuse pattern from `pkg/cmd/lint/shared_options.go`:
- IO streams
- ConfigFlags
- OutputFormat, Verbose, Timeout
- Client creation

### Phase 3: User Interaction Utilities

#### 3.1 Confirmation Prompts
**File:** `pkg/util/confirmation/confirmation.go`

```go
func Prompt(io iostreams.Interface, message string) bool {
    fmt.Fprintf(io.ErrOut(), "%s [y/N]: ", message)
    reader := bufio.NewReader(io.In())
    response, _ := reader.ReadString('\n')
    response = strings.TrimSpace(strings.ToLower(response))
    return response == "y" || response == "yes"
}
```

**Usage in actions:**
```go
if !target.SkipConfirm {
    if !confirmation.Prompt(target.IO, "Proceed with operator installation?") {
        step.Status = result.StepSkipped
        step.Message = "User cancelled"
        return step
    }
}
```

### Phase 4: RHBOK Migration Action

#### 4.1 Main Action Implementation
**File:** `pkg/migrate/actions/kueue/rhbok/rhbok.go`

```go
type RHBOKMigrationAction struct{}

func (a *RHBOKMigrationAction) ID() string {
    return "kueue.rhbok.migrate"
}

func (a *RHBOKMigrationAction) CanApply(currentVersion, targetVersion *semver.Version) bool {
    if currentVersion == nil || targetVersion == nil {
        return false
    }
    return currentVersion.Major == 2 && targetVersion.Major >= 3
}

func (a *RHBOKMigrationAction) Validate(ctx, target) (*result.ActionResult, error) {
    // Pre-flight checks:
    // 1. Verify current Kueue state
    // 2. Check for RHBOK conflicts
    // 3. Verify Kueue resources exist
    // 4. Verify RBAC permissions
}

func (a *RHBOKMigrationAction) Execute(ctx, target) (*result.ActionResult, error) {
    result := result.New("migration", "rhbok", "execute", "Execute RHBOK migration")

    // Step 1: Install RHBOK Operator
    step1 := a.installRHBOKOperator(ctx, target)
    result.Status.Steps = append(result.Status.Steps, step1)
    if step1.Status == result.StepFailed {
        return result, fmt.Errorf("failed: %s", step1.Message)
    }

    // Step 2: Update DataScienceCluster
    step2 := a.updateDataScienceCluster(ctx, target)
    result.Status.Steps = append(result.Status.Steps, step2)
    if step2.Status == result.StepFailed {
        return result, fmt.Errorf("failed: %s", step2.Message)
    }

    // Step 3: Verify resources preserved
    step3 := a.verifyResourcesPreserved(ctx, target)
    result.Status.Steps = append(result.Status.Steps, step3)

    result.Status.Completed = true
    return result, nil
}
```

#### 4.2 Install RHBOK Operator
**Method:** `installRHBOKOperator(ctx, target) ActionStep`

1. Check if dry-run: return skipped step with message
2. If not `--yes`: prompt for confirmation
3. Create Subscription resource:
   ```go
   subscription := &unstructured.Unstructured{
       Object: map[string]interface{}{
           "apiVersion": "operators.coreos.com/v1alpha1",
           "kind": "Subscription",
           "metadata": map[string]interface{}{
               "name": "kueue-operator",
               "namespace": "openshift-kueue-operator",
           },
           "spec": map[string]interface{}{
               "channel": "stable-v1.1",
               "name": "kueue-operator",
               "source": "redhat-operators",
               "sourceNamespace": "openshift-marketplace",
           },
       },
   }
   ```
4. Create via `target.Client.Dynamic.Resource(resources.Subscription.GVR()).Create()`
5. Wait for CSV to be ready
6. Return completed step

#### 4.3 Update DataScienceCluster
**Method:** `updateDataScienceCluster(ctx, target) ActionStep`

1. Check if dry-run: return skipped step
2. If not `--yes`: prompt for confirmation
3. Get DataScienceCluster via `target.Client.GetDataScienceCluster(ctx)`
4. Update managementState:
   ```go
   unstructured.SetNestedField(dsc.Object, "Unmanaged", "spec", "components", "kueue", "managementState")
   ```
5. Update via `target.Client.Dynamic.Resource(resources.DataScienceCluster.GVR()).Update()`
6. Return completed step

#### 4.4 Verify Resources Preserved
**Method:** `verifyResourcesPreserved(ctx, target) ActionStep`

1. List ClusterQueues via `target.Client.List()` with ClusterQueue GVR
2. List LocalQueues via `target.Client.List()` with LocalQueue GVR
3. Compare counts/names with backup or pre-migration state
4. Return completed step with count message

#### 4.5 Backup Functionality
**File:** `pkg/migrate/actions/kueue/rhbok/backup.go`

```go
func (a *RHBOKMigrationAction) BackupResources(ctx, target) error {
    timestamp := time.Now().Format("20060102-150405")

    // Backup ClusterQueues
    clusterQueues := /* list all */
    writeYAML(filepath.Join(target.BackupPath, fmt.Sprintf("clusterqueues-%s.yaml", timestamp)), clusterQueues)

    // Backup LocalQueues
    localQueues := /* list all */
    writeYAML(filepath.Join(target.BackupPath, fmt.Sprintf("localqueues-%s.yaml", timestamp)), localQueues)

    // Backup DataScienceCluster
    dsc := /* get DSC */
    writeYAML(filepath.Join(target.BackupPath, fmt.Sprintf("datasciencecluster-%s.yaml", timestamp)), dsc)
}
```

#### 4.6 Pre-flight Checks
**File:** `pkg/migrate/actions/kueue/rhbok/preflight.go`

```go
func (a *RHBOKMigrationAction) checkCurrentKueueState(ctx, target) ActionStep
func (a *RHBOKMigrationAction) checkNoRHBOKConflicts(ctx, target) ActionStep
func (a *RHBOKMigrationAction) verifyKueueResources(ctx, target) ActionStep
func (a *RHBOKMigrationAction) verifyPermissions(ctx, target) ActionStep
```

### Phase 5: Resource Types

#### 5.1 Add Kueue Resource Types
**File:** `pkg/resources/types.go`

Add to existing vars:
```go
ClusterQueue = ResourceType{
    Group:    "kueue.x-k8s.io",
    Version:  "v1beta1",
    Kind:     "ClusterQueue",
    Resource: "clusterqueues",
}

LocalQueue = ResourceType{
    Group:    "kueue.x-k8s.io",
    Version:  "v1beta1",
    Kind:     "LocalQueue",
    Resource: "localqueues",
}

InstallPlan = ResourceType{
    Group:    "operators.coreos.com",
    Version:  "v1alpha1",
    Kind:     "InstallPlan",
    Resource: "installplans",
}
```

### Phase 6: Command Registration

#### 6.1 Register Command
**File:** `cmd/main.go`

Add after lint registration:
```go
import (
    "github.com/opendatahub-io/odh-cli/cmd/migrate"
)

func main() {
    // ...
    lint.AddCommand(cmd, flags)
    migrate.AddCommand(cmd, flags)  // ADD THIS
    // ...
}
```

## Critical Implementation Details

### Sequential Execution Requirement
Actions MUST execute sequentially (never parallel) for:
- **State dependencies**: Step 2 depends on Step 1's cluster modifications
- **Determinism**: Reproducible output for debugging
- **Error handling**: Clear identification of which step failed
- Pattern: `pkg/lint/check/executor.go:60-93`

### Dry-Run Pattern
```go
if target.DryRun {
    step.Status = result.StepSkipped
    step.Message = "DRY RUN: Would create Subscription kueue-operator"
    return step
}
// ... actual execution
```

### Fail-Fast Error Handling
Stop immediately on step failure:
```go
if step1.Status == result.StepFailed {
    return result, fmt.Errorf("migration halted: %s", step1.Message)
}
```

### Auto-Registration Pattern
```go
// In rhbok.go
func init() {
    action.MustRegisterAction(&RHBOKMigrationAction{})
}

// In cmd/migrate/migrate.go
import (
    _ "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
)
```

## User Experience Examples

### List available migrations
```bash
$ kubectl odh migrate list --target-version 3.0.0

ID                      NAME                          APPLICABLE  DESCRIPTION
kueue.rhbok.migrate     Migrate Kueue to RHBOK        Yes         Migrates from OpenShift AI built-in Kueue...
```

### Default (with confirmations)
```bash
$ kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.0.0
Preparing migration: kueue.rhbok.migrate

Migration Steps:
1. Install Red Hat Build of Kueue Operator
2. Update DataScienceCluster Kueue managementState to Unmanaged
3. Verify ClusterQueue and LocalQueue resources preserved

[Step 1/3] Installing RHBOK Operator...
About to install Red Hat Build of Kueue Operator
Proceed with operator installation? [y/N]: y
✓ RHBOK operator installed successfully

[Step 2/3] Updating DataScienceCluster...
About to update DataScienceCluster Kueue managementState to Unmanaged
Proceed with configuration update? [y/N]: y
✓ DataScienceCluster updated successfully

[Step 3/3] Verifying resources preserved...
✓ All 3 ClusterQueues preserved
✓ All 5 LocalQueues preserved

Migration completed successfully!
```

### With --yes flag
```bash
$ kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.0.0
Running migration: kueue.rhbok.migrate (confirmations skipped)

[Step 1/3] Installing RHBOK Operator...
✓ RHBOK operator installed successfully

[Step 2/3] Updating DataScienceCluster...
✓ DataScienceCluster updated successfully

[Step 3/3] Verifying resources preserved...
✓ All 3 ClusterQueues preserved

Migration completed successfully!
```

### With --dry-run
```bash
$ kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0 --dry-run

DRY RUN MODE: No changes will be made to the cluster

[Step 1/3] Install RHBOK Operator
  → Would create Subscription kueue-operator in openshift-kueue-operator
  → Would wait for CSV kueue-operator.v1.x.x to be ready

[Step 2/3] Update DataScienceCluster
  → Would set spec.components.kueue.managementState=Unmanaged

[Step 3/3] Verify resources preserved
  → Would check 3 ClusterQueues
  → Would check 5 LocalQueues

DRY RUN complete. Use --yes to execute without prompts.
```

### With --prepare
```bash
$ kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0 --prepare

Running pre-flight checks for migration: kueue.rhbok.migrate

Pre-flight Validation:
✓ Current Kueue state verified
✓ No RHBOK conflicts detected
✓ Kueue resources found: 3 ClusterQueues, 5 LocalQueues
✓ Sufficient permissions verified

Backing up Kueue resources to ./backups...
✓ Backed up 3 ClusterQueues to ./backups/clusterqueues-20251212-153045.yaml
✓ Backed up 5 LocalQueues to ./backups/localqueues-20251212-153045.yaml
✓ Backed up DataScienceCluster to ./backups/datasciencecluster-20251212-153045.yaml

Preparation complete. Run without --prepare to execute migration.
```

### With multiple migrations
```bash
$ kubectl odh migrate run --migration kueue.rhbok.migrate --migration other.migration --target-version 3.0.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.0.0

=== Migration 1/2: kueue.rhbok.migrate ===
Running migration: kueue.rhbok.migrate (confirmations skipped)

[Step 1/3] Installing RHBOK Operator...
✓ RHBOK operator installed successfully

[Step 2/3] Updating DataScienceCluster...
✓ DataScienceCluster updated successfully

[Step 3/3] Verifying resources preserved...
✓ All 3 ClusterQueues preserved

Migration kueue.rhbok.migrate completed successfully!

=== Migration 2/2: other.migration ===
Running migration: other.migration (confirmations skipped)

[Step 1/1] Performing action...
✓ Action completed

Migration other.migration completed successfully!

All migrations completed successfully!
```

## File Checklist

### New Files (16 total)
1. `cmd/migrate/migrate.go` - Command entry point
2. `pkg/cmd/migrate/migrate.go` - Command implementation
3. `pkg/cmd/migrate/shared_options.go` - Shared options
4. `pkg/migrate/action/action.go` - Action interface
5. `pkg/migrate/action/registry.go` - Action registry
6. `pkg/migrate/action/global.go` - Global registry
7. `pkg/migrate/action/executor.go` - Sequential executor
8. `pkg/migrate/action/result/result.go` - Result types
9. `pkg/migrate/actions/kueue/rhbok/rhbok.go` - RHBOK action
10. `pkg/migrate/actions/kueue/rhbok/backup.go` - Backup logic
11. `pkg/migrate/actions/kueue/rhbok/preflight.go` - Pre-flight checks
12. `pkg/util/confirmation/confirmation.go` - User confirmation
13. `pkg/cmd/migrate/migrate_test.go` - Command tests
14. `pkg/migrate/action/executor_test.go` - Executor tests
15. `pkg/migrate/actions/kueue/rhbok/rhbok_test.go` - Action tests
16. `pkg/util/confirmation/confirmation_test.go` - Confirmation tests

### Modified Files (2 total)
1. `cmd/main.go` - Register migrate command
2. `pkg/resources/types.go` - Add ClusterQueue, LocalQueue, InstallPlan

## Testing Strategy

### Unit Tests
- Action tests: Validate(), Execute() with mocked client
- Dry-run tests: Verify no state changes
- Confirmation tests: All prompt scenarios
- Coverage target: >80%

### Integration Tests
- Full migration flow with test cluster
- Error scenarios (missing resources, permissions)
- Idempotency (running migration twice)

## Key Architectural Decisions

1. **Sequential execution**: Required for state dependencies and determinism
2. **Separate Validate/Execute**: Safety - validate before destructive operations
3. **Confirmation prompts**: Default to safe (ask), with --yes for automation
4. **Fail-fast errors**: Stop on first failure to avoid inconsistent state
5. **Backup in --prepare**: Separate safety measures from execution

## References

- Lint architecture: `docs/lint/architecture.md`
- Lint command: `cmd/lint/lint.go`, `pkg/cmd/lint/lint.go`
- Check executor: `pkg/lint/check/executor.go`
- Existing Kueue checks: `pkg/lint/checks/components/kueue/kueue.go`
- RHBOK migration docs: Red Hat OpenShift AI 2.25 documentation
