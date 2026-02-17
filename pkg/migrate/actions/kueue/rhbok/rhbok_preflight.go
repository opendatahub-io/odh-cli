package rhbok

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

func (a *RHBOKMigrationAction) checkCurrentKueueState(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"check-kueue-state",
		"Verify current Kueue state",
	)

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Complete(result.StepFailed, "DataScienceCluster not found - OpenShift AI may not be installed")

			return
		}

		step.Complete(result.StepFailed, "Failed to get DataScienceCluster: %v", err)

		return
	}

	managementState, err := jq.Query[string](dsc, ".spec.components.kueue.managementState")
	if err != nil {
		step.Complete(result.StepFailed, "Failed to query Kueue managementState: %v", err)

		return
	}

	if managementState == "" {
		step.Complete(result.StepFailed, "Kueue component not configured in DataScienceCluster")

		return
	}

	step.Complete(result.StepCompleted, "Current Kueue state verified (managementState: %s)", managementState)
}

func (a *RHBOKMigrationAction) checkNoRHBOKConflicts(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"check-rhbok-conflicts",
		"Check for RHBOK operator conflicts",
	)

	subscription, err := target.Client.Dynamic().Resource(resources.Subscription.GVR()).
		Namespace("openshift-kueue-operator").
		Get(ctx, "kueue-operator", metav1.GetOptions{})

	if err == nil && subscription != nil {
		step.Complete(result.StepCompleted, "RHBOK operator already installed - migration may be partially complete")

		return
	}

	if !apierrors.IsNotFound(err) {
		step.Complete(result.StepFailed, "Failed to check RHBOK subscription: %v", err)

		return
	}

	step.Complete(result.StepCompleted, "No RHBOK conflicts detected")
}

func (a *RHBOKMigrationAction) verifyKueueResources(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"verify-kueue-resources",
		"Verify Kueue resources exist",
	)

	clusterQueues, err := target.Client.ListResources(ctx, resources.ClusterQueue.GVR())
	if err != nil {
		step.Complete(result.StepFailed, "Failed to list ClusterQueues: %v", err)

		return
	}

	localQueues, err := target.Client.ListResources(ctx, resources.LocalQueue.GVR())
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Complete(result.StepCompleted,
				"Kueue resources found: %d ClusterQueues (LocalQueue CRD not found)",
				len(clusterQueues))

			return
		}

		step.Complete(result.StepFailed, "Failed to list LocalQueues: %v", err)

		return
	}

	step.Complete(result.StepCompleted,
		"Kueue resources found: %d ClusterQueues, %d LocalQueues",
		len(clusterQueues), len(localQueues))
}
