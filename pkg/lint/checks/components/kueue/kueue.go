package kueue

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	kind                       = "kueue"
	checkTypeManagementState   = "management-state"
	managementStateRemediation = "Migrate to the standalone Kueue operator (RHBOK) and set managementState to 'Removed' in DataScienceCluster before upgrading"
)

// ManagementStateCheck validates that Kueue managed option is not used before upgrading to 3.x.
// In RHOAI 3.x, the Managed option for Kueue is removed — users must migrate to the standalone
// Kueue operator (RHBOK) and set managementState to Removed or Unmanaged.
type ManagementStateCheck struct {
	base.BaseCheck
}

func NewManagementStateCheck() *ManagementStateCheck {
	return &ManagementStateCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             checkTypeManagementState,
			CheckID:          "components.kueue.management-state",
			CheckName:        "Components :: Kueue :: Management State (3.x)",
			CheckDescription: "Validates that Kueue managementState is compatible with RHOAI 3.x (Managed option will be removed)",
			CheckRemediation: managementStateRemediation,
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ManagementStateCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *ManagementStateCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
			switch req.ManagementState {
			case check.ManagementStateManaged:
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage("Kueue is managed by OpenShift AI (state: %s) but Managed option will be removed in RHOAI 3.x", req.ManagementState),
					check.WithRemediation(c.CheckRemediation),
				))
			default:
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("Kueue managementState is %s — compatible with RHOAI 3.x", req.ManagementState),
				))
			}

			return nil
		})
}
