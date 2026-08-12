package trainingoperator

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// RemovalCheck validates that TrainingOperator v1 is disabled before upgrading to 3.6.
type RemovalCheck struct {
	check.BaseCheck
}

func NewRemovalCheck() *RemovalCheck {
	return &RemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentTrainingOperator,
			Type:             check.CheckTypeRemoval,
			CheckID:          "components.trainingoperator.removal",
			CheckName:        "Components :: TrainingOperator :: Removal (3.6)",
			CheckDescription: "Validates that TrainingOperator (Kubeflow Training Operator v1) is disabled before upgrading to RHOAI 3.6 (component is removed, use Trainer v2)",
			CheckRemediation: "Before upgrading, drain active PyTorchJobs then set trainingoperator managementState to 'Removed' in DataScienceCluster and delete the TrainingOperator CR.",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Applies when target version >= 3.6 and TrainingOperator is Managed.
func (c *RemovalCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	//nolint:mnd // Version numbers 3.6
	if !version.IsVersionAtLeast(target.TargetVersion, 3, 6) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentTrainingOperator, constants.ManagementStateManaged), nil
}

func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, validate.Removal("TrainingOperator (Kubeflow v1) is enabled (state: %s) but is removed in RHOAI %s. Use Trainer v2 instead.",
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation)))
}
