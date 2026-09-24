package modelserving

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"

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
	msgModelMeshFailed     = "Processed %d of %d InferenceService(s): %d standard, %d PVC-backed, %d failed"
	msgModelMeshDryRun     = "Dry-run: would convert %d InferenceService(s) from ModelMesh to RawDeployment (%d standard, %d PVC-backed)"
	msgModelMeshBackupDone = "Backed up %d ModelMesh InferenceServices to %s"
	msgModelMeshNoISVCs    = "No ModelMesh InferenceServices found"

	msgPVCDetected                  = "InferenceService %s/%s uses PVC storage %s (key: %s)"
	msgPVCClassifyFailed            = "Failed to detect storage type for InferenceService %s/%s: %v (skipped — resolve and retry)"
	msgPVCRuntimeUpdate             = "Updated ServingRuntime %s/%s with OVMS single-model args, port 8888, readiness probe"
	msgPVCRuntimeDryRun             = "Would update ServingRuntime %s/%s with OVMS single-model args, port 8888, readiness probe"
	msgPVCRuntimeFailed             = "Failed to update ServingRuntime %s/%s for PVC single-model: %v"
	msgRuntimeReferenceFailed       = "Failed to read ServingRuntime reference for InferenceService %s/%s: %v"
	msgPVCRuntimeNoContainer        = "ServingRuntime %s/%s has no containers"
	msgPVCRuntimeInvalidContainer   = "ServingRuntime %s/%s has an invalid container"
	msgPVCServingRuntimeUnavailable = "Cannot convert InferenceService %s/%s: ServingRuntime %s/%s is unavailable: %v"
	msgPVCStorageURIInvalid         = "Failed to build storageUri for InferenceService %s/%s: %v"
	msgPVCStorageURISet             = "Set storageUri=%s and deploymentMode=RawDeployment on InferenceService %s/%s"
	msgPVCStorageURIDryRun          = "Would set storageUri=%s and deploymentMode=RawDeployment on InferenceService %s/%s"
	msgPVCStorageURIFailed          = "Failed to update InferenceService %s/%s for PVC conversion: %v"
	msgPVCConversionAborted         = "Skipping remaining steps for InferenceService %s/%s due to PVC conversion failure"
	msgPVCConflictingRuntime        = "Cannot convert InferenceServices sharing ServingRuntime(s): %s; assign each model a dedicated ServingRuntime"
	msgRuntimeNotFound              = "ServingRuntime %s/%s not found (skipped)"
	msgRuntimeGetFailed             = "Failed to get ServingRuntime %s/%s: %v"
	msgRuntimeRollback              = "Restored ServingRuntime %s/%s after conversion failure"
	msgRuntimeRollbackFailed        = "Failed to restore ServingRuntime %s/%s after conversion failure: %v"
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

	standardISVCs, pvcISVCs, skippedISVCs := a.classifyISVCsByStorageType(ctx, target, isvcs, detectionStep)
	skippedCount := len(skippedISVCs)

	if skippedCount > 0 {
		detectionStep.Completef(result.StepFailed, "Detected %d standard, %d PVC-backed, %d skipped (detection failed) InferenceService(s)", len(standardISVCs), len(pvcISVCs), skippedCount)
	} else {
		detectionStep.Completef(result.StepCompleted, "Detected %d standard and %d PVC-backed InferenceService(s)", len(standardISVCs), len(pvcISVCs))
	}

	if !validatePVCServingRuntimeAssignments(ctx, target, standardISVCs, pvcISVCs, skippedISVCs, step) {
		return
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

	processedNamespaces := make(map[string]bool)
	blockedNamespaces := sets.New[string]()
	for _, isvc := range skippedISVCs {
		blockedNamespaces.Insert(isvc.GetNamespace())
	}

	standardCount, standardFailedCount := a.convertStandardISVCs(ctx, target, standardISVCs, step, processedNamespaces, blockedNamespaces)
	pvcCount, pvcFailedCount := a.convertPVCISVCs(ctx, target, pvcISVCs, step, processedNamespaces, blockedNamespaces)

	// Remove modelmesh-enabled label from processed namespaces
	for ns := range processedNamespaces {
		if blockedNamespaces.Has(ns) {
			continue
		}

		removeModelMeshLabel(ctx, target, ns, step)
	}

	total := standardCount + pvcCount
	failedCount := skippedCount + standardFailedCount + pvcFailedCount

	if failedCount > 0 {
		step.Completef(result.StepFailed, msgModelMeshFailed, total, len(isvcs), standardCount, pvcCount, failedCount)
	} else if target.DryRun {
		step.Completef(result.StepSkipped, msgModelMeshDryRun, total, standardCount, pvcCount)
	} else {
		step.Completef(result.StepCompleted, msgModelMeshComplete, total, standardCount, pvcCount)
	}
}

