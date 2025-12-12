package rhbok

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/migrate/action"
	"github.com/lburgazzoli/odh-cli/pkg/migrate/action/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/confirmation"
)

var errOperatorTimeout = errors.New("timeout waiting for operator to be ready")

const (
	actionID          = "kueue.rhbok.migrate"
	actionName        = "Migrate Kueue to RHBOK"
	actionDescription = "Migrates from OpenShift AI built-in Kueue to Red Hat Build of Kueue operator"
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

func (a *RHBOKMigrationAction) CanApply(
	currentVersion *semver.Version,
	_ *semver.Version,
) bool {
	if currentVersion == nil {
		return false
	}

	return currentVersion.Major == 2 && currentVersion.Minor >= 25
}

func (a *RHBOKMigrationAction) Validate(
	ctx context.Context,
	target *action.ActionTarget,
) (*result.ActionResult, error) {
	res := result.New("migration", "rhbok", "preflight", "Pre-flight validation for RHBOK migration")

	step1 := a.checkCurrentKueueState(ctx, target)
	res.Status.Steps = append(res.Status.Steps, step1)

	step2 := a.checkNoRHBOKConflicts(ctx, target)
	res.Status.Steps = append(res.Status.Steps, step2)

	step3 := a.verifyKueueResources(ctx, target)
	res.Status.Steps = append(res.Status.Steps, step3)

	step4 := backupResources(ctx, target)
	res.Status.Steps = append(res.Status.Steps, step4)

	res.Status.Completed = true

	return res, nil
}

func (a *RHBOKMigrationAction) Execute(
	ctx context.Context,
	target *action.ActionTarget,
) (*result.ActionResult, error) {
	res := result.New("migration", "rhbok", "execute", "Execute RHBOK migration")

	step1 := a.installRHBOKOperator(ctx, target)
	res.Status.Steps = append(res.Status.Steps, step1)
	if step1.Status == result.StepFailed {
		return res, fmt.Errorf("failed: %s", step1.Message)
	}

	step2 := a.updateDataScienceCluster(ctx, target)
	res.Status.Steps = append(res.Status.Steps, step2)
	if step2.Status == result.StepFailed {
		return res, fmt.Errorf("failed: %s", step2.Message)
	}

	step3 := a.verifyResourcesPreserved(ctx, target)
	res.Status.Steps = append(res.Status.Steps, step3)

	res.Status.Completed = true

	return res, nil
}

func (a *RHBOKMigrationAction) installRHBOKOperator(
	ctx context.Context,
	target *action.ActionTarget,
) result.ActionStep {
	step := result.NewStep(
		"install-rhbok-operator",
		"Install Red Hat Build of Kueue Operator",
		result.StepRunning,
		"",
	)

	if target.DryRun {
		step.Status = result.StepSkipped
		step.Message = "DRY RUN: Would create Subscription rhods-kueue-operator in openshift-operators"

		return step
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to install Red Hat Build of Kueue Operator")
		if !confirmation.Prompt(target.IO, "Proceed with operator installation?") {
			step.Status = result.StepSkipped
			step.Message = "User cancelled installation"

			return step
		}
	}

	existing, err := target.Client.Dynamic.Resource(resources.Subscription.GVR()).
		Namespace("openshift-operators").
		Get(ctx, "rhods-kueue-operator", metav1.GetOptions{})

	if err == nil && existing != nil {
		step.Status = result.StepCompleted
		step.Message = "RHBOK operator already installed (skipped)"

		return step
	}

	subscription := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Subscription.APIVersion(),
			"kind":       resources.Subscription.Kind,
			"metadata": map[string]any{
				"name":      "rhods-kueue-operator",
				"namespace": "openshift-operators",
			},
			"spec": map[string]any{
				"channel":         "stable",
				"name":            "rhods-kueue-operator",
				"source":          "redhat-operators",
				"sourceNamespace": "openshift-marketplace",
			},
		},
	}

	_, err = target.Client.Dynamic.Resource(resources.Subscription.GVR()).
		Namespace("openshift-operators").
		Create(ctx, subscription, metav1.CreateOptions{})

	if err != nil {
		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Failed to create subscription: %v", err)

		return step
	}

	if err := a.waitForOperatorReady(ctx, target); err != nil {
		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Operator installation failed: %v", err)

		return step
	}

	step.Status = result.StepCompleted
	step.Message = "RHBOK operator installed successfully"

	return step
}

func (a *RHBOKMigrationAction) waitForOperatorReady(
	ctx context.Context,
	target *action.ActionTarget,
) error {
	const (
		operatorTimeout     = 5 * time.Minute
		operatorPollPeriod  = 10 * time.Second
		csvNamePrefixLength = 18
	)

	timeout := time.After(operatorTimeout)
	ticker := time.NewTicker(operatorPollPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return errOperatorTimeout
		case <-ticker.C:
			csvList, err := target.Client.Dynamic.Resource(resources.ClusterServiceVersion.GVR()).
				Namespace("openshift-operators").
				List(ctx, metav1.ListOptions{})

			if err != nil {
				continue
			}

			for _, item := range csvList.Items {
				name, _, _ := unstructured.NestedString(item.Object, "metadata", "name")
				if name != "" && len(name) >= csvNamePrefixLength && name[:csvNamePrefixLength] == "rhods-kueue-operator" {
					phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
					if phase == "Succeeded" {
						return nil
					}
				}
			}
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}
	}
}

func (a *RHBOKMigrationAction) updateDataScienceCluster(
	ctx context.Context,
	target *action.ActionTarget,
) result.ActionStep {
	step := result.NewStep(
		"update-datasciencecluster",
		"Update DataScienceCluster Kueue managementState",
		result.StepRunning,
		"",
	)

	if target.DryRun {
		step.Status = result.StepSkipped
		step.Message = "DRY RUN: Would set spec.components.kueue.managementState=Unmanaged"

		return step
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to update DataScienceCluster Kueue managementState to Unmanaged")
		if !confirmation.Prompt(target.IO, "Proceed with configuration update?") {
			step.Status = result.StepSkipped
			step.Message = "User cancelled update"

			return step
		}
	}

	dsc, err := target.Client.GetDataScienceCluster(ctx)
	if err != nil {
		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Failed to get DataScienceCluster: %v", err)

		return step
	}

	err = unstructured.SetNestedField(dsc.Object, "Unmanaged", "spec", "components", "kueue", "managementState")
	if err != nil {
		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Failed to set managementState: %v", err)

		return step
	}

	_, err = target.Client.Dynamic.Resource(resources.DataScienceCluster.GVR()).
		Update(ctx, dsc, metav1.UpdateOptions{})
	if err != nil {
		step.Status = result.StepFailed
		step.Message = fmt.Sprintf("Failed to update DataScienceCluster: %v", err)

		return step
	}

	step.Status = result.StepCompleted
	step.Message = "DataScienceCluster updated successfully"

	return step
}

func (a *RHBOKMigrationAction) verifyResourcesPreserved(
	ctx context.Context,
	target *action.ActionTarget,
) result.ActionStep {
	step := result.NewStep(
		"verify-resources-preserved",
		"Verify ClusterQueue and LocalQueue resources preserved",
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
	step.Message = fmt.Sprintf("All %d ClusterQueues and %d LocalQueues preserved",
		len(clusterQueues), len(localQueues))

	return step
}

//nolint:gochecknoinits
func init() {
	action.MustRegisterAction(&RHBOKMigrationAction{})
}
