package rhbok

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/migrate/action"
	"github.com/lburgazzoli/odh-cli/pkg/migrate/action/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

func (a *RHBOKMigrationAction) checkCurrentKueueState(
	ctx context.Context,
	target *action.ActionTarget,
) result.ActionStep {
	step := result.NewStep(
		"check-kueue-state",
		"Verify current Kueue state",
		result.StepRunning,
		"",
	)

	dsc, err := target.Client.GetDataScienceCluster(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Status = result.StepFailed
			step.Message = "DataScienceCluster not found - OpenShift AI may not be installed"

			return step
		}

		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Failed to get DataScienceCluster: %v", err)

		return step
	}

	managementState, err := jq.Query[string](dsc, ".spec.components.kueue.managementState")
	if err != nil {
		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Failed to query Kueue managementState: %v", err)

		return step
	}

	if managementState == "" {
		step.Status = result.StepFailed
		step.Message = "Kueue component not configured in DataScienceCluster"

		return step
	}

	step.Status = result.StepCompleted
	step.Message = fmt.Sprintf("Current Kueue state verified (managementState: %s)", managementState)

	return step
}

func (a *RHBOKMigrationAction) checkNoRHBOKConflicts(
	ctx context.Context,
	target *action.ActionTarget,
) result.ActionStep {
	step := result.NewStep(
		"check-rhbok-conflicts",
		"Check for RHBOK operator conflicts",
		result.StepRunning,
		"",
	)

	subscription, err := target.Client.Dynamic.Resource(resources.Subscription.GVR()).
		Namespace("openshift-operators").
		Get(ctx, "rhods-kueue-operator", metav1.GetOptions{})

	if err == nil && subscription != nil {
		step.Status = result.StepCompleted
		step.Message = "RHBOK operator already installed - migration may be partially complete"

		return step
	}

	if !apierrors.IsNotFound(err) {
		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Failed to check RHBOK subscription: %v", err)

		return step
	}

	step.Status = result.StepCompleted
	step.Message = "No RHBOK conflicts detected"

	return step
}

func (a *RHBOKMigrationAction) verifyKueueResources(
	ctx context.Context,
	target *action.ActionTarget,
) result.ActionStep {
	step := result.NewStep(
		"verify-kueue-resources",
		"Verify Kueue resources exist",
		result.StepRunning,
		"",
	)

	clusterQueues, err := target.Client.ListResources(ctx, resources.ClusterQueue.GVR())
	if err != nil {
		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Failed to list ClusterQueues: %v", err)

		return step
	}

	localQueues, err := target.Client.ListResources(ctx, resources.LocalQueue.GVR())
	if err != nil {
		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Failed to list LocalQueues: %v", err)

		return step
	}

	step.Status = result.StepCompleted
	step.Message = fmt.Sprintf("Kueue resources found: %d ClusterQueues, %d LocalQueues",
		len(clusterQueues), len(localQueues))

	return step
}
