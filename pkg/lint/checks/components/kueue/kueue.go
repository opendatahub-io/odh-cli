package kueue

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

const (
	kind                       = "kueue"
	checkTypeManagementState   = "management-state"
	managementStateRemediation = "Migrate to the RHBoK operator following https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/2.25/html/managing_openshift_ai/managing-workloads-with-kueue#migrating-to-the-rhbok-operator_kueue before upgrading"
)

// ManagementStateCheck validates that Kueue managed option is not used before upgrading to 3.x.
// In RHOAI 3.x, the Managed option for Kueue is removed — users must migrate to the standalone
// Kueue operator (RHBOK) and set managementState to Removed or Unmanaged.
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
			CheckDescription: "Validates that Kueue managementState is compatible with RHOAI 3.x (Managed option will be removed)",
			CheckRemediation: managementStateRemediation,
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
					check.WithMessage("Kueue is managed by OpenShift AI (state: %s) but Managed option will be removed in RHOAI 3.x", req.ManagementState),
					check.WithImpact(result.ImpactBlocking),
					check.WithRemediation(c.CheckRemediation),
				))
			case constants.ManagementStateUnmanaged:
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