func (a *ModelMeshToRawAction) convertStandardISVCs(
	ctx context.Context,
	target action.Target,
	isvcs []*unstructured.Unstructured,
	parentStep action.StepRecorder,
	processedNamespaces map[string]bool,
	blockedNamespaces sets.Set[string],
) (int, int) {
	var converted, failed int

	for _, isvc := range isvcs {
		isvcStep := parentStep.Child(
			fmt.Sprintf("convert-%s-%s", isvc.GetNamespace(), isvc.GetName()),
			fmt.Sprintf("Convert %s/%s", isvc.GetNamespace(), isvc.GetName()),
		)

		if !finalizeISVCConversion(ctx, target, isvc, isvcStep) {
			blockedNamespaces.Insert(isvc.GetNamespace())
			failed++

			continue
		}

		runtimeSnapshot, runtimeOK := a.updateServingRuntime(ctx, target, isvc, isvcStep)
		if !runtimeOK {
			blockedNamespaces.Insert(isvc.GetNamespace())
			failed++

			continue
		}

		if !patchISVCDeploymentMode(ctx, target, isvc, deploymentModeRawDeployment, isvcStep) {
			restoreServingRuntime(ctx, target, runtimeSnapshot, isvcStep)
			blockedNamespaces.Insert(isvc.GetNamespace())
			failed++

			continue
		}

		processedNamespaces[isvc.GetNamespace()] = true
		converted++
	}

	return converted, failed
}

func (a *ModelMeshToRawAction) convertPVCISVCs(
	ctx context.Context,
	target action.Target,
	pvcISVCs []pvcISVCInfo,
	parentStep action.StepRecorder,
	processedNamespaces map[string]bool,
	blockedNamespaces sets.Set[string],
) (int, int) {
	var converted, failed int

	for _, pi := range pvcISVCs {
		isvcStep := parentStep.Child(
			fmt.Sprintf("convert-pvc-%s-%s", pi.isvc.GetNamespace(), pi.isvc.GetName()),
			fmt.Sprintf("Convert PVC-backed %s/%s", pi.isvc.GetNamespace(), pi.isvc.GetName()),
		)

		isvcStep.Recordf(
			"pvc-detected-"+pi.isvc.GetName(),
			msgPVCDetected,
			result.StepCompleted,
			pi.isvc.GetNamespace(), pi.isvc.GetName(), pi.entry.Name, pi.storageKey,
		)

		if !finalizeISVCConversion(ctx, target, pi.isvc, isvcStep) || !a.convertPVCISVC(ctx, target, pi, isvcStep) {
			failed++
			isvcStep.Recordf(
				"pvc-aborted-"+pi.isvc.GetName(),
				msgPVCConversionAborted,
				result.StepFailed,
				pi.isvc.GetNamespace(), pi.isvc.GetName(),
			)
			blockedNamespaces.Insert(pi.isvc.GetNamespace())

			continue
		}

		processedNamespaces[pi.isvc.GetNamespace()] = true
		converted++
	}

	return converted, failed
}

