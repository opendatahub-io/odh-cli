package aipipelines

import (
	"context"
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// PreUpgradeCheckAction captures DSPA pod health, migrates v1alpha1→v1, and detects RBAC gaps.
type PreUpgradeCheckAction struct{}

func (a *PreUpgradeCheckAction) ID() string          { return preUpgradeCheckID }
func (a *PreUpgradeCheckAction) Name() string        { return preUpgradeCheckName }
func (a *PreUpgradeCheckAction) Description() string { return preUpgradeCheckDescription }

func (a *PreUpgradeCheckAction) Group() action.ActionGroup { return action.GroupMigration }
func (a *PreUpgradeCheckAction) Phase() action.ActionPhase { return action.PhasePreUpgrade }

func (a *PreUpgradeCheckAction) CanApply(target action.Target) bool {
	return target.CurrentVersion != nil && target.CurrentVersion.Major == 2
}

func (a *PreUpgradeCheckAction) Prepare() action.Task { return &preUpgradeCheckPrepareTask{} }
func (a *PreUpgradeCheckAction) Run() action.Task     { return &preUpgradeCheckRunTask{} }

// --- Prepare task: capture pod health state, detect v1alpha1, detect RBAC gaps ---

type preUpgradeCheckPrepareTask struct{}

func (t *preUpgradeCheckPrepareTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	validateTarget := target
	validateTarget.DryRun = true

	return t.Execute(ctx, validateTarget)
}

func (t *preUpgradeCheckPrepareTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	t.discover(ctx, target, recorder)

	return recorder.Build(), nil
}

func (t *preUpgradeCheckPrepareTask) discover(
	ctx context.Context,
	target action.Target,
	recorder action.RootRecorder,
) {
	// Step 1: Capture pod health state for v1 DSPAs
	captureAndSavePodHealth(ctx, target.Client, recorder, target.DryRun, false)

	// Step 2: Detect v1alpha1 DSPAs by checking CRD storedVersions
	v1alpha1Step := recorder.Child("detect-v1alpha1", "Detect deprecated v1alpha1 DSPAs")

	needsMigration, err := hasV1Alpha1StoredVersion(ctx, target.Client)
	if err != nil {
		v1alpha1Step.Completef(result.StepFailed, "Failed to check CRD stored versions: %v", err)

		return
	}

	if !needsMigration {
		v1alpha1Step.Completef(result.StepCompleted, "No deprecated v1alpha1 DSPAs found")
	} else {
		v1alpha1Step.Completef(result.StepCompleted, "v1alpha1 is in CRD storedVersions — migration required")
	}

	// Step 3: Detect custom roles needing RBAC update
	_, _ = findRolesNeedingFix(ctx, target.Client, recorder)
}

// --- Run task: migrate v1alpha1 DSPAs to v1 ---

type preUpgradeCheckRunTask struct{}

func (t *preUpgradeCheckRunTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	step := recorder.Child("check-v1alpha1", "Check for v1alpha1 DSPAs")

	needsMigration, err := hasV1Alpha1StoredVersion(ctx, target.Client)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to check CRD stored versions: %v", err)

		return recorder.Build(), nil
	}

	if !needsMigration {
		step.Completef(result.StepCompleted, "No v1alpha1 DSPAs found — nothing to migrate")
	} else {
		step.Completef(result.StepCompleted, "v1alpha1 is in CRD storedVersions — migration needed")
	}

	return recorder.Build(), nil
}

func (t *preUpgradeCheckRunTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	// Capture pod health state so post-upgrade-check can compare even if
	// the user did not run "migrate prepare" separately.
	captureAndSavePodHealth(ctx, target.Client, recorder, target.DryRun, true)

	if err := migrateAllDSPAsToV1(ctx, target.Client, recorder, migrateOpts{DryRun: target.DryRun}); err != nil {
		return recorder.Build(), fmt.Errorf("v1alpha1 migration failed: %w", err)
	}

	return recorder.Build(), nil
}

// captureAndSavePodHealth captures DSPA pod health state and persists it to disk.
// When skipIfExists is true, it skips capture if a state file already exists (used
// by the run task to avoid overwriting state saved by a prior "migrate prepare").
func captureAndSavePodHealth(
	ctx context.Context,
	c client.Client,
	recorder action.RootRecorder,
	dryRun bool,
	skipIfExists bool,
) {
	statePath := defaultStatePath()

	if skipIfExists {
		if _, err := loadPodHealthState(statePath); err == nil {
			recorder.Recordf("pod-health-state", "Pre-upgrade pod health state already exists at %s", result.StepCompleted, statePath)

			return
		}
	}

	healthStep := recorder.Child("capture-pod-health", "Capture pre-upgrade DSPA pod health")

	v1DSPAs, err := listDSPAs(ctx, c, resources.DataSciencePipelinesApplicationV1)
	if err != nil {
		healthStep.Completef(result.StepFailed, "Failed to list v1 DSPAs: %v", err)

		return
	}

	var state PodHealthState

	if len(v1DSPAs) == 0 {
		healthStep.Completef(result.StepCompleted, "No v1 DSPAs found")

		state = PodHealthState{
			CapturedAt: time.Now().UTC().Format(time.RFC3339),
			DSPAs:      []DSPAState{},
		}
	} else {
		var captureErr error

		state, captureErr = capturePodHealth(ctx, c, healthStep, v1DSPAs)
		if captureErr != nil {
			healthStep.Completef(result.StepFailed, "Failed to capture pod health: %v", captureErr)

			return
		}

		healthStep.Completef(result.StepCompleted, "Captured pod health for %d DSPA(s)", len(v1DSPAs))
	}

	saveStep := recorder.Child("save-state", "Save pre-upgrade state")

	if dryRun {
		saveStep.Completef(result.StepSkipped, "Would save pre-upgrade pod health state")

		return
	}

	if err := savePodHealthState(state, statePath); err != nil {
		saveStep.Completef(result.StepFailed, "Failed to save state: %v", err)

		return
	}

	saveStep.Completef(result.StepCompleted, "State saved to %s", statePath)
}
