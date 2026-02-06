package trainingoperator

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type DeprecationCheck struct {
	base.BaseCheck
}

func NewDeprecationCheck() *DeprecationCheck {
	return &DeprecationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentTrainingOperator,
			CheckType:        check.CheckTypeDeprecation,
			CheckID:          "components.trainingoperator.deprecation",
			CheckName:        "Components :: TrainingOperator :: Deprecation (3.3+)",
			CheckDescription: "Validates that TrainingOperator (Kubeflow Training Operator v1) deprecation is acknowledged - will be replaced by Trainer v2 in future RHOAI releases",
		},
	}
}

func (c *DeprecationCheck) CanApply(target check.Target) bool {
	//nolint:mnd // Version numbers 3.3
	return version.IsVersionAtLeast(target.TargetVersion, 3, 3)
}

func (c *DeprecationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(string(c.Group()), c.Kind, c.CheckType, c.Description()), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	managementStateStr, configured, err := components.GetManagementState(dsc, "trainingoperator")
	if err != nil {
		return nil, fmt.Errorf("querying trainingoperator managementState: %w", err)
	}

	if !configured {
		results.SetComponentNotConfigured(dr, "TrainingOperator")

		return dr, nil
	}

	dr.Annotations[check.AnnotationComponentManagementState] = managementStateStr
	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	if managementStateStr == check.ManagementStateManaged || managementStateStr == check.ManagementStateUnmanaged {
		// Deprecation is advisory (non-blocking).
		results.SetCondition(dr, check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.ReasonDeprecated,
			"TrainingOperator (Kubeflow Training Operator v1) is enabled (state: %s) but is deprecated in RHOAI 3.3 and will be replaced by Trainer v2 in a future release",
			managementStateStr,
			check.WithImpact(result.ImpactAdvisory),
		))

		return dr, nil
	}

	results.SetCompatibilitySuccessf(dr,
		"TrainingOperator is disabled (state: %s) - no action required for deprecation in RHOAI 3.3+",
		managementStateStr)

	return dr, nil
}
