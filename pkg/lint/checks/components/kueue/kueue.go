package kueue

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind                     = "kueue"
	checkTypeManagementState = "management-state"

	// Deferred: parameterize hardcoded version references using ComponentRequest.TargetVersion.
	msgManagedProhibited   = "The 3.3.1 upgrade currently only supports the Kueue managementState of Removed. A future 3.3.x release will allow an upgrade when you have migrated to the Red Hat build of Kueue Operator and the Kueue managementState is Unmanaged."
	msgUnmanagedProhibited = "The 3.3.1 upgrade currently only supports the Kueue managementState of Removed. A future 3.3.x release will allow an upgrade when the Kueue managementState is Unmanaged."
)

// ManagementStateCheck validates that Kueue managementState is Removed before upgrading to 3.x.
// In RHOAI 3.3.1, only the Removed state is supported. A future 3.3.x release will support
// Unmanaged with the Red Hat build of Kueue Operator.
type ManagementStateCheck struct {
	check.BaseCheck
}

func NewManagementStateCheck() *ManagementStateCheck {
	return &ManagementStateCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             checkTypeManagementState,
			CheckID:          "components.kueue.management-state",
			CheckName:        "Components :: Kueue :: Management State (3.x)",
			CheckDescription: "Validates that Kueue managementState is Removed before upgrading to RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x and Kueue is Managed or Unmanaged.
func (c *ManagementStateCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(
		dsc, "kueue",
		constants.ManagementStateManaged, constants.ManagementStateUnmanaged,
	), nil
}

func (c *ManagementStateCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
			switch req.ManagementState {
			case constants.ManagementStateManaged:
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage(msgManagedProhibited),
					check.WithImpact(result.ImpactProhibited),
				))
			case constants.ManagementStateUnmanaged:
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage(msgUnmanagedProhibited),
					check.WithImpact(result.ImpactProhibited),
				))
			default:
				return fmt.Errorf("unexpected management state %q for kueue", req.ManagementState)
			}

			return nil
		})
}
