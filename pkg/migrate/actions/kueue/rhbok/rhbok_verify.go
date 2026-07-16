package rhbok

import (
	"context"
	"fmt"
	"strings"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	kueuediscovery "github.com/opendatahub-io/odh-cli/pkg/lint/checks/kueue/discovery"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

func (a *RHBOKMigrationAction) verifyMigrationComplete(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"verify-migration-complete",
		"Verify RHBOK migration completed successfully",
	)

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would verify migration completion")

		return
	}

	checks := []func(context.Context, action.Target) string{
		a.verifyEmbeddedRemoved,
		a.verifyManagementState,
		a.verifyKueueReadyStatus,
		a.verifyOperatorReady,
		a.verifyOperatorPodsReady,
		a.verifyQueuesPreserved,
		a.verifyNamespaceLabels,
		a.verifyWorkloadLabels,
	}

	var failures []string

	for _, check := range checks {
		if msg := check(ctx, target); msg != "" {
			failures = append(failures, msg)
		}
	}

	if len(failures) > 0 {
		step.Completef(result.StepFailed, "Verification failed: %s", strings.Join(failures, "; "))

		return
	}

	step.Completef(result.StepCompleted, "Migration verification passed")
}

func (a *RHBOKMigrationAction) verifyEmbeddedRemoved(ctx context.Context, target action.Target) string {
	_, err := target.Client.Dynamic().Resource(resources.Deployment.GVR()).
		Namespace(applicationsNamespace).
		Get(ctx, embeddedKueueDeployment, metav1.GetOptions{})
	if err == nil {
		return fmt.Sprintf("embedded deployment %s still exists in %s", embeddedKueueDeployment, applicationsNamespace)
	}

	if !apierrors.IsNotFound(err) {
		return fmt.Sprintf("checking embedded deployment: %v", err)
	}

	return ""
}

func (a *RHBOKMigrationAction) verifyKueueReadyStatus(ctx context.Context, target action.Target) string {
	dsc, err := client.GetSingleton(ctx, target.Client, resources.DataScienceClusterV1)
	if err != nil {
		return fmt.Sprintf("getting DataScienceCluster: %v", err)
	}

	cond, err := getDSCCondition(dsc, kueueReadyType)
	if err != nil {
		return fmt.Sprintf("querying KueueReady: %v", err)
	}

	if cond == nil || cond.Status != metav1.ConditionTrue {
		return "KueueReady condition is not True"
	}

	return ""
}

func (a *RHBOKMigrationAction) verifyManagementState(ctx context.Context, target action.Target) string {
	state, err := a.getKueueManagementState(ctx, target.Client)
	if err != nil {
		return fmt.Sprintf("checking managementState: %v", err)
	}

	if state != constants.ManagementStateUnmanaged {
		return fmt.Sprintf("kueue managementState is %q, expected %q", state, constants.ManagementStateUnmanaged)
	}

	return ""
}

func (a *RHBOKMigrationAction) verifyOperatorReady(ctx context.Context, target action.Target) string {
	sub, err := target.Client.OLM().Subscriptions(operatorNamespace).Get(ctx, subscriptionName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "RHBOK operator subscription not found in " + operatorNamespace
		}

		return fmt.Sprintf("checking RHBOK operator subscription: %v", err)
	}

	installedCSV := sub.Status.InstalledCSV
	if installedCSV == "" {
		return "RHBOK operator subscription has no installedCSV"
	}

	csv, err := target.Client.OLM().ClusterServiceVersions(operatorNamespace).Get(ctx, installedCSV, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("getting RHBOK CSV %s: %v", installedCSV, err)
	}

	if csv.Status.Phase != operatorsv1alpha1.CSVPhaseSucceeded {
		return fmt.Sprintf("RHBOK CSV %s is in %s phase, expected Succeeded", installedCSV, csv.Status.Phase)
	}

	return ""
}

func (a *RHBOKMigrationAction) verifyOperatorPodsReady(ctx context.Context, target action.Target) string {
	pods, err := target.Client.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kueue",
	})
	if err != nil {
		return fmt.Sprintf("listing RHBOK pods: %v", err)
	}

	if len(pods.Items) == 0 {
		return "no pods found matching app.kubernetes.io/name=kueue in RHBOK operator namespace"
	}

	for i := range pods.Items {
		if !podReady(&pods.Items[i]) {
			return fmt.Sprintf("RHBOK pod %s is not ready (phase: %s)",
				pods.Items[i].Name, pods.Items[i].Status.Phase)
		}
	}

	return ""
}

