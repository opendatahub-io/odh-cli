package rhbok

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/lburgazzoli/odh-cli/pkg/migrate/action"
	"github.com/lburgazzoli/odh-cli/pkg/migrate/action/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/confirmation"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube/olm"
)

const (
	actionID          = "kueue.rhbok.migrate"
	actionName        = "Migrate Kueue to RHBOK"
	actionDescription = "Migrates from OpenShift AI built-in Kueue to Red Hat Build of Kueue operator"

	// Operator constants.
	operatorNamespace   = "openshift-kueue-operator"
	subscriptionName    = "kueue-operator"
	subscriptionPackage = "kueue-operator"

	// Retry configuration for conflict resolution.
	retryInitialDuration = 500 * time.Millisecond
	retryFactor          = 2.0
	retryJitter          = 0.1
	retryMaxSteps        = 5
	subscriptionChannel  = "stable-v1.1"
	subscriptionSource   = "redhat-operators"
	sourceNamespace      = "openshift-marketplace"
	csvNamePrefix        = "kueue-operator"
	operatorTimeout      = 5 * time.Minute
	operatorPollPeriod   = 10 * time.Second

	// DataScienceCluster constants.
	managementStateManaged   = "Managed"
	managementStateUnmanaged = "Unmanaged"
	kueueComponentPath       = ".spec.components.kueue.managementState"

	// ConfigMap constants.
	configMapName            = "kueue-manager-config"
	applicationsNamespace    = "redhat-ods-applications"
	configMapAnnotationKey   = "opendatahub.io/managed"
	configMapAnnotationValue = "false"
)

type RHBOKMigrationAction struct{}

func (a *RHBOKMigrationAction) ID() string {
	return actionID
}

func (a *RHBOKMigrationAction) Name() string {
	return actionName
}

func (a *RHBOKMigrationAction) Description() string {
	return actionDescription
}

func (a *RHBOKMigrationAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *RHBOKMigrationAction) CanApply(target action.Target) bool {
	return target.CurrentVersion.Major == 2 && target.CurrentVersion.Minor >= 25
}

func (a *RHBOKMigrationAction) Prepare() action.Task {
	return &prepareTask{action: a}
}

func (a *RHBOKMigrationAction) Run() action.Task {
	return &runTask{action: a}
}

func (a *RHBOKMigrationAction) checkKueueManaged(
	ctx context.Context,
	target action.Target,
) bool {
	step := target.Recorder.Child(
		"check-kueue-managed",
		"Check if Kueue is managed by DataScienceCluster",
	)

	dsc, err := target.Client.Dynamic().Resource(resources.DataScienceCluster.GVR()).
		Namespace("").
		Get(ctx, "default-dsc", metav1.GetOptions{})

	if err != nil {
		step.Complete(result.StepFailed, "Failed to get DataScienceCluster: %v", err)

		return false
	}

	managementState, err := jq.Query[string](dsc, kueueComponentPath)
	if err != nil {
		step.Complete(result.StepCompleted,
			"Kueue component not found in DataScienceCluster (not managed)")

		return false
	}

	if managementState == managementStateManaged {
		step.Complete(result.StepCompleted, "Kueue is managed (managementState=%s)", managementState)

		return true
	}

	step.Complete(result.StepCompleted, "Kueue is not managed (managementState=%s)", managementState)

	return false
}

