package modelserving

import (
	"context"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	modelMeshToRawActionID          = "modelserving.modelmesh-to-raw"
	modelMeshToRawActionName        = "Convert ModelMesh InferenceServices to RawDeployment"
	modelMeshToRawActionDescription = "Converts InferenceServices using ModelMesh deployment mode to RawDeployment, updates associated ServingRuntimes, and creates auth resources"

	msgModelMeshConfirm    = "About to convert %d InferenceService(s) from ModelMesh to RawDeployment"
	msgModelMeshCancelled  = "User cancelled ModelMesh to RawDeployment conversion"
	msgModelMeshComplete   = "Processed %d InferenceService(s) for ModelMesh to RawDeployment conversion"
	msgModelMeshDryRun     = "Dry-run: would convert %d InferenceService(s) from ModelMesh to RawDeployment"
	msgModelMeshBackupDone = "Backed up %d ModelMesh InferenceServices to %s"
	msgModelMeshNoISVCs    = "No ModelMesh InferenceServices found"

	msgModelMeshPVCSummary      = "Processed %d InferenceService(s): %d auto-converted, %d PVC-backed (skipped, manual conversion required)"
	msgModelMeshPVCDryRun       = "Dry-run: would convert %d InferenceService(s), %d PVC-backed skipped (manual conversion required)"
	msgModelMeshAllPVC          = "All %d ModelMesh InferenceService(s) use PVC-backed storage — none can be auto-converted"
	msgModelMeshValidateSummary = "Found %d auto-convertible and %d PVC-backed InferenceService(s) with deploymentMode=%s"
)

// ModelMeshToRawAction converts InferenceServices from ModelMesh to RawDeployment mode.
type ModelMeshToRawAction struct{}

func (a *ModelMeshToRawAction) ID() string {
	return modelMeshToRawActionID
}

func (a *ModelMeshToRawAction) Name() string {
	return modelMeshToRawActionName
}

func (a *ModelMeshToRawAction) Description() string {
	return modelMeshToRawActionDescription
}

func (a *ModelMeshToRawAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *ModelMeshToRawAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func (a *ModelMeshToRawAction) CanApply(target action.Target) bool {
	return target.CurrentVersion.Major == 2 && target.CurrentVersion.Minor >= 25
}

func (a *ModelMeshToRawAction) Prepare() action.Task {
	return &modelMeshToRawPrepareTask{action: a}
}

func (a *ModelMeshToRawAction) Run() action.Task {
	return &modelMeshToRawRunTask{action: a}
}

func (a *ModelMeshToRawAction) convertISVCs(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"convert-modelmesh-to-raw",
		"Convert ModelMesh InferenceServices to RawDeployment",
	)

	isvcs, err := listISVCsByDeploymentMode(ctx, target, deploymentModeModelMesh)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list ModelMesh InferenceServices: %v", err)

		return
	}

	if len(isvcs) == 0 {
		step.Completef(result.StepSkipped, msgModelMeshNoISVCs)

		return
	}

	step.Recordf("list-isvcs", msgFoundISVCs, result.StepCompleted, len(isvcs), deploymentModeModelMesh)

	// Classify ISVCs by storage type
	convertible, pvcBacked := classifyISVCsByStorage(ctx, target, isvcs, step)

	// Warn about PVC-backed ISVCs
	for _, isvc := range pvcBacked {
		storageKey := getISVCStorageKey(isvc)

		step.Recordf(
			fmt.Sprintf("pvc-warn-%s-%s", isvc.GetNamespace(), isvc.GetName()),
			msgPVCStorageDetected,
			result.StepSkipped,
			isvc.GetNamespace(), isvc.GetName(), storageKey,
		)
	}

	if len(pvcBacked) > 0 {
		step.Recordf("pvc-manual-steps", msgPVCManualSteps, result.StepSkipped)
	}

	if len(convertible) == 0 {
		step.Completef(result.StepSkipped, msgModelMeshAllPVC, len(pvcBacked))

		return
	}

	// Confirm with user
	if !target.SkipConfirm && !target.DryRun {
		target.IO.Fprintln()
		target.IO.Errorf(msgModelMeshConfirm, len(convertible))

		if !confirmation.Prompt(target.IO, "Proceed with conversion?") {
			step.Completef(result.StepSkipped, msgModelMeshCancelled)

			return
		}
	}

	convertedCount := 0
	processedNamespaces := make(map[string]bool)

	for _, isvc := range convertible {
		isvcStep := step.Child(
			fmt.Sprintf("convert-%s-%s", isvc.GetNamespace(), isvc.GetName()),
			fmt.Sprintf("Convert %s/%s", isvc.GetNamespace(), isvc.GetName()),
		)

		// Update associated ServingRuntime if multi-model
		a.updateServingRuntime(ctx, target, isvc, isvcStep)

		// Patch deployment mode
		patchISVCDeploymentMode(ctx, target, isvc, deploymentModeRawDeployment, isvcStep)

		// Create auth resources only when auth is enabled
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

		processedNamespaces[isvc.GetNamespace()] = true
		convertedCount++
	}

	// Remove modelmesh-enabled label from processed namespaces
	for ns := range processedNamespaces {
		removeModelMeshLabel(ctx, target, ns, step)
	}

	if len(pvcBacked) > 0 {
		if target.DryRun {
			step.Completef(result.StepSkipped, msgModelMeshPVCDryRun, convertedCount, len(pvcBacked))
		} else {
			step.Completef(result.StepCompleted, msgModelMeshPVCSummary, convertedCount+len(pvcBacked), convertedCount, len(pvcBacked))
		}
	} else {
		if target.DryRun {
			step.Completef(result.StepSkipped, msgModelMeshDryRun, convertedCount)
		} else {
			step.Completef(result.StepCompleted, msgModelMeshComplete, convertedCount)
		}
	}
}

