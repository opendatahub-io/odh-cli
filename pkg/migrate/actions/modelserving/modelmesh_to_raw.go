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
	msgModelMeshComplete   = "Processed %d InferenceService(s): %d standard, %d PVC-backed"
	msgModelMeshDryRun     = "Dry-run: would convert %d InferenceService(s) from ModelMesh to RawDeployment (%d standard, %d PVC-backed)"
	msgModelMeshBackupDone = "Backed up %d ModelMesh InferenceServices to %s"
	msgModelMeshNoISVCs    = "No ModelMesh InferenceServices found"

	msgPVCDetected          = "InferenceService %s/%s uses PVC storage (key: %s)"
	msgPVCClassifyFailed    = "Failed to detect storage type for InferenceService %s/%s: %v (skipped — resolve and retry)"
	msgPVCRuntimeUpdate     = "Updated ServingRuntime %s/%s with OVMS single-model args, port 8888, readiness probe"
	msgPVCRuntimeDryRun     = "Would update ServingRuntime %s/%s with OVMS single-model args, port 8888, readiness probe"
	msgPVCRuntimeFailed     = "Failed to update ServingRuntime %s/%s for PVC single-model: %v"
	msgPVCRuntimeShared     = "ServingRuntime %s/%s is already patched for another PVC InferenceService; --model_name keeps the first ISVC processed. Create a dedicated ServingRuntime for %s/%s"
	msgPVCStorageURIInvalid = "Failed to build storageUri for InferenceService %s/%s: %v"
	msgPVCStorageURISet     = "Set storageUri=%s and deploymentMode=RawDeployment on InferenceService %s/%s"
	msgPVCStorageURIDryRun  = "Would set storageUri=%s and deploymentMode=RawDeployment on InferenceService %s/%s"
	msgPVCStorageURIFailed  = "Failed to update InferenceService %s/%s for PVC conversion: %v"
	msgPVCConversionAborted = "Skipping remaining steps for InferenceService %s/%s due to PVC conversion failure"
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

	// Classify ISVCs by storage type (detection only — no mutations, safe before consent)
	detectionStep := step.Child(
		"detect-storage-types",
		"Detect PVC-backed InferenceServices",
	)

	standardISVCs, pvcISVCs := a.classifyISVCsByStorageType(ctx, target, isvcs, detectionStep)
	skippedCount := len(isvcs) - len(standardISVCs) - len(pvcISVCs)

	if skippedCount > 0 {
		detectionStep.Completef(result.StepFailed, "Detected %d standard, %d PVC-backed, %d skipped (detection failed) InferenceService(s)", len(standardISVCs), len(pvcISVCs), skippedCount)
	} else {
		detectionStep.Completef(result.StepCompleted, "Detected %d standard and %d PVC-backed InferenceService(s)", len(standardISVCs), len(pvcISVCs))
	}

	// Confirm with user
	if !target.SkipConfirm && !target.DryRun {
		target.IO.Fprintln()
		target.IO.Errorf(msgModelMeshConfirm, len(standardISVCs)+len(pvcISVCs))

		if len(pvcISVCs) > 0 {
			target.IO.Errorf("  (%d standard, %d PVC-backed requiring storageUri rewrite)", len(standardISVCs), len(pvcISVCs))
		}

		if !confirmation.Prompt(target.IO, "Proceed with conversion?") {
			step.Completef(result.StepSkipped, msgModelMeshCancelled)

			return
		}
	}

	var (
		standardCount       int
		pvcCount            int
		processedNamespaces = make(map[string]bool)
		patchedRuntimes     = make(map[string]bool)
	)

	// Convert standard (S3/HDFS) ISVCs
	for _, isvc := range standardISVCs {
		isvcStep := step.Child(
			fmt.Sprintf("convert-%s-%s", isvc.GetNamespace(), isvc.GetName()),
			fmt.Sprintf("Convert %s/%s", isvc.GetNamespace(), isvc.GetName()),
		)

		a.updateServingRuntime(ctx, target, isvc, isvcStep)
		patchISVCDeploymentMode(ctx, target, isvc, deploymentModeRawDeployment, isvcStep)
		finalizeISVCConversion(ctx, target, isvc, isvcStep, processedNamespaces)

		standardCount++
	}

	// Convert PVC-backed ISVCs
	for _, pi := range pvcISVCs {
		isvcStep := step.Child(
			fmt.Sprintf("convert-pvc-%s-%s", pi.isvc.GetNamespace(), pi.isvc.GetName()),
			fmt.Sprintf("Convert PVC-backed %s/%s", pi.isvc.GetNamespace(), pi.isvc.GetName()),
		)

		isvcStep.Recordf(
			"pvc-detected-"+pi.isvc.GetName(),
			msgPVCDetected,
			result.StepCompleted,
			pi.isvc.GetNamespace(), pi.isvc.GetName(), pi.storageKey,
		)

		if !a.convertPVCISVC(ctx, target, pi, isvcStep, patchedRuntimes) {
			isvcStep.Recordf(
				"pvc-aborted-"+pi.isvc.GetName(),
				msgPVCConversionAborted,
				result.StepFailed,
				pi.isvc.GetNamespace(), pi.isvc.GetName(),
			)

			continue
		}

		finalizeISVCConversion(ctx, target, pi.isvc, isvcStep, processedNamespaces)

		pvcCount++
	}

	// Remove modelmesh-enabled label from processed namespaces
	for ns := range processedNamespaces {
		removeModelMeshLabel(ctx, target, ns, step)
	}

	total := standardCount + pvcCount

	if target.DryRun {
		step.Completef(result.StepSkipped, msgModelMeshDryRun, total, standardCount, pvcCount)
	} else {
		step.Completef(result.StepCompleted, msgModelMeshComplete, total, standardCount, pvcCount)
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

// finalizeISVCConversion handles auth resources and namespace tracking common to all ISVC conversions.
func finalizeISVCConversion(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	step action.StepRecorder,
	processedNamespaces map[string]bool,
) {
	if hasAuthEnabled(isvc) {
		ensureAuthResources(ctx, target, isvc, step)
	} else {
		step.Recordf(
			"auth-skip-"+isvc.GetName(),
			msgAuthSkipped,
			result.StepSkipped,
			isvc.GetNamespace(), isvc.GetName(),
		)
	}

	processedNamespaces[isvc.GetNamespace()] = true
}

// pvcISVCInfo holds a PVC-backed ISVC with its resolved storage config.
type pvcISVCInfo struct {
	isvc       *unstructured.Unstructured
	storageKey string
	entry      *storageConfigEntry
}

// classifyISVCsByStorageType splits ISVCs into standard (S3/HDFS) and PVC-backed.
func (a *ModelMeshToRawAction) classifyISVCsByStorageType(
	ctx context.Context,
	target action.Target,
	isvcs []*unstructured.Unstructured,
	step action.StepRecorder,
) ([]*unstructured.Unstructured, []pvcISVCInfo) {
	var (
		standard  []*unstructured.Unstructured
		pvcBacked []pvcISVCInfo
	)

	for _, isvc := range isvcs {
		storageKey := getISVCStorageKey(isvc)
		if storageKey == "" {
			standard = append(standard, isvc)

			continue
		}

		entry, err := getStorageConfigEntry(ctx, target, isvc.GetNamespace(), storageKey)
		if err != nil {
			step.Recordf(
				"classify-"+isvc.GetName(),
				msgPVCClassifyFailed,
				result.StepFailed,
				isvc.GetNamespace(), isvc.GetName(), err,
			)

			continue
		}

		if entry == nil || entry.Type != storageTypePVC {
			standard = append(standard, isvc)

			continue
		}

		pvcBacked = append(pvcBacked, pvcISVCInfo{
			isvc:       isvc,
			storageKey: storageKey,
			entry:      entry,
		})
	}

	return standard, pvcBacked
}

// convertPVCISVC rewrites an ISVC's storage and deployment mode for PVC-backed models,
// and updates its ServingRuntime. Returns true on success, false if the ISVC was not modified.
func (a *ModelMeshToRawAction) convertPVCISVC(
	ctx context.Context,
	target action.Target,
	pi pvcISVCInfo,
	step action.StepRecorder,
	patchedRuntimes map[string]bool,
) bool {
	ns := pi.isvc.GetNamespace()
	name := pi.isvc.GetName()

	storageURI, err := buildPVCStorageURI(pi.entry)
	if err != nil {
		step.Recordf("pvc-storageuri-"+name, msgPVCStorageURIInvalid, result.StepFailed, ns, name, err)

		return false
	}

	if target.DryRun {
		step.Recordf("pvc-storageuri-"+name, msgPVCStorageURIDryRun, result.StepSkipped, storageURI, ns, name)
		a.updateServingRuntimeForPVC(ctx, target, pi.isvc, step, patchedRuntimes)

		return true
	}

	if err := jq.Transform(pi.isvc, ".spec.predictor.model.storageUri = %q", storageURI); err != nil {
		step.Recordf("pvc-storageuri-"+name, msgPVCStorageURIFailed, result.StepFailed, ns, name, err)

		return false
	}

	// Remove ModelMesh storage key/path — storageUri replaces them
	unstructured.RemoveNestedField(pi.isvc.Object, "spec", "predictor", "model", "storage")

	// Set deployment mode to RawDeployment in the same update
	annotations := pi.isvc.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[annotationDeploymentMode] = deploymentModeRawDeployment
	pi.isvc.SetAnnotations(annotations)

	_, err = target.Client.Dynamic().Resource(resources.InferenceService.GVR()).
		Namespace(ns).
		Update(ctx, pi.isvc, metav1.UpdateOptions{})
	if err != nil {
		step.Recordf("pvc-storageuri-"+name, msgPVCStorageURIFailed, result.StepFailed, ns, name, err)

		return false
	}

	step.Recordf("pvc-storageuri-"+name, msgPVCStorageURISet, result.StepCompleted, storageURI, ns, name)

	a.updateServingRuntimeForPVC(ctx, target, pi.isvc, step, patchedRuntimes)

	return true
}

// updateServingRuntimeForPVC patches a ServingRuntime for PVC single-model OVMS deployment:
// sets multiModel=false, renames container, replaces args, adds port 8888 and readiness probe.
func (a *ModelMeshToRawAction) updateServingRuntimeForPVC(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	parentStep action.StepRecorder,
	patchedRuntimes map[string]bool,
) {
	runtimeName, err := jq.Query[string](isvc, ".spec.predictor.model.runtime")
	if err != nil {
		return
	}

	ns := isvc.GetNamespace()
	isvcName := isvc.GetName()
	runtimeKey := ns + "/" + runtimeName

	step := parentStep.Child(
		fmt.Sprintf("update-pvc-runtime-%s-%s", ns, runtimeName),
		fmt.Sprintf("Update ServingRuntime %s/%s for PVC single-model", ns, runtimeName),
	)

	if patchedRuntimes[runtimeKey] {
		step.Completef(result.StepFailed, msgPVCRuntimeShared, ns, runtimeName, ns, isvcName)

		return
	}

	runtime, err := target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
		Namespace(ns).
		Get(ctx, runtimeName, metav1.GetOptions{})
	if err != nil {
		step.Completef(result.StepSkipped, "ServingRuntime %s/%s not found (skipped)", ns, runtimeName)

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, msgPVCRuntimeDryRun, ns, runtimeName)
		patchedRuntimes[runtimeKey] = true

		return
	}

	// Set multiModel=false
	if err := jq.Transform(runtime, ".spec.multiModel = false"); err != nil {
		step.Completef(result.StepFailed, msgPVCRuntimeFailed, ns, runtimeName, err)

		return
	}

	// Rename container to kserve-container
	if err := jq.Transform(runtime, ".spec.containers[0].name = %q", kserveContainerName); err != nil {
		step.Completef(result.StepFailed, msgPVCRuntimeFailed, ns, runtimeName, err)

		return
	}

	// Replace container args with single-model OVMS args
	containers, _, _ := unstructured.NestedSlice(runtime.Object, "spec", "containers")
	if len(containers) > 0 {
		container, ok := containers[0].(map[string]any)
		if ok {
			container["args"] = []any{
				"--model_name=" + isvcName,
				"--model_path=/mnt/models",
				fmt.Sprintf("--port=%d", ovmsGRPCPort),
				fmt.Sprintf("--rest_port=%d", ovmsRESTPort),
			}

			container["ports"] = []any{
				map[string]any{
					"containerPort": ovmsRESTPort,
					"protocol":      "TCP",
				},
			}

			container["readinessProbe"] = map[string]any{
				"tcpSocket": map[string]any{
					"port": ovmsRESTPort,
				},
				"initialDelaySeconds": ovmsReadinessInitialDelay,
				"periodSeconds":       ovmsReadinessPeriod,
			}

			containers[0] = container

			if err := unstructured.SetNestedSlice(runtime.Object, containers, "spec", "containers"); err != nil {
				step.Completef(result.StepFailed, msgPVCRuntimeFailed, ns, runtimeName, err)

				return
			}
		}
	}

	_, err = target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
		Namespace(ns).
		Update(ctx, runtime, metav1.UpdateOptions{})
	if err != nil {
		step.Completef(result.StepFailed, msgPVCRuntimeFailed, ns, runtimeName, err)

		return
	}

	patchedRuntimes[runtimeKey] = true

	step.Completef(result.StepCompleted, msgPVCRuntimeUpdate, ns, runtimeName)
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
	} else if len(isvcs) == 0 {
		step.Completef(result.StepSkipped, msgModelMeshNoISVCs)
	} else {
		step.Completef(result.StepCompleted, msgFoundISVCs, len(isvcs), deploymentModeModelMesh)
	}

	return action.BuildResult(target)
}

func (t *modelMeshToRawRunTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	t.action.convertISVCs(ctx, target)

	return action.BuildResult(target)
}