func validatePVCServingRuntimeAssignments(
	ctx context.Context,
	target action.Target,
	standardISVCs []*unstructured.Unstructured,
	pvcISVCs []pvcISVCInfo,
	skippedISVCs []*unstructured.Unstructured,
	parentStep action.StepRecorder,
) bool {
	if len(pvcISVCs) == 0 && len(skippedISVCs) == 0 {
		return true
	}

	runtimeValidationStep := parentStep.Child(
		"validate-pvc-runtimes",
		"Validate ServingRuntime assignments for PVC-backed InferenceServices",
	)
	conflictingRuntimes := findPVCServingRuntimeConflicts(standardISVCs, skippedISVCs, pvcISVCs)
	if len(conflictingRuntimes) > 0 {
		message := fmt.Sprintf(msgPVCConflictingRuntime, strings.Join(conflictingRuntimes, ", "))
		runtimeValidationStep.Completef(result.StepFailed, "%s", message)
		parentStep.Completef(result.StepFailed, "%s", message)

		return false
	}

	validatedRuntimes := sets.New[string]()
	for _, pi := range pvcISVCs {
		runtimeName, err := jq.Query[string](pi.isvc, ".spec.predictor.model.runtime")
		if err != nil {
			message := fmt.Sprintf(msgRuntimeReferenceFailed, pi.isvc.GetNamespace(), pi.isvc.GetName(), err)
			runtimeValidationStep.Completef(result.StepFailed, "%s", message)
			parentStep.Completef(result.StepFailed, "%s", message)

			return false
		}

		runtimeKey := pi.isvc.GetNamespace() + "/" + runtimeName
		if validatedRuntimes.Has(runtimeKey) {
			continue
		}

		if _, err := target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
			Namespace(pi.isvc.GetNamespace()).
			Get(ctx, runtimeName, metav1.GetOptions{}); err != nil {
			message := fmt.Sprintf(
				msgPVCServingRuntimeUnavailable,
				pi.isvc.GetNamespace(), pi.isvc.GetName(), pi.isvc.GetNamespace(), runtimeName, err,
			)
			runtimeValidationStep.Completef(result.StepFailed, "%s", message)
			parentStep.Completef(result.StepFailed, "%s", message)

			return false
		}

		validatedRuntimes.Insert(runtimeKey)
	}

	runtimeValidationStep.Completef(result.StepCompleted, "No conflicting ServingRuntime assignments found")

	return true
}

func (a *ModelMeshToRawAction) updateServingRuntime(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	parentStep action.StepRecorder,
) (*unstructured.Unstructured, bool) {
	runtimeName, err := jq.Query[string](isvc, ".spec.predictor.model.runtime")
	if err != nil {
		parentStep.Recordf(
			"runtime-reference-"+isvc.GetName(),
			msgRuntimeReferenceFailed,
			result.StepFailed,
			isvc.GetNamespace(), isvc.GetName(), err,
		)

		return nil, false
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
		if apierrors.IsNotFound(err) {
			step.Completef(result.StepSkipped, msgRuntimeNotFound, ns, runtimeName)

			return nil, true
		}

		step.Completef(result.StepFailed, msgRuntimeGetFailed, ns, runtimeName, err)

		return nil, false
	}

	// Check if multi-model
	multiModel, err := jq.Query[bool](runtime, ".spec.multiModel")
	if err != nil || !multiModel {
		step.Completef(result.StepSkipped, "ServingRuntime %s/%s is not multi-model (skipped)", ns, runtimeName)

		return nil, true
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would update ServingRuntime %s/%s for RawDeployment (multiModel=false, rename container to %s)", ns, runtimeName, kserveContainerName)

		return nil, true
	}

	runtimeSnapshot := runtime.DeepCopy()

	if err := jq.Transform(runtime, ".spec.multiModel = false"); err != nil {
		step.Completef(result.StepFailed, "Failed to update ServingRuntime %s/%s: %v", ns, runtimeName, err)

		return nil, false
	}

	// KServe RawDeployment requires a container named "kserve-container"
	if err := jq.Transform(runtime, ".spec.containers[0].name = %q", kserveContainerName); err != nil {
		step.Completef(result.StepFailed, "Failed to rename container in ServingRuntime %s/%s: %v", ns, runtimeName, err)

		return nil, false
	}

	_, err = target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
		Namespace(ns).
		Update(ctx, runtime, metav1.UpdateOptions{})

	if err != nil {
		step.Completef(result.StepFailed, "Failed to update ServingRuntime %s/%s: %v", ns, runtimeName, err)

		return nil, false
	}

	step.Completef(result.StepCompleted, "Updated ServingRuntime %s/%s (multiModel=false, container renamed to %s)", ns, runtimeName, kserveContainerName)

	return runtimeSnapshot, true
}