func (a *ModelMeshToRawAction) updateServingRuntime(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	parentStep action.StepRecorder,
) {
	runtimeName, err := jq.Query[string](isvc, ".spec.predictor.model.runtime")
	if err != nil {
		return
	}

	ns := isvc.GetNamespace()

	step := parentStep.Child(
		fmt.Sprintf("update-runtime-%s-%s", ns, runtimeName),
		fmt.Sprintf("Update ServingRuntime %s/%s", ns, runtimeName),
	)

	runtime, err := target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
		Namespace(ns).
		Get(ctx, runtimeName, metav1.GetOptions{})

	if err != nil {
		step.Completef(result.StepSkipped, "ServingRuntime %s/%s not found (skipped)", ns, runtimeName)

		return
	}

	// Check if multi-model
	multiModel, err := jq.Query[bool](runtime, ".spec.multiModel")
	if err != nil || !multiModel {
		step.Completef(result.StepSkipped, "ServingRuntime %s/%s is not multi-model (skipped)", ns, runtimeName)

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would update ServingRuntime %s/%s for RawDeployment (multiModel=false, rename container to %s)", ns, runtimeName, kserveContainerName)

		return
	}

	if err := jq.Transform(runtime, ".spec.multiModel = false"); err != nil {
		step.Completef(result.StepFailed, "Failed to update ServingRuntime %s/%s: %v", ns, runtimeName, err)

		return
	}

	// KServe RawDeployment requires a container named "kserve-container"
	if err := jq.Transform(runtime, ".spec.containers[0].name = %q", kserveContainerName); err != nil {
		step.Completef(result.StepFailed, "Failed to rename container in ServingRuntime %s/%s: %v", ns, runtimeName, err)

		return
	}

	_, err = target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
		Namespace(ns).
		Update(ctx, runtime, metav1.UpdateOptions{})

	if err != nil {
		step.Completef(result.StepFailed, "Failed to update ServingRuntime %s/%s: %v", ns, runtimeName, err)

		return
	}

	step.Completef(result.StepCompleted, "Updated ServingRuntime %s/%s (multiModel=false, container renamed to %s)", ns, runtimeName, kserveContainerName)
}

// --- Prepare Task ---

