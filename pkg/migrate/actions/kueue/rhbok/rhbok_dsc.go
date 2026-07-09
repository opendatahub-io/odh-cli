package rhbok

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

func (a *RHBOKMigrationAction) removeEmbeddedKueue(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"remove-embedded-kueue",
		"Uninstall embedded Kueue by setting managementState to Removed",
	)

	if a.SkipRemoveEmbedded {
		step.Completef(result.StepSkipped,
			"Skipping embedded Kueue removal (--skip-remove-embedded); this may cause configuration conflicts")

		return
	}

	state, err := a.getKueueManagementState(ctx, target.Client)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to get DataScienceCluster: %v", err)

		return
	}

	if state == constants.ManagementStateRemoved || state == constants.ManagementStateUnmanaged {
		step.Completef(result.StepSkipped, "Embedded Kueue already removed (managementState=%s)", state)

		return
	}

	if state != constants.ManagementStateManaged {
		step.Completef(result.StepSkipped, "Kueue managementState is %s; removal not required", state)

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would set %s=%s", kueueComponentPath, constants.ManagementStateRemoved)

		return
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to set DataScienceCluster Kueue managementState to %s", constants.ManagementStateRemoved)
		if !confirmation.Prompt(target.IO, "Proceed with embedded Kueue removal?") {
			step.Completef(result.StepSkipped, "User cancelled removal")

			return
		}
		target.IO.Fprintln()
	}

	if err := a.patchKueueManagementState(ctx, target, constants.ManagementStateRemoved, nil); err != nil {
		step.Completef(result.StepFailed, "Failed to remove embedded Kueue: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "DataScienceCluster Kueue set to Removed")
}

func (a *RHBOKMigrationAction) activateRHBOK(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"activate-rhbok",
		"Activate Red Hat build of Kueue in DataScienceCluster",
	)

	state, err := a.getKueueManagementState(ctx, target.Client)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to get DataScienceCluster: %v", err)

		return
	}

	if state == constants.ManagementStateUnmanaged {
		step.Completef(result.StepSkipped, "DataScienceCluster Kueue already set to Unmanaged")

		return
	}

	queueNames := map[string]string{}
	if a.ClusterQueueName != "" {
		queueNames["defaultClusterQueueName"] = a.ClusterQueueName
	}

	if a.LocalQueueName != "" {
		queueNames["defaultLocalQueueName"] = a.LocalQueueName
	}

	if target.DryRun {
		msg := fmt.Sprintf("Would set %s=%s", kueueComponentPath, constants.ManagementStateUnmanaged)
		if len(queueNames) > 0 {
			msg += fmt.Sprintf(" with queue names %v", queueNames)
		}

		step.Completef(result.StepSkipped, "%s", msg)

		return
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to set DataScienceCluster Kueue managementState to %s", constants.ManagementStateUnmanaged)
		if !confirmation.Prompt(target.IO, "Proceed with RHBOK activation?") {
			step.Completef(result.StepSkipped, "User cancelled activation")

			return
		}
		target.IO.Fprintln()
	}

	if err := a.patchKueueManagementState(ctx, target, constants.ManagementStateUnmanaged, queueNames); err != nil {
		step.Completef(result.StepFailed, "Failed to activate RHBOK: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "DataScienceCluster updated to Unmanaged")
}

type kueueQueueNames map[string]string

func (a *RHBOKMigrationAction) patchKueueManagementState(
	ctx context.Context,
	target action.Target,
	state string,
	queueNames kueueQueueNames,
) error {
	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: retryInitialDuration,
		Factor:   retryFactor,
		Jitter:   retryJitter,
		Steps:    retryMaxSteps,
	}, func() (bool, error) {
		latestDSC, err := client.GetSingleton(ctx, target.Client, resources.DataScienceClusterV1)
		if err != nil {
			return false, fmt.Errorf("failed to get DataScienceCluster: %w", err)
		}

		if err := jq.Transform(latestDSC, ".spec.components.kueue.managementState = %q", state); err != nil {
			return false, fmt.Errorf("failed to set managementState: %w", err)
		}

		for field, value := range queueNames {
			if err := unstructured.SetNestedField(latestDSC.Object, value, "spec", "components", "kueue", field); err != nil {
				return false, fmt.Errorf("failed to set %s: %w", field, err)
			}
		}

		_, err = target.Client.Dynamic().Resource(resources.DataScienceClusterV1.GVR()).
			Update(ctx, latestDSC, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsConflict(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to update DataScienceCluster: %w", err)
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("updating kueue management state: %w", err)
	}

	return nil
}