func (a *RHBOKMigrationAction) preserveKueueConfig(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"preserve-kueue-config",
		"Preserve Kueue ConfigMap for reference",
	)

	// Check if ConfigMap exists (read-only, safe to run in dry-run)
	checkStep := step.Child(
		"check-configmap",
		fmt.Sprintf("Checking if ConfigMap '%s' exists in namespace '%s'", configMapName, applicationsNamespace),
	)

	configMap, err := target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
		Namespace(applicationsNamespace).
		Get(ctx, configMapName, metav1.GetOptions{})

	if err != nil {
		checkStep.Complete(result.StepCompleted, "ConfigMap not found")
		step.Complete(result.StepSkipped, "ConfigMap %s not found (skipped): %v", configMapName, err)

		return
	}

	checkStep.Complete(result.StepCompleted, "ConfigMap exists")

	// Apply annotation
	annotateStep := step.Child(
		"apply-annotation",
		fmt.Sprintf("Apply annotation %s=%s", configMapAnnotationKey, configMapAnnotationValue),
	)

	if target.DryRun {
		annotateStep.Complete(result.StepSkipped, "Would annotate ConfigMap %s/%s", applicationsNamespace, configMapName)
		step.Complete(result.StepSkipped, "Dry-run: ConfigMap annotation skipped")

		return
	}

	annotations, err := jq.Query[map[string]any](configMap, ".metadata.annotations")
	if err != nil || annotations == nil {
		annotations = make(map[string]any)
	}

	annotations[configMapAnnotationKey] = configMapAnnotationValue

	annotationsJSON, err := json.Marshal(annotations)
	if err != nil {
		annotateStep.Complete(result.StepFailed, "Failed to marshal annotations: %v", err)
		step.Complete(result.StepFailed, "Failed to annotate ConfigMap")

		return
	}

	if err := jq.Transform(configMap, ".metadata.annotations = %s", annotationsJSON); err != nil {
		annotateStep.Complete(result.StepFailed, "Failed to set annotations: %v", err)
		step.Complete(result.StepFailed, "Failed to annotate ConfigMap")

		return
	}

	_, err = target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
		Namespace(applicationsNamespace).
		Update(ctx, configMap, metav1.UpdateOptions{})

	if err != nil {
		annotateStep.Complete(result.StepFailed, "Failed to update ConfigMap: %v", err)
		step.Complete(result.StepFailed, "Failed to annotate ConfigMap")

		return
	}

	annotateStep.Complete(result.StepCompleted, "Annotation applied successfully")
	step.Complete(result.StepCompleted, "ConfigMap %s annotated for preservation", configMapName)
}

func (a *RHBOKMigrationAction) installRHBOKOperator(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"install-rhbok-operator",
		"Install Red Hat Build of Kueue Operator",
	)

	// Check if subscription exists first
	subscriptionExists := false
	if !target.DryRun {
		_, err := target.Client.OLMClient().OperatorsV1alpha1().Subscriptions(operatorNamespace).Get(ctx, subscriptionName, metav1.GetOptions{})
		subscriptionExists = err == nil
	}

	// Only prompt if subscription doesn't exist and we're not in dry-run mode
	if !target.DryRun && !target.SkipConfirm && !subscriptionExists {
		target.IO.Fprintln()
		target.IO.Errorf("About to install Red Hat Build of Kueue Operator")
		if !confirmation.Prompt(target.IO, "Proceed with operator installation?") {
			step.Complete(result.StepSkipped, "User cancelled installation")

			return
		}
	}

	err := olm.EnsureOperatorInstalled(ctx, target.Client, olm.InstallConfig{
		Name:            subscriptionName,
		Namespace:       operatorNamespace,
		Package:         subscriptionPackage,
		Channel:         subscriptionChannel,
		Source:          subscriptionSource,
		SourceNamespace: sourceNamespace,
		CSVNamePrefix:   csvNamePrefix,
		PollInterval:    operatorPollPeriod,
		Timeout:         operatorTimeout,
		DryRun:          target.DryRun,
		Recorder:        step,
		IO:              target.IO,
	})

	if err != nil {
		step.Complete(result.StepFailed, "Failed to install operator: %v", err)

		return
	}

	if target.DryRun {
		step.Complete(result.StepSkipped, "Operator installation checks completed")
	} else if subscriptionExists {
		step.Complete(result.StepCompleted, "RHBOK operator already installed and ready")
	} else {
		step.Complete(result.StepCompleted, "RHBOK operator installed successfully")
	}
}

