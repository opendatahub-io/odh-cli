package modelserving

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	addOwnerRefsActionID          = "modelserving.add-owner-references"
	addOwnerRefsActionName        = "Add owner references to auth resources"
	addOwnerRefsActionDescription = "Patches ServiceAccounts, Roles, and RoleBindings associated with RawDeployment InferenceServices with ownerReferences pointing to the ISVC"

	msgOwnerRefNoISVCs        = "No InferenceServices found with deploymentMode=RawDeployment"
	msgOwnerRefFoundISVCs     = "Found %d InferenceServices with deploymentMode=RawDeployment"
	msgOwnerRefPatched        = "Patched %s %s/%s with ownerReference to %s"
	msgOwnerRefPatchDryRun    = "Would patch %s %s/%s with ownerReference to %s"
	msgOwnerRefPatchFailed    = "Failed to patch %s %s/%s: %v"
	msgOwnerRefNotFound       = "%s %s/%s not found (skipped)"
	msgOwnerRefComplete       = "Processed owner references for %d InferenceService(s)"
	msgOwnerRefPartialFailure = "Processed owner references for %d/%d InferenceService(s); %d failed"
	msgOwnerRefISVCProcessed  = "Processed owner references for %s/%s"
)

// AddOwnerReferencesAction patches auth resources (SA, Role, RoleBinding) with
// ownerReferences pointing to the associated InferenceService.
type AddOwnerReferencesAction struct{}

func (a *AddOwnerReferencesAction) ID() string {
	return addOwnerRefsActionID
}

func (a *AddOwnerReferencesAction) Name() string {
	return addOwnerRefsActionName
}

func (a *AddOwnerReferencesAction) Description() string {
	return addOwnerRefsActionDescription
}

func (a *AddOwnerReferencesAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *AddOwnerReferencesAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func (a *AddOwnerReferencesAction) CanApply(target action.Target) bool {
	return target.CurrentVersion.Major == 2 && target.CurrentVersion.Minor >= 25
}

func (a *AddOwnerReferencesAction) Prepare() action.Task {
	return nil
}

func (a *AddOwnerReferencesAction) Run() action.Task {
	return &addOwnerRefsRunTask{action: a}
}

func (a *AddOwnerReferencesAction) addOwnerReferences(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"add-owner-references",
		"Add owner references to auth resources",
	)

	isvcs, err := listISVCsByDeploymentMode(ctx, target, deploymentModeRawDeployment)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list InferenceServices: %v", err)

		return
	}

	if len(isvcs) == 0 {
		step.Completef(result.StepSkipped, msgOwnerRefNoISVCs)

		return
	}

	step.Recordf("list-isvcs", msgOwnerRefFoundISVCs, result.StepCompleted, len(isvcs))

	processedCount := 0
	failedCount := 0

	for _, isvc := range isvcs {
		isvcStep := step.Child(
			fmt.Sprintf("isvc-%s-%s", isvc.GetNamespace(), isvc.GetName()),
			fmt.Sprintf("Add owner references for %s/%s", isvc.GetNamespace(), isvc.GetName()),
		)

		// patchAuthResourceOwnerRefs already sets a failing condition on isvcStep
		// when a patch or marshal error occurs; only mark the step completed here
		// when every underlying patch succeeded, so failures are never overwritten.
		if !a.patchAuthResourceOwnerRefs(ctx, target, isvc, isvcStep) {
			failedCount++

			continue
		}

		isvcStep.Completef(result.StepCompleted, msgOwnerRefISVCProcessed, isvc.GetNamespace(), isvc.GetName())
		processedCount++
	}

	switch {
	case failedCount > 0:
		step.Completef(result.StepFailed, msgOwnerRefPartialFailure, processedCount, len(isvcs), failedCount)
	default:
		step.Completef(result.StepCompleted, msgOwnerRefComplete, processedCount)
	}
}

// patchAuthResourceOwnerRefs patches the ServiceAccount, Role, and RoleBinding
// associated with isvc. It returns false if the patch payload could not be
// built or if any of the three resource patches failed, so callers must not
// mark the parent step completed on top of a failing condition.
func (a *AddOwnerReferencesAction) patchAuthResourceOwnerRefs(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	step action.StepRecorder,
) bool {
	name := isvc.GetName()
	ns := isvc.GetNamespace()
	uid := isvc.GetUID()

	ownerRef := map[string]any{
		"apiVersion":         resources.InferenceService.APIVersion(),
		"kind":               resources.InferenceService.Kind,
		"name":               name,
		"uid":                string(uid),
		"blockOwnerDeletion": false,
	}

	ownerRefPatch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"ownerReferences": []any{ownerRef},
		},
	})
	if err != nil {
		step.Completef(result.StepFailed, "Failed to marshal owner reference patch: %v", err)

		return false
	}

	// Patch ServiceAccount
	success := a.patchResourceOwnerRef(ctx, target, resources.ServiceAccount, ns, name+authSASuffix, name, ownerRefPatch, step)

	// Patch Role
	if !a.patchResourceOwnerRef(ctx, target, resources.Role, ns, name+authRoleSuffix, name, ownerRefPatch, step) {
		success = false
	}

	// Patch RoleBinding
	if !a.patchResourceOwnerRef(ctx, target, resources.RoleBinding, ns, name+authRoleBindingSuffix, name, ownerRefPatch, step) {
		success = false
	}

	return success
}

// patchResourceOwnerRef patches a single auth resource with an ownerReference.
// It returns true for success, skip (resource not found), and dry-run cases,
// and false only when the resource lookup or patch call actually fails.
func (a *AddOwnerReferencesAction) patchResourceOwnerRef(
	ctx context.Context,
	target action.Target,
	resourceType resources.ResourceType,
	namespace string,
	resourceName string,
	isvcName string,
	patchData []byte,
	step action.StepRecorder,
) bool {
	// Check if resource exists first
	_, err := target.Client.Dynamic().Resource(resourceType.GVR()).
		Namespace(namespace).
		Get(ctx, resourceName, metav1.GetOptions{})

	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Recordf(
				"patch-"+resourceName,
				msgOwnerRefNotFound,
				result.StepSkipped,
				resourceType.Kind, namespace, resourceName,
			)

			return true
		}

		step.Recordf(
			"patch-"+resourceName,
			msgOwnerRefPatchFailed,
			result.StepFailed,
			resourceType.Kind, namespace, resourceName, err,
		)

		return false
	}

	if target.DryRun {
		step.Recordf(
			"patch-"+resourceName,
			msgOwnerRefPatchDryRun,
			result.StepSkipped,
			resourceType.Kind, namespace, resourceName, isvcName,
		)

		return true
	}

	_, err = target.Client.Dynamic().Resource(resourceType.GVR()).
		Namespace(namespace).
		Patch(ctx, resourceName, types.MergePatchType, patchData, metav1.PatchOptions{})

	if err != nil {
		step.Recordf(
			"patch-"+resourceName,
			msgOwnerRefPatchFailed,
			result.StepFailed,
			resourceType.Kind, namespace, resourceName, err,
		)

		return false
	}

	step.Recordf(
		"patch-"+resourceName,
		msgOwnerRefPatched,
		result.StepCompleted,
		resourceType.Kind, namespace, resourceName, isvcName,
	)

	return true
}

// --- Run Task ---

type addOwnerRefsRunTask struct {
	action *AddOwnerReferencesAction
}

func (t *addOwnerRefsRunTask) Validate(
	_ context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	return action.BuildResult(target)
}

func (t *addOwnerRefsRunTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	t.action.addOwnerReferences(ctx, target)

	return action.BuildResult(target)
}
