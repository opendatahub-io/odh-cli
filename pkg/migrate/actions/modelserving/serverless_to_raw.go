package modelserving

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

const (
	serverlessToRawActionID          = "modelserving.serverless-to-raw"
	serverlessToRawActionName        = "Convert Serverless InferenceServices to RawDeployment"
	serverlessToRawActionDescription = "Converts InferenceServices using Serverless deployment mode to RawDeployment and creates associated auth resources (ServiceAccount, Role, RoleBinding)"

	msgServerlessConfirm    = "About to convert %d InferenceService(s) from Serverless to RawDeployment"
	msgServerlessCancelled  = "User cancelled Serverless to RawDeployment conversion"
	msgServerlessComplete   = "Converted %d of %d InferenceService(s) from Serverless to RawDeployment"
	msgServerlessFailed     = "Failed to convert %d of %d InferenceService(s) from Serverless to RawDeployment"
	msgServerlessDryRun     = "Dry-run: would convert %d InferenceService(s) from Serverless to RawDeployment"
	msgServerlessBackupDone = "Backed up %d Serverless InferenceServices to %s"
	msgServerlessNoISVCs    = "No Serverless InferenceServices found"
)

// ServerlessToRawAction converts InferenceServices from Serverless to RawDeployment mode.
type ServerlessToRawAction struct{}

func (a *ServerlessToRawAction) ID() string {
	return serverlessToRawActionID
}

func (a *ServerlessToRawAction) Name() string {
	return serverlessToRawActionName
}

func (a *ServerlessToRawAction) Description() string {
	return serverlessToRawActionDescription
}

func (a *ServerlessToRawAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *ServerlessToRawAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func (a *ServerlessToRawAction) CanApply(target action.Target) bool {
	return target.CurrentVersion.Major == 2 && target.CurrentVersion.Minor >= 25
}

func (a *ServerlessToRawAction) Prepare() action.Task {
	return &serverlessToRawPrepareTask{action: a}
}

func (a *ServerlessToRawAction) Run() action.Task {
	return &serverlessToRawRunTask{action: a}
}

func (a *ServerlessToRawAction) convertISVCs(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"convert-serverless-to-raw",
		"Convert Serverless InferenceServices to RawDeployment",
	)

	isvcs, err := listISVCsByDeploymentMode(ctx, target, deploymentModeServerless)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Serverless InferenceServices: %v", err)

		return
	}

	if len(isvcs) == 0 {
		step.Completef(result.StepSkipped, msgServerlessNoISVCs)

		return
	}

	step.Recordf("list-isvcs", msgFoundISVCs, result.StepCompleted, len(isvcs), deploymentModeServerless)

	// Confirm with user
	if !target.SkipConfirm && !target.DryRun {
		target.IO.Fprintln()
		target.IO.Errorf(msgServerlessConfirm, len(isvcs))

		if !confirmation.Prompt(target.IO, "Proceed with conversion?") {
			step.Completef(result.StepSkipped, msgServerlessCancelled)

			return
		}
	}

	convertedCount := 0
	failedCount := 0

	for _, isvc := range isvcs {
		isvcStep := step.Child(
			fmt.Sprintf("convert-%s-%s", isvc.GetNamespace(), isvc.GetName()),
			fmt.Sprintf("Convert %s/%s", isvc.GetNamespace(), isvc.GetName()),
		)

		if err := deleteAndRecreateISVC(ctx, target, isvc, isvcStep); err != nil {
			failedCount++

			continue
		}

		deleteIstioRoute(ctx, target, isvc, isvcStep)

		if hasAuthEnabled(isvc) {
			ensureAuthResources(ctx, target, isvc, isvcStep)
		} else {
			isvcStep.Recordf(
				"auth-skip-"+isvc.GetName(),
				msgAuthSkipped,
				result.StepSkipped,
				isvc.GetNamespace(), isvc.GetName(),
			)
		}

		convertedCount++
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, msgServerlessDryRun, len(isvcs))
	} else if failedCount > 0 {
		step.Completef(result.StepFailed, msgServerlessFailed, failedCount, len(isvcs))
	} else {
		step.Completef(result.StepCompleted, msgServerlessComplete, convertedCount, len(isvcs))
	}
}

// --- Prepare Task ---

type serverlessToRawPrepareTask struct {
	action *ServerlessToRawAction
}

func (t *serverlessToRawPrepareTask) Validate(
	_ context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	return action.BuildResult(target)
}

func (t *serverlessToRawPrepareTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	step := target.Recorder.Child(
		"backup-serverless-isvcs",
		"Backup Serverless InferenceServices",
	)

	isvcs, err := listISVCsByDeploymentMode(ctx, target, deploymentModeServerless)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Serverless InferenceServices: %v", err)

		return action.BuildResult(target)
	}

	if len(isvcs) == 0 {
		step.Completef(result.StepSkipped, msgServerlessNoISVCs)

		return action.BuildResult(target)
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would backup %d Serverless InferenceServices", len(isvcs))

		return action.BuildResult(target)
	}

	// Group ISVCs by namespace for backup
	byNamespace := groupByNamespace(isvcs)

	for ns, nsISVCs := range byNamespace {
		outputDir := filepath.Join(target.OutputDir, ns)
		if err := backup.WriteResourcesToDir(outputDir, resources.InferenceService.GVR(), nsISVCs); err != nil {
			step.Completef(result.StepFailed, "Failed to write InferenceServices backup for namespace %s: %v", ns, err)

			return action.BuildResult(target)
		}
	}

	step.Completef(result.StepCompleted, msgServerlessBackupDone, len(isvcs), target.OutputDir)

	return action.BuildResult(target)
}

// --- Run Task ---

type serverlessToRawRunTask struct {
	action *ServerlessToRawAction
}

func (t *serverlessToRawRunTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	step := target.Recorder.Child("validate-serverless", "Check for Serverless InferenceServices")

	isvcs, err := listISVCsByDeploymentMode(ctx, target, deploymentModeServerless)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Serverless InferenceServices: %v", err)
	} else if len(isvcs) == 0 {
		step.Completef(result.StepSkipped, msgServerlessNoISVCs)
	} else {
		step.Completef(result.StepCompleted, msgFoundISVCs, len(isvcs), deploymentModeServerless)
	}

	return action.BuildResult(target)
}

func (t *serverlessToRawRunTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	t.action.convertISVCs(ctx, target)

	return action.BuildResult(target)
}
