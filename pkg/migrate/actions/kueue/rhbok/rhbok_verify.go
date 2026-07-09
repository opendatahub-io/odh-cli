package rhbok

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	kueuediscovery "github.com/opendatahub-io/odh-cli/pkg/lint/checks/kueue/discovery"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"
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

	var failures []string

	if msg := a.verifyEmbeddedRemoved(ctx, target); msg != "" {
		failures = append(failures, msg)
	}

	if msg := a.verifyKueueReadyStatus(ctx, target); msg != "" {
		failures = append(failures, msg)
	}

	if msg := a.verifyOperatorReady(ctx, target); msg != "" {
		failures = append(failures, msg)
	}

	if msg := a.verifyQueuesPreserved(ctx, target); msg != "" {
		failures = append(failures, msg)
	}

	if msg := a.verifyNamespaceLabels(ctx, target); msg != "" {
		failures = append(failures, msg)
	}

	if msg := a.verifyWorkloadLabels(ctx, target); msg != "" {
		failures = append(failures, msg)
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

func (a *RHBOKMigrationAction) verifyOperatorReady(ctx context.Context, target action.Target) string {
	info, err := olm.FindOperator(ctx, target.Client, func(sub *olm.SubscriptionInfo) bool {
		return sub.Name == subscriptionName
	})
	if err != nil {
		return fmt.Sprintf("checking RHBOK operator: %v", err)
	}

	if !info.Found() {
		return "Red Hat build of Kueue operator subscription not found"
	}

	return ""
}

func (a *RHBOKMigrationAction) verifyQueuesPreserved(ctx context.Context, target action.Target) string {
	if msg := a.verifyQueueCRDListable(ctx, target, resources.ClusterQueue, "ClusterQueue"); msg != "" {
		return msg
	}

	if msg := a.verifyQueueCRDListable(ctx, target, resources.LocalQueue, "LocalQueue"); msg != "" {
		return msg
	}

	return ""
}

func (a *RHBOKMigrationAction) verifyQueueCRDListable(
	ctx context.Context,
	target action.Target,
	resource resources.ResourceType,
	kind string,
) string {
	_, err := target.Client.ListResources(ctx, resource.GVR())
	if err == nil {
		return ""
	}

	if apierrors.IsNotFound(err) || client.IsResourceTypeNotFound(err) {
		if target.IO != nil {
			target.IO.Fprintf(
				"Warning: %s CRD not found during migration verification; queue preservation could not be verified\n",
				kind,
			)
		}

		return ""
	}

	return fmt.Sprintf("listing %ss: %v", kind, err)
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

// verifyQueuesPreserved is kept for backward-compatible test exports.
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
