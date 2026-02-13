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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const checkType = "deprecation"

type DeprecationCheck struct {
	check.BaseCheck
}

func NewDeprecationCheck() *DeprecationCheck {
	return &DeprecationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentTrainingOperator,
			Type:             checkType,
			CheckID:          "components.trainingoperator.deprecation",
			CheckName:        "Components :: TrainingOperator :: Deprecation (3.3+)",
			CheckDescription: "Validates that TrainingOperator (Kubeflow Training Operator v1) deprecation is acknowledged - will be replaced by Trainer v2 in future RHOAI releases",
			CheckRemediation: "Plan migration from TrainingOperator (Kubeflow v1) to Trainer v2 in a future release",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when target version is >= 3.3 and TrainingOperator is Managed.
func (c *DeprecationCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	//nolint:mnd // Version numbers 3.3
	if !version.IsVersionAtLeast(target.TargetVersion, 3, 3) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentTrainingOperator, constants.ManagementStateManaged), nil
}

func (c *DeprecationCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Complete(ctx, newDeprecationCondition)
}

func newDeprecationCondition(_ context.Context, req *validate.ComponentRequest) ([]result.Condition, error) {
	return []result.Condition{
		check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonDeprecated),
			check.WithMessage("TrainingOperator (Kubeflow Training Operator v1) is enabled (state: %s) but is deprecated in RHOAI 3.3 and will be replaced by Trainer v2 in a future release", req.ManagementState),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation("Plan migration from TrainingOperator (Kubeflow v1) to Trainer v2 in a future release"),
		),
	}, nil
}
