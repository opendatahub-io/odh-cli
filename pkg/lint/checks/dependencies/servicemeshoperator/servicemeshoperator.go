package servicemeshoperator

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind      = "servicemesh-operator-v2"
	checkType = "upgrade"
)

// Check validates that Service Mesh Operator v2 is not installed when upgrading to 3.x,
// as it is no longer required by RHOAI 3.x (OpenShift 4.19+ handles service mesh internally).
type Check struct {
	check.BaseCheck
}

// NewCheck creates a new Service Mesh Operator v2 upgrade check.
func NewCheck() *Check {
	return &Check{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             kind,
			Type:             checkType,
			CheckID:          "dependencies.servicemeshoperator2.upgrade",
			CheckName:        "Dependencies :: ServiceMeshOperator2 :: Upgrade (3.x)",
			CheckDescription: "Validates that Service Mesh Operator v2 is not installed when upgrading to RHOAI 3.x (no longer required, OpenShift 4.19+ handles service mesh internally)",
		},
	}
}

func (c *Check) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Operator(c, target).
		WithNames("servicemeshoperator").
		WithChannels("stable", "v2.x").
		WithConditionBuilder(func(found bool, version string) result.Condition {
			// Inverted logic: NOT finding the operator is good.
			if !found {
				return check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("Service Mesh Operator v2 is not installed - ready for RHOAI 3.x upgrade"),
				)
			}

			return check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonVersionIncompatible),
				check.WithMessage("Service Mesh Operator v2 (%s) is installed but no longer required by RHOAI 3.x and should be removed. OpenShift 4.19+ handles service mesh internally", version),
			)
		}).
		Run(ctx)
}