func restoreServingRuntime(
	ctx context.Context,
	target action.Target,
	runtimeSnapshot *unstructured.Unstructured,
	step action.StepRecorder,
) {
	if runtimeSnapshot == nil || target.DryRun {
		return
	}

	ns := runtimeSnapshot.GetNamespace()
	name := runtimeSnapshot.GetName()
	runtime, err := target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
		Namespace(ns).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		step.Recordf("rollback-runtime-"+name, msgRuntimeRollbackFailed, result.StepFailed, ns, name, err)

		return
	}

	runtime.Object["spec"] = runtimeSnapshot.Object["spec"]
	if _, err = target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
		Namespace(ns).
		Update(ctx, runtime, metav1.UpdateOptions{}); err != nil {
		step.Recordf("rollback-runtime-"+name, msgRuntimeRollbackFailed, result.StepFailed, ns, name, err)

		return
	}

	step.Recordf("rollback-runtime-"+name, msgRuntimeRollback, result.StepCompleted, ns, name)
}

// finalizeISVCConversion handles auth resources and namespace tracking common to all ISVC conversions.
func finalizeISVCConversion(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	step action.StepRecorder,
) bool {
	if hasAuthEnabled(isvc) {
		return ensureAuthResources(ctx, target, isvc, step)
	}

	step.Recordf(
		"auth-skip-"+isvc.GetName(),
		msgAuthSkipped,
		result.StepSkipped,
		isvc.GetNamespace(), isvc.GetName(),
	)

	return true
}

// pvcISVCInfo holds a PVC-backed ISVC with its resolved storage config.
type pvcISVCInfo struct {
	isvc        *unstructured.Unstructured
	storageKey  string
	storagePath string
	entry       *storageConfigEntry
}

// classifyISVCsByStorageType splits ISVCs into standard (S3/HDFS), PVC-backed, and skipped.
func (a *ModelMeshToRawAction) classifyISVCsByStorageType(
	ctx context.Context,
	target action.Target,
	isvcs []*unstructured.Unstructured,
	step action.StepRecorder,
) ([]*unstructured.Unstructured, []pvcISVCInfo, []*unstructured.Unstructured) {
	type storageConfigCacheEntry struct {
		data map[string]any
		err  error
	}

	var (
		standard  []*unstructured.Unstructured
		pvcBacked []pvcISVCInfo
		skipped   []*unstructured.Unstructured
	)
	storageConfigCache := make(map[string]storageConfigCacheEntry)

	for _, isvc := range isvcs {
		storageKey := getISVCStorageKey(isvc)
		if storageKey == "" {
			standard = append(standard, isvc)

			continue
		}

		cacheEntry, found := storageConfigCache[isvc.GetNamespace()]
		if !found {
			cacheEntry.data, cacheEntry.err = getStorageConfigSecretData(ctx, target, isvc.GetNamespace())
			storageConfigCache[isvc.GetNamespace()] = cacheEntry
		}

		var entry *storageConfigEntry
		var err error
		if cacheEntry.err == nil && cacheEntry.data != nil {
			entry, err = decodeStorageConfigEntry(cacheEntry.data, storageKey)
		} else {
			err = cacheEntry.err
		}
		if err != nil {
			step.Recordf(
				"classify-"+isvc.GetName(),
				msgPVCClassifyFailed,
				result.StepFailed,
				isvc.GetNamespace(), isvc.GetName(), err,
			)
			skipped = append(skipped, isvc)

			continue
		}

		if entry == nil || entry.Type != storageTypePVC {
			standard = append(standard, isvc)

			continue
		}

		pvcBacked = append(pvcBacked, pvcISVCInfo{
			isvc:        isvc,
			storageKey:  storageKey,
			storagePath: getISVCStoragePath(isvc),
			entry:       entry,
		})
	}

	return standard, pvcBacked, skipped
}

