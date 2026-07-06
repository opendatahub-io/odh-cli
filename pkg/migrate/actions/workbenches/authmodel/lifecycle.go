package authmodel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	kueuediscovery "github.com/opendatahub-io/odh-cli/pkg/lint/checks/kueue/discovery"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	stopWaitTimeout  = 120 * time.Second
	stopWaitInterval = 2 * time.Second
)

// stopWorkbench annotates a notebook with kubeflow-resource-stopped and waits
// for the StatefulSet to scale down. Returns an error if stopping fails.
func stopWorkbench(
	ctx context.Context,
	target action.Target,
	nb *unstructured.Unstructured,
	step action.StepRecorder,
) error {
	name := nb.GetName()
	namespace := nb.GetNamespace()

	timestamp := time.Now().UTC().Format(time.RFC3339)

	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{
				annotationKubeflowResourceStopped: timestamp,
			},
		},
	}

	patchData, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling stop patch: %w", err)
	}

	_, err = target.Client.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace(namespace).
		Patch(ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("annotating notebook %s/%s for stop: %w", namespace, name, err)
	}

	step.Recordf(
		fmt.Sprintf("stop-%s-%s", namespace, name),
		"Stopped %s/%s, waiting for scale-down",
		result.StepCompleted,
		namespace, name,
	)

	if err := waitForStatefulSetScaleDown(ctx, target, name, namespace); err != nil {
		step.Recordf(
			fmt.Sprintf("wait-%s-%s", namespace, name),
			"Timed out waiting for StatefulSet %s/%s to scale down: %v",
			result.StepFailed,
			namespace, name, err,
		)

		return err
	}

	return nil
}

// waitForStatefulSetScaleDown polls until the StatefulSet has 0 replicas or is deleted.
func waitForStatefulSetScaleDown(
	ctx context.Context,
	target action.Target,
	name string,
	namespace string,
) error {
	err := wait.PollUntilContextTimeout(ctx, stopWaitInterval, stopWaitTimeout, true,
		func(ctx context.Context) (bool, error) {
			sts, err := target.Client.Dynamic().Resource(resources.StatefulSet.GVR()).
				Namespace(namespace).
				Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}

			if err != nil {
				return false, fmt.Errorf("getting statefulset %s/%s: %w", namespace, name, err)
			}

			replicas, found, err := unstructured.NestedInt64(sts.Object, "spec", "replicas")
			if err != nil {
				return false, fmt.Errorf("reading spec.replicas: %w", err)
			}

			if !found || replicas == 0 {
				return true, nil
			}

			return false, nil
		})
	if err != nil {
		return fmt.Errorf("waiting for statefulset %s/%s scale-down: %w", namespace, name, err)
	}

	return nil
}

// restartWorkbench removes the kubeflow-resource-stopped annotation to restart a notebook.
func restartWorkbench(
	ctx context.Context,
	target action.Target,
	nb *unstructured.Unstructured,
	step action.StepRecorder,
) {
	name := nb.GetName()
	namespace := nb.GetNamespace()

	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{
				annotationKubeflowResourceStopped: nil,
			},
		},
	}

	patchData, err := json.Marshal(patch)
	if err != nil {
		step.Recordf(
			fmt.Sprintf("restart-%s-%s", namespace, name),
			"Failed to marshal restart patch for %s/%s: %v",
			result.StepFailed,
			namespace, name, err,
		)

		return
	}

	_, err = target.Client.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace(namespace).
		Patch(ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		step.Recordf(
			fmt.Sprintf("restart-%s-%s", namespace, name),
			"Failed to restart %s/%s: %v",
			result.StepFailed,
			namespace, name, err,
		)

		return
	}

	step.Recordf(
		fmt.Sprintf("restart-%s-%s", namespace, name),
		"Restarted %s/%s",
		result.StepCompleted,
		namespace, name,
	)
}

// deleteStatefulSet deletes the StatefulSet associated with a notebook. IsNotFound is not an error.
// Returns true when the StatefulSet was successfully deleted or already absent.
func deleteStatefulSet(
	ctx context.Context,
	target action.Target,
	name string,
	namespace string,
	step action.StepRecorder,
) bool {
	err := target.Client.Dynamic().Resource(resources.StatefulSet.GVR()).
		Namespace(namespace).
		Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		step.Recordf(
			fmt.Sprintf("delete-sts-%s-%s", namespace, name),
			"StatefulSet %s/%s already absent",
			result.StepCompleted,
			namespace, name,
		)

		return true
	}

	if err != nil {
		step.Recordf(
			fmt.Sprintf("delete-sts-%s-%s", namespace, name),
			"Failed to delete StatefulSet %s/%s: %v",
			result.StepFailed,
			namespace, name, err,
		)

		return false
	}

	step.Recordf(
		fmt.Sprintf("delete-sts-%s-%s", namespace, name),
		"Deleted StatefulSet %s/%s",
		result.StepCompleted,
		namespace, name,
	)

	return true
}

// checkKueueTerminatingPods detects pods stuck in Terminating state in
// Kueue-managed namespaces and reports them with remediation commands.
func checkKueueTerminatingPods(
	ctx context.Context,
	target action.Target,
	notebooks []*unstructured.Unstructured,
	step action.StepRecorder,
) bool {
	kueueNamespaces, err := kueuediscovery.KueueEnabledNamespaces(ctx, target.Client)
	if err != nil {
		step.Recordf("kueue-check",
			"Failed to discover Kueue-managed namespaces: %v",
			result.StepFailed, err)

		return true
	}

	if kueueNamespaces.Len() == 0 {
		return false
	}

	var kueueNotebooks []*unstructured.Unstructured

	for _, nb := range notebooks {
		if kueueNamespaces.Has(nb.GetNamespace()) {
			kueueNotebooks = append(kueueNotebooks, nb)
		}
	}

	if len(kueueNotebooks) == 0 {
		return false
	}

	var stuckPods []string

	for _, nb := range kueueNotebooks {
		podName := nb.GetName() + "-0"
		namespace := nb.GetNamespace()

		pod, err := target.Client.Dynamic().Resource(resources.Pod.GVR()).
			Namespace(namespace).
			Get(ctx, podName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}

		if err != nil {
			continue
		}

		if pod.GetDeletionTimestamp() != nil {
			stuckPods = append(stuckPods, fmt.Sprintf("%s/%s", namespace, podName))
		}
	}

	if len(stuckPods) == 0 {
		step.Recordf("kueue-check",
			"No pods stuck in Terminating state in Kueue-managed namespaces",
			result.StepCompleted)

		return false
	}

	target.IO.Fprintln()
	target.IO.Errorf("WARNING: Kueue Finalizer Conflicts detected")
	target.IO.Errorf("The following pods are stuck in Terminating state:")

	for _, pod := range stuckPods {
		target.IO.Errorf("  - %s", pod)
	}

	target.IO.Errorf("")
	target.IO.Errorf("To resolve, remove the finalizer from the affected pod:")
	target.IO.Errorf("  oc patch pod <pod-name> -n <namespace> -p '{\"metadata\":{\"finalizers\":null}}' --type=merge")

	step.Recordf("kueue-check",
		"Found %d pod(s) stuck in Terminating state in Kueue-managed namespaces",
		result.StepFailed, len(stuckPods))

	return true
}