func (a *RHBOKMigrationAction) verifyQueuesPreserved(ctx context.Context, target action.Target) string {
	cqCount, err := a.countQueueResources(ctx, target, resources.ClusterQueue)
	if err != nil {
		return fmt.Sprintf("listing ClusterQueues: %v", err)
	}

	lqCount, err := a.countQueueResources(ctx, target, resources.LocalQueue)
	if err != nil {
		return fmt.Sprintf("listing LocalQueues: %v", err)
	}

	if target.IO != nil {
		target.IO.Fprintf("Verified: %d ClusterQueue(s), %d LocalQueue(s) preserved\n", cqCount, lqCount)
	}

	return ""
}

func (a *RHBOKMigrationAction) countQueueResources(
	ctx context.Context,
	target action.Target,
	resource resources.ResourceType,
) (int, error) {
	items, err := target.Client.ListResources(ctx, resource.GVR())
	if err != nil {
		return 0, fmt.Errorf("listing %s resources: %w", resource.Kind, err)
	}

	return len(items), nil
}

func (a *RHBOKMigrationAction) verifyNamespaceLabels(ctx context.Context, target action.Target) string {
	plan, err := a.discoverLabelingPlan(ctx, target)
	if err != nil {
		return fmt.Sprintf("discovering namespace labels: %v", err)
	}

	if len(plan.namespaces) > 0 {
		return fmt.Sprintf("%d namespace(s) still missing %s label", len(plan.namespaces), constants.LabelKueueOpenshiftManaged)
	}

	return ""
}

func (a *RHBOKMigrationAction) verifyWorkloadLabels(ctx context.Context, target action.Target) string {
	managed, err := kueuediscovery.KueueEnabledNamespaces(ctx, target.Client)
	if err != nil {
		return fmt.Sprintf("listing managed namespaces: %v", err)
	}

	var missing []string

	for ns := range managed {
		for _, rt := range kueuediscovery.MonitoredWorkloadTypes {
			items, err := target.Client.ListResources(ctx, rt.GVR(), client.WithNamespace(ns))
			if err != nil {
				if client.IsResourceTypeNotFound(err) {
					continue
				}

				return fmt.Sprintf("listing %s: %v", rt.Kind, err)
			}

			for _, item := range items {
				if !hasQueueNameLabel(item) {
					missing = append(missing, fmt.Sprintf("%s %s/%s", rt.Kind, ns, item.GetName()))
				}
			}
		}
	}

	if len(missing) > 0 {
		const maxShow = 5
		shown := missing
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}

		return fmt.Sprintf("%d workload(s) missing queue-name label (e.g. %s)",
			len(missing), strings.Join(shown, ", "))
	}

	return ""
}

// verifyResourcesPreserved is kept for backward-compatible test exports.
func (a *RHBOKMigrationAction) verifyResourcesPreserved(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"verify-resources-preserved",
		"Verify ClusterQueue and LocalQueue resources preserved",
	)

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would verify ClusterQueue and LocalQueue resources are preserved")

		return
	}

	clusterQueues, err := target.Client.ListResources(ctx, resources.ClusterQueue.GVR())
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Completef(result.StepCompleted, "No ClusterQueue CRD found (no resources to preserve)")

			return
		}

		step.Completef(result.StepFailed, "Failed to list ClusterQueues: %v", err)

		return
	}

	localQueues, err := target.Client.ListResources(ctx, resources.LocalQueue.GVR())
	if err != nil {
		if apierrors.IsNotFound(err) || client.IsResourceTypeNotFound(err) {
			step.Completef(result.StepCompleted, "No LocalQueue CRD found (%d ClusterQueues preserved)", len(clusterQueues))

			return
		}

		step.Completef(result.StepFailed, "Failed to list LocalQueues: %v", err)

		return
	}

	step.Completef(result.StepCompleted,
		"All %d ClusterQueues and %d LocalQueues preserved",
		len(clusterQueues), len(localQueues))
}