func findPVCServingRuntimeConflicts(
	standardISVCs []*unstructured.Unstructured,
	skippedISVCs []*unstructured.Unstructured,
	pvcISVCs []pvcISVCInfo,
) []string {
	standardRuntimes := sets.New[string]()
	for _, isvc := range standardISVCs {
		if runtimeKey := getServingRuntimeKey(isvc); runtimeKey != "" {
			standardRuntimes.Insert(runtimeKey)
		}
	}

	skippedRuntimes := sets.New[string]()
	for _, isvc := range skippedISVCs {
		if runtimeKey := getServingRuntimeKey(isvc); runtimeKey != "" {
			skippedRuntimes.Insert(runtimeKey)
		}
	}

	pvcRuntimeCounts := make(map[string]int)
	for _, pi := range pvcISVCs {
		if runtimeKey := getServingRuntimeKey(pi.isvc); runtimeKey != "" {
			pvcRuntimeCounts[runtimeKey]++
		}
	}

	conflicts := sets.New[string]()
	for runtimeKey := range standardRuntimes {
		if skippedRuntimes.Has(runtimeKey) {
			conflicts.Insert(runtimeKey)
		}
	}

	for runtimeKey, count := range pvcRuntimeCounts {
		if count > 1 || standardRuntimes.Has(runtimeKey) || skippedRuntimes.Has(runtimeKey) {
			conflicts.Insert(runtimeKey)
		}
	}

	return sets.List(conflicts)
}

func getServingRuntimeKey(isvc *unstructured.Unstructured) string {
	runtimeName, err := jq.Query[string](isvc, ".spec.predictor.model.runtime")
	if err != nil || runtimeName == "" {
		return ""
	}

	return isvc.GetNamespace() + "/" + runtimeName
}

// convertPVCISVC rewrites an ISVC's storage and deployment mode for PVC-backed models,
// and updates its ServingRuntime. Returns true on success, false if the ISVC was not modified.
func (a *ModelMeshToRawAction) convertPVCISVC(
	ctx context.Context,
	target action.Target,
	pi pvcISVCInfo,
	step action.StepRecorder,
) bool {
	ns := pi.isvc.GetNamespace()
	name := pi.isvc.GetName()

	storageURI, err := buildPVCStorageURI(pi.entry, pi.storagePath)
	if err != nil {
		step.Recordf("pvc-storageuri-"+name, msgPVCStorageURIInvalid, result.StepFailed, ns, name, err)

		return false
	}

	if target.DryRun {
		step.Recordf("pvc-storageuri-"+name, msgPVCStorageURIDryRun, result.StepSkipped, storageURI, ns, name)

		_, runtimeOK := a.updateServingRuntimeForPVC(ctx, target, pi.isvc, step)

		return runtimeOK
	}

	runtimeSnapshot, runtimeOK := a.updateServingRuntimeForPVC(ctx, target, pi.isvc, step)
	if !runtimeOK {
		return false
	}

	if err := jq.Transform(pi.isvc, ".spec.predictor.model.storageUri = %q", storageURI); err != nil {
		step.Recordf("pvc-storageuri-"+name, msgPVCStorageURIFailed, result.StepFailed, ns, name, err)
		restoreServingRuntime(ctx, target, runtimeSnapshot, step)

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
		restoreServingRuntime(ctx, target, runtimeSnapshot, step)

		return false
	}

	step.Recordf("pvc-storageuri-"+name, msgPVCStorageURISet, result.StepCompleted, storageURI, ns, name)

	return true
}