type modelMeshToRawPrepareTask struct {
	action *ModelMeshToRawAction
}

func (t *modelMeshToRawPrepareTask) Validate(
	_ context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	return action.BuildResult(target)
}

func (t *modelMeshToRawPrepareTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	step := target.Recorder.Child(
		"backup-modelmesh-resources",
		"Backup ModelMesh InferenceServices and ServingRuntimes",
	)

	// Backup ISVCs
	isvcs, err := listISVCsByDeploymentMode(ctx, target, deploymentModeModelMesh)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list ModelMesh InferenceServices: %v", err)

		return action.BuildResult(target)
	}

	if len(isvcs) == 0 {
		step.Completef(result.StepSkipped, msgModelMeshNoISVCs)

		return action.BuildResult(target)
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would backup %d ModelMesh InferenceServices and associated ServingRuntimes", len(isvcs))

		return action.BuildResult(target)
	}

	// Backup ISVCs grouped by namespace
	byNamespace := groupByNamespace(isvcs)

	for ns, nsISVCs := range byNamespace {
		outputDir := filepath.Join(target.OutputDir, ns)
		if err := backup.WriteResourcesToDir(outputDir, resources.InferenceService.GVR(), nsISVCs); err != nil {
			step.Completef(result.StepFailed, "Failed to backup InferenceServices in namespace %s: %v", ns, err)

			return action.BuildResult(target)
		}
	}

	// Backup multi-model ServingRuntimes
	multiModelFilter := jq.Predicate(".spec.multiModel == true")

	servingRuntimes, err := client.List[*unstructured.Unstructured](
		ctx, target.Client, resources.ServingRuntime, multiModelFilter,
	)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list ServingRuntimes: %v", err)

		return action.BuildResult(target)
	}

	for ns, nsSRs := range groupByNamespace(servingRuntimes) {
		outputDir := filepath.Join(target.OutputDir, ns)
		if writeErr := backup.WriteResourcesToDir(outputDir, resources.ServingRuntime.GVR(), nsSRs); writeErr != nil {
			step.Completef(result.StepFailed, "Failed to backup ServingRuntimes in namespace %s: %v", ns, writeErr)

			return action.BuildResult(target)
		}
	}

	step.Completef(result.StepCompleted, msgModelMeshBackupDone, len(isvcs), target.OutputDir)

	return action.BuildResult(target)
}

// --- Run Task ---

type modelMeshToRawRunTask struct {
	action *ModelMeshToRawAction
}

func (t *modelMeshToRawRunTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	step := target.Recorder.Child("validate-modelmesh", "Check for ModelMesh InferenceServices")

	isvcs, err := listISVCsByDeploymentMode(ctx, target, deploymentModeModelMesh)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list ModelMesh InferenceServices: %v", err)

		return action.BuildResult(target)
	}

	if len(isvcs) == 0 {
		step.Completef(result.StepSkipped, msgModelMeshNoISVCs)

		return action.BuildResult(target)
	}

	step.Recordf("list-isvcs", msgFoundISVCs, result.StepCompleted, len(isvcs), deploymentModeModelMesh)

	// Detect PVC-backed ISVCs
	convertible, pvcBacked := classifyISVCsByStorage(ctx, target, isvcs, step)

	for _, isvc := range pvcBacked {
		storageKey := getISVCStorageKey(isvc)

		step.Recordf(
			fmt.Sprintf("pvc-detect-%s-%s", isvc.GetNamespace(), isvc.GetName()),
			msgPVCStorageDetected,
			result.StepSkipped,
			isvc.GetNamespace(), isvc.GetName(), storageKey,
		)
	}

	if len(pvcBacked) > 0 {
		step.Recordf("pvc-manual-steps", msgPVCManualSteps, result.StepSkipped)
	}

	step.Completef(result.StepCompleted, msgModelMeshValidateSummary, len(convertible), len(pvcBacked), deploymentModeModelMesh)

	return action.BuildResult(target)
}

func (t *modelMeshToRawRunTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	t.action.convertISVCs(ctx, target)

	return action.BuildResult(target)
}
