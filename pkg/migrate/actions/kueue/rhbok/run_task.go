package rhbok

import (
	"context"
	"errors"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
)

type runTask struct {
	action *RHBOKMigrationAction
}

func (t *runTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	t.action.verifyRBAC(ctx, target, runPermissions())
	t.action.checkCertManager(ctx, target)
	t.action.checkCurrentKueueState(ctx, target)
	t.action.checkNoRHBOKConflicts(ctx, target)
	t.action.checkOperatorChannel(ctx, target)
	t.action.verifyKueueResources(ctx, target)
	t.action.reportLabelingPlan(ctx, target)

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}

func (t *runTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	validationResult, err := t.Validate(ctx, target)
	if err != nil {
		return nil, err
	}

	if hasFailedStep(validationResult.Status.Steps) {
		return validationResult, errPreflightFailed
	}

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	if t.action.isMigrationComplete(ctx, target) {
		step := target.Recorder.Child(
			"migration-complete",
			"Check if migration already complete",
		)
		step.Completef(result.StepSkipped, "RHBOK migration already complete (Unmanaged + operator installed)")
	} else {
		t.executeMigration(ctx, target)
	}

	return rootRecorder.Build(), nil
}

var errPreflightFailed = errors.New("preflight checks failed")

func (t *runTask) executeMigration(ctx context.Context, target action.Target) {
	t.action.clearExecuteLabelingPlan()

	state, err := t.action.getKueueManagementState(ctx, target.Client)
	if err != nil {
		target.Recorder.Child("get-kueue-state", "Get Kueue managementState").
			Completef(result.StepFailed, "Failed: %v", err)

		return
	}

	if state == constants.ManagementStateManaged {
		t.action.preserveKueueConfig(ctx, target)
		if haltIfStepFailed(target, "preserve-kueue-config") {
			return
		}
	}

	if state == constants.ManagementStateManaged && !t.action.SkipRemoveEmbedded {
		t.action.removeEmbeddedKueue(ctx, target)
		if haltIfStepFailed(target, "remove-embedded-kueue") {
			return
		}

		t.action.waitForEmbeddedRemoval(ctx, target)
		if haltIfStepFailed(target, "wait-embedded-removal") {
			return
		}
	}

	t.action.deleteLegacyCRDs(ctx, target)
	if haltIfStepFailed(target, "delete-legacy-crds") {
		return
	}

	t.action.installRHBOKOperator(ctx, target)
	if haltIfStepFailed(target, "install-rhbok-operator") {
		return
	}

	if state != constants.ManagementStateUnmanaged {
		t.action.activateRHBOK(ctx, target)
		if haltIfStepFailed(target, "activate-rhbok") {
			return
		}
	}

	t.action.waitForKueueReady(ctx, target)
	if haltIfStepFailed(target, "wait-kueue-ready") {
		return
	}

	t.action.labelKueueNamespaces(ctx, target)
	if haltIfStepFailed(target, "label-kueue-namespaces") {
		return
	}

	t.action.labelKueueWorkloads(ctx, target)
	if haltIfStepFailed(target, "label-kueue-workloads") {
		return
	}

	t.action.verifyMigrationComplete(ctx, target)
	t.action.verifyResourcesPreserved(ctx, target)
}

func haltIfStepFailed(target action.Target, stepName string) bool {
	root, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return false
	}

	if !stepTreeFailed(root.Build().Status.Steps, stepName) {
		return false
	}

	target.Recorder.Child("migration-halted", "Halt migration after step failure").
		Completef(result.StepFailed, "Migration halted: step %q failed", stepName)

	return true
}

func stepTreeFailed(steps []result.ActionStep, stepName string) bool {
	step := findActionStep(steps, stepName)
	if step == nil {
		return false
	}

	if step.Status == result.StepFailed {
		return true
	}

	return hasFailedStep(step.Children)
}

func hasFailedStep(steps []result.ActionStep) bool {
	for _, step := range steps {
		if step.Status == result.StepFailed {
			return true
		}

		if hasFailedStep(step.Children) {
			return true
		}
	}

	return false
}

func findActionStep(steps []result.ActionStep, name string) *result.ActionStep {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}

		if found := findActionStep(steps[i].Children, name); found != nil {
			return found
		}
	}

	return nil
}
