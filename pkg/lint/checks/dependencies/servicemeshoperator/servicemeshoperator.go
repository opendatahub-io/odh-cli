package servicemeshoperator

import (
	"context"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

// Check validates that Service Mesh Operator v2 is not installed when upgrading to 3.x.
type Check struct {
	base.BaseCheck
}

// NewCheck creates a new Service Mesh Operator v2 upgrade check.
func NewCheck() *Check {
	return &Check{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             check.DependencyServiceMeshOperatorV2,
			Type:             check.CheckTypeUpgrade,
			CheckID:          "dependencies.servicemeshoperator2.upgrade",
			CheckName:        "Dependencies :: ServiceMeshOperator2 :: Upgrade (3.x)",
			CheckDescription: "Validates that servicemeshoperator2 is not installed when upgrading to RHOAI 3.x (requires servicemeshoperator3)",
		},
	}
}

func (c *Check) CanApply(_ context.Context, target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Operator(c, target).
		WithNames("servicemeshoperator").
		WithChannels("stable", "v2.x").
		WithConditionBuilder(func(found bool, version string) result.Condition {
			// Inverted logic: NOT finding the operator is good.
			if !found {
				return results.NewCompatibilitySuccess("Service Mesh Operator v2 is not installed - ready for RHOAI 3.x upgrade")
			}

			return results.NewCompatibilityFailure("Service Mesh Operator v2 (%s) is installed but RHOAI 3.x requires v3", version)
		}).
		Run(ctx)
}
