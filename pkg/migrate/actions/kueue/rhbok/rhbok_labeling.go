package rhbok

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	kueuediscovery "github.com/opendatahub-io/odh-cli/pkg/lint/checks/kueue/discovery"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

type labelingPlan struct {
	namespaces []string
	workloads  []workloadRef
}

type workloadRef struct {
	resourceType resources.ResourceType
	object       *unstructured.Unstructured
	queueName    string
}

func (a *RHBOKMigrationAction) discoverLabelingPlan(
	ctx context.Context,
	target action.Target,
) (labelingPlan, error) {
	plan := labelingPlan{}

	enabled, err := kueuediscovery.KueueEnabledNamespaces(ctx, target.Client)
	if err != nil {
		return plan, fmt.Errorf("listing kueue-enabled namespaces: %w", err)
	}

	workloadNS, err := kueuediscovery.WorkloadLabeledNamespaces(ctx, target.Client)
	if err != nil {
		return plan, fmt.Errorf("listing workload-labeled namespaces: %w", err)
	}

	localQueueNS, err := a.namespacesWithLocalQueues(ctx, target)
	if err != nil {
		return plan, err
	}

	candidates := enabled.Union(workloadNS).Union(localQueueNS)

	for ns := range candidates {
		labeled, err := a.namespaceHasOpenshiftManagedLabel(ctx, target, ns)
		if err != nil {
			return plan, fmt.Errorf("checking namespace %s: %w", ns, err)
		}

		if labeled {
			continue
		}

		plan.namespaces = append(plan.namespaces, ns)
	}

	managedNS := enabled.Union(workloadNS)
	for _, ns := range plan.namespaces {
		managedNS.Insert(ns)
	}

	for ns := range managedNS {
		queueName, err := a.queueNameForNamespace(ctx, target, ns)
		if err != nil {
			return plan, err
		}

		for _, rt := range kueuediscovery.MonitoredWorkloadTypes {
			items, err := target.Client.ListResources(ctx, rt.GVR(), client.WithNamespace(ns))
			if err != nil {
				if client.IsResourceTypeNotFound(err) {
					continue
				}

				return plan, fmt.Errorf("listing %s in %s: %w", rt.Kind, ns, err)
			}

			for _, item := range items {
				if hasQueueNameLabel(item) {
					continue
				}

				plan.workloads = append(plan.workloads, workloadRef{
					resourceType: rt,
					object:       item,
					queueName:    queueName,
				})
			}
		}
	}

	return plan, nil
}

func (a *RHBOKMigrationAction) clearExecuteLabelingPlan() {
	a.executeLabelingPlan = nil
}

func (a *RHBOKMigrationAction) discoverExecuteLabelingPlan(
	ctx context.Context,
	target action.Target,
) (labelingPlan, error) {
	if a.executeLabelingPlan != nil {
		return *a.executeLabelingPlan, nil
	}

	plan, err := a.discoverLabelingPlan(ctx, target)
	if err != nil {
		return plan, err
	}

	a.executeLabelingPlan = &plan

	return plan, nil
}

func (a *RHBOKMigrationAction) reportLabelingPlan(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"report-labeling-plan",
		"Report namespaces and workloads requiring labels",
	)

	plan, err := a.discoverLabelingPlan(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover labeling targets: %v", err)

		return
	}

	step.Completef(result.StepCompleted,
		"Will label %d namespace(s) and %d workload(s)",
		len(plan.namespaces), len(plan.workloads))
}

func (a *RHBOKMigrationAction) labelKueueNamespaces(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"label-kueue-namespaces",
		"Apply kueue.openshift.io/managed=true to namespaces",
	)

	plan, err := a.discoverExecuteLabelingPlan(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover namespaces: %v", err)

		return
	}

	if len(plan.namespaces) == 0 {
		step.Completef(result.StepCompleted, "No namespaces require labeling")

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would label %d namespace(s)", len(plan.namespaces))

		return
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to label %d namespace(s) with %s=true", len(plan.namespaces), constants.LabelKueueOpenshiftManaged)
		if !confirmation.Prompt(target.IO, "Proceed with namespace labeling?") {
			step.Completef(result.StepSkipped, "User cancelled namespace labeling")

			return
		}
	}

	success := 0

	for _, ns := range plan.namespaces {
		if err := a.patchNamespaceLabel(ctx, target, ns); err != nil {
			step.Child("label-"+ns, "Label namespace "+ns).
				Completef(result.StepFailed, "Failed: %v", err)

			continue
		}

		success++
	}

	step.Completef(result.StepCompleted, "Labeled %d/%d namespace(s)", success, len(plan.namespaces))
}