// updateServingRuntimeForPVC patches a ServingRuntime for PVC single-model OVMS deployment:
// sets multiModel=false, renames the container, updates model args, and adds port 8888 and readiness probe.
func (a *ModelMeshToRawAction) updateServingRuntimeForPVC(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	parentStep action.StepRecorder,
) (*unstructured.Unstructured, bool) {
	runtimeName, err := jq.Query[string](isvc, ".spec.predictor.model.runtime")
	if err != nil {
		parentStep.Recordf(
			"pvc-runtime-reference-"+isvc.GetName(),
			msgRuntimeReferenceFailed,
			result.StepFailed,
			isvc.GetNamespace(), isvc.GetName(), err,
		)

		return nil, false
	}

	ns := isvc.GetNamespace()
	isvcName := isvc.GetName()

	step := parentStep.Child(
		fmt.Sprintf("update-pvc-runtime-%s-%s", ns, runtimeName),
		fmt.Sprintf("Update ServingRuntime %s/%s for PVC single-model", ns, runtimeName),
	)

	runtime, err := target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
		Namespace(ns).
		Get(ctx, runtimeName, metav1.GetOptions{})
	if err != nil {
		step.Completef(result.StepFailed, msgPVCRuntimeFailed, ns, runtimeName, err)

		return nil, false
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, msgPVCRuntimeDryRun, ns, runtimeName)

		return nil, true
	}

	runtimeSnapshot := runtime.DeepCopy()

	// Set multiModel=false
	if err := jq.Transform(runtime, ".spec.multiModel = false"); err != nil {
		step.Completef(result.StepFailed, msgPVCRuntimeFailed, ns, runtimeName, err)

		return nil, false
	}

	// Rename container to kserve-container
	if err := jq.Transform(runtime, ".spec.containers[0].name = %q", kserveContainerName); err != nil {
		step.Completef(result.StepFailed, msgPVCRuntimeFailed, ns, runtimeName, err)

		return nil, false
	}

	// Replace container args with single-model OVMS args
	containers, _, _ := unstructured.NestedSlice(runtime.Object, "spec", "containers")
	if len(containers) == 0 {
		step.Completef(result.StepFailed, msgPVCRuntimeNoContainer, ns, runtimeName)

		return nil, false
	}

	container, ok := containers[0].(map[string]any)
	if !ok {
		step.Completef(result.StepFailed, msgPVCRuntimeInvalidContainer, ns, runtimeName)

		return nil, false
	}

	containerArgs, _ := container["args"].([]any)
	container["args"] = buildSingleModelOVMSArgs(containerArgs, isvcName)

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

		return nil, false
	}

	_, err = target.Client.Dynamic().Resource(resources.ServingRuntime.GVR()).
		Namespace(ns).
		Update(ctx, runtime, metav1.UpdateOptions{})
	if err != nil {
		step.Completef(result.StepFailed, msgPVCRuntimeFailed, ns, runtimeName, err)

		return nil, false
	}

	step.Completef(result.StepCompleted, msgPVCRuntimeUpdate, ns, runtimeName)

	return runtimeSnapshot, true
}

func buildSingleModelOVMSArgs(existingArgs []any, isvcName string) []any {
	args := make([]any, 0, len(existingArgs)+ovmsSingleModelArgCount)
	skipValue := false
	for _, arg := range existingArgs {
		if skipValue {
			skipValue = false

			continue
		}

		argString, ok := arg.(string)
		if !ok {
			args = append(args, arg)

			continue
		}

		switch {
		case strings.HasPrefix(argString, "--model_name="),
			strings.HasPrefix(argString, "--model_path="),
			strings.HasPrefix(argString, "--port="),
			strings.HasPrefix(argString, "--rest_port="),
			strings.HasPrefix(argString, "--config_path="),
			strings.HasPrefix(argString, "--grpc_bind_address="),
			strings.HasPrefix(argString, "--rest_bind_address="):
			continue
		case argString == "--model_name",
			argString == "--model_path",
			argString == "--port",
			argString == "--rest_port",
			argString == "--config_path",
			argString == "--grpc_bind_address",
			argString == "--rest_bind_address":
			skipValue = true

			continue
		default:
			args = append(args, arg)
		}
	}

	return append(args,
		"--model_name="+isvcName,
		"--model_path=/mnt/models",
		fmt.Sprintf("--port=%d", ovmsGRPCPort),
		fmt.Sprintf("--rest_port=%d", ovmsRESTPort),
	)
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
