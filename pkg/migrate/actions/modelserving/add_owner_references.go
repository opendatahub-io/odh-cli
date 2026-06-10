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

	msgOwnerRefNoISVCs     = "No InferenceServices found with deploymentMode=RawDeployment"
	msgOwnerRefFoundISVCs  = "Found %d InferenceServices with deploymentMode=RawDeployment"
	msgOwnerRefPatched     = "Patched %s %s/%s with ownerReference to %s"
	msgOwnerRefPatchDryRun = "Would patch %s %s/%s with ownerReference to %s"
	msgOwnerRefPatchFailed = "Failed to patch %s %s/%s: %v"
	msgOwnerRefNotFound    = "%s %s/%s not found (skipped)"
	msgOwnerRefComplete    = "Processed owner references for %d InferenceService(s)"
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

	patchedCount := 0

	for _, isvc := range isvcs {
		isvcStep := step.Child(
			fmt.Sprintf("isvc-%s-%s", isvc.GetNamespace(), isvc.GetName()),
			fmt.Sprintf("Add owner references for %s/%s", isvc.GetNamespace(), isvc.GetName()),
		)

		a.patchAuthResourceOwnerRefs(ctx, target, isvc, isvcStep)
		isvcStep.Complete(result.StepCompleted, "Processed owner references for %s/%s", isvc.GetNamespace(), isvc.GetName())
		patchedCount++
	}

	step.Completef(result.StepCompleted, msgOwnerRefComplete, patchedCount)
}

func (a *AddOwnerReferencesAction) patchAuthResourceOwnerRefs(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	step action.StepRecorder,
) {
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

		return
	}

	// Patch ServiceAccount
	a.patchResourceOwnerRef(ctx, target, resources.ServiceAccount, ns, name+authSASuffix, name, ownerRefPatch, step)

	// Patch Role
	a.patchResourceOwnerRef(ctx, target, resources.Role, ns, name+authRoleSuffix, name, ownerRefPatch, step)

	// Patch RoleBinding
	a.patchResourceOwnerRef(ctx, target, resources.RoleBinding, ns, name+authRoleBindingSuffix, name, ownerRefPatch, step)
}

func (a *AddOwnerReferencesAction) patchResourceOwnerRef(
	ctx context.Context,
	target action.Target,
	resourceType resources.ResourceType,
	namespace string,
	resourceName string,
	isvcName string,
	patchData []byte,
	step action.StepRecorder,
) {
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

			return
		}

		step.Recordf(
			"patch-"+resourceName,
			msgOwnerRefPatchFailed,
			result.StepFailed,
			resourceType.Kind, namespace, resourceName, err,
		)

		return
	}

	if target.DryRun {
		step.Recordf(
			"patch-"+resourceName,
			msgOwnerRefPatchDryRun,
			result.StepSkipped,
			resourceType.Kind, namespace, resourceName, isvcName,
		)

		return
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

		return
	}

	step.Recordf(
		"patch-"+resourceName,
		msgOwnerRefPatched,
		result.StepCompleted,
		resourceType.Kind, namespace, resourceName, isvcName,
	)
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