func (a *RHBOKMigrationAction) updateDataScienceCluster(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"update-datasciencecluster",
		"Update DataScienceCluster Kueue managementState",
	)

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		step.Complete(result.StepFailed, "Failed to get DataScienceCluster: %v", err)

		return
	}

	// Check if already set to Unmanaged
	currentState, err := jq.Query[string](dsc, ".spec.components.kueue.managementState")
	if err == nil && currentState == managementStateUnmanaged {
		step.Complete(result.StepSkipped, "DataScienceCluster Kueue already set to Unmanaged")

		return
	}

	if target.DryRun {
		step.Complete(result.StepSkipped, "Would set %s=%s", kueueComponentPath, managementStateUnmanaged)

		return
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to update DataScienceCluster Kueue managementState to %s", managementStateUnmanaged)
		if !confirmation.Prompt(target.IO, "Proceed with configuration update?") {
			step.Complete(result.StepSkipped, "User cancelled update")

			return
		}
		target.IO.Fprintln()
	}

	// Retry update with exponential backoff in case of conflicts
	err = wait.ExponentialBackoff(wait.Backoff{
		Duration: retryInitialDuration,
		Factor:   retryFactor,
		Jitter:   retryJitter,
		Steps:    retryMaxSteps,
	}, func() (bool, error) {
		// Get latest version
		latestDSC, err := client.GetDataScienceCluster(ctx, target.Client)
		if err != nil {
			return false, fmt.Errorf("failed to get DataScienceCluster: %w", err)
		}

		// Apply the change
		if err := jq.Transform(latestDSC, ".spec.components.kueue.managementState = %q", managementStateUnmanaged); err != nil {
			return false, fmt.Errorf("failed to set managementState: %w", err)
		}

		// Attempt update
		_, err = target.Client.Dynamic().Resource(resources.DataScienceCluster.GVR()).
			Update(ctx, latestDSC, metav1.UpdateOptions{})
		if err != nil {
			// Retry on conflict errors
			if apierrors.IsConflict(err) {
				return false, nil // Retry
			}

			return false, fmt.Errorf("failed to update DataScienceCluster: %w", err)
		}

		return true, nil // Success
	})

	if err != nil {
		step.Complete(result.StepFailed, "Failed to update DataScienceCluster: %v", err)

		return
	}

	step.Complete(result.StepCompleted, "DataScienceCluster updated successfully")
}

func (a *RHBOKMigrationAction) verifyResourcesPreserved(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"verify-resources-preserved",
		"Verify ClusterQueue and LocalQueue resources preserved",
	)

	if target.DryRun {
		step.Complete(result.StepSkipped, "Would verify ClusterQueue and LocalQueue resources are preserved")

		return
	}

	clusterQueues, err := target.Client.ListResources(ctx, resources.ClusterQueue.GVR())
	if err != nil {
		// If the CRD doesn't exist, that's fine - it means there are no Kueue resources
		if apierrors.IsNotFound(err) {
			step.Complete(result.StepCompleted, "No ClusterQueue CRD found (no resources to preserve)")

			return
		}

		step.Complete(result.StepFailed, "Failed to list ClusterQueues: %v", err)

		return
	}

	localQueues, err := target.Client.ListResources(ctx, resources.LocalQueue.GVR())
	if err != nil {
		// If the CRD doesn't exist, that's fine - it means there are no Kueue resources
		if apierrors.IsNotFound(err) {
			step.Complete(result.StepCompleted, "No LocalQueue CRD found (%d ClusterQueues preserved)", len(clusterQueues))

			return
		}

		step.Complete(result.StepFailed, "Failed to list LocalQueues: %v", err)

		return
	}

	step.Complete(result.StepCompleted,
		"All %d ClusterQueues and %d LocalQueues preserved",
		len(clusterQueues), len(localQueues))
}
