package kserve

import (
	"context"
	"errors"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const checkType = "serverless-removal"

// ServerlessRemovalCheck validates that KServe serverless is disabled before upgrading to 3.x.
type ServerlessRemovalCheck struct {
	check.BaseCheck
}

func NewServerlessRemovalCheck() *ServerlessRemovalCheck {
	return &ServerlessRemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentKServe,
			Type:             checkType,
			CheckID:          "components.kserve.serverless-removal",
			CheckName:        "Components :: KServe :: Serverless Removal (3.x)",
			CheckDescription: "Validates that KServe serverless mode is disabled before upgrading from RHOAI 2.x to 3.x (serverless support will be removed)",
			CheckRemediation: "Disable KServe serverless mode by setting serving.managementState to 'Removed' in DataScienceCluster before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x and KServe is Managed.
func (c *ServerlessRemovalCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentKServe, constants.ManagementStateManaged), nil
}

func (c *ServerlessRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
			state, err := jq.Query[string](req.DSC, ".spec.components.kserve.serving.managementState")

			switch {
			case errors.Is(err, jq.ErrNotFound):
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("KServe serverless mode is not configured - ready for RHOAI 3.x upgrade"),
				))
			case err != nil:
				return fmt.Errorf("querying kserve serving managementState: %w", err)
			case state == constants.ManagementStateManaged || state == constants.ManagementStateUnmanaged:
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage("KServe serverless mode is enabled (state: %s) but will be removed in RHOAI 3.x", state),
					check.WithImpact(result.ImpactBlocking),
					check.WithRemediation(c.CheckRemediation),
				))
			default:
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("KServe serverless mode is disabled (state: %s) - ready for RHOAI 3.x upgrade", state),
				))
			}

			return nil
		})
}