func (a *RHBOKMigrationAction) labelKueueWorkloads(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"label-kueue-workloads",
		"Apply kueue.x-k8s.io/queue-name to workloads",
	)

	plan, err := a.discoverExecuteLabelingPlan(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover workloads: %v", err)

		return
	}

	if len(plan.workloads) == 0 {
		step.Completef(result.StepCompleted, "No workloads require queue-name labels")

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would label %d workload(s)", len(plan.workloads))

		return
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to add queue-name labels to %d workload(s)", len(plan.workloads))
		if !confirmation.Prompt(target.IO, "Proceed with workload labeling?") {
			step.Completef(result.StepSkipped, "User cancelled workload labeling")

			return
		}
	}

	success := 0

	for _, wl := range plan.workloads {
		if err := a.patchWorkloadQueueLabel(ctx, target, wl); err != nil {
			step.Child(
				fmt.Sprintf("label-%s-%s", wl.object.GetNamespace(), wl.object.GetName()),
				fmt.Sprintf("Label %s %s/%s", wl.resourceType.Kind, wl.object.GetNamespace(), wl.object.GetName()),
			).Completef(result.StepFailed, "Failed: %v", err)

			continue
		}

		success++
	}

	step.Completef(result.StepCompleted, "Labeled %d/%d workload(s)", success, len(plan.workloads))
}

func (a *RHBOKMigrationAction) namespacesWithLocalQueues(
	ctx context.Context,
	target action.Target,
) (sets.Set[string], error) {
	namespaces := sets.New[string]()

	items, err := target.Client.ListResources(ctx, resources.LocalQueue.GVR())
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return namespaces, nil
		}

		return nil, fmt.Errorf("listing LocalQueues: %w", err)
	}

	for _, item := range items {
		if ns := item.GetNamespace(); ns != "" {
			namespaces.Insert(ns)
		}
	}

	return namespaces, nil
}

func (a *RHBOKMigrationAction) namespaceHasOpenshiftManagedLabel(
	ctx context.Context,
	target action.Target,
	name string,
) (bool, error) {
	ns, err := target.Client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("getting namespace %s: %w", name, err)
	}

	return ns.Labels[constants.LabelKueueOpenshiftManaged] == "true", nil
}

func (a *RHBOKMigrationAction) queueNameForNamespace(
	ctx context.Context,
	target action.Target,
	namespace string,
) (string, error) {
	if a.QueueName != "" && a.QueueName != defaultQueueName {
		return a.workloadQueueName(), nil
	}

	items, err := target.Client.ListResources(ctx, resources.LocalQueue.GVR(), client.WithNamespace(namespace))
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return a.workloadQueueName(), nil
		}

		return "", fmt.Errorf("listing LocalQueues in %s: %w", namespace, err)
	}

	if len(items) == 1 {
		return items[0].GetName(), nil
	}

	return a.workloadQueueName(), nil
}

func hasQueueNameLabel(obj *unstructured.Unstructured) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}

	_, exists := labels[constants.LabelKueueQueueName]

	return exists
}

func (a *RHBOKMigrationAction) patchNamespaceLabel(
	ctx context.Context,
	target action.Target,
	name string,
) error {
	patch := map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{
				constants.LabelKueueOpenshiftManaged: "true",
			},
		},
	}

	patchData, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}

	_, err = target.Client.CoreV1().Namespaces().Patch(ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patching namespace %s: %w", name, err)
	}

	return nil
}

func (a *RHBOKMigrationAction) patchWorkloadQueueLabel(
	ctx context.Context,
	target action.Target,
	wl workloadRef,
) error {
	patch := map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{
				constants.LabelKueueQueueName: wl.queueName,
			},
		},
	}

	patchData, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}

	_, err = target.Client.Dynamic().Resource(wl.resourceType.GVR()).
		Namespace(wl.object.GetNamespace()).
		Patch(ctx, wl.object.GetName(), types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patching %s/%s: %w", wl.object.GetNamespace(), wl.object.GetName(), err)
	}

	return nil
}
