package kserve

import (
	"context"
	"errors"
	"fmt"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const checkType = "serverless-removal"

// ServerlessRemovalCheck validates that KServe serverless is disabled before upgrading to 3.x.
type ServerlessRemovalCheck struct {
	base.BaseCheck
}

func NewServerlessRemovalCheck() *ServerlessRemovalCheck {
	return &ServerlessRemovalCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentKServe,
			Type:             checkType,
			CheckID:          "components.kserve.serverless-removal",
			CheckName:        "Components :: KServe :: Serverless Removal (3.x)",
			CheckDescription: "Validates that KServe serverless mode is disabled before upgrading from RHOAI 2.x to 3.x (serverless support will be removed)",
			CheckRemediation: "Disable KServe serverless mode by setting serving.managementState to 'Removed' in DataScienceCluster before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ServerlessRemovalCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *ServerlessRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		InState(check.ManagementStateManaged).
		Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
			state, err := jq.Query[string](req.DSC, ".spec.components.kserve.serving.managementState")

			switch {
			case errors.Is(err, jq.ErrNotFound):
				results.SetCompatibilitySuccessf(req.Result, "KServe serverless mode is not configured - ready for RHOAI 3.x upgrade")
			case err != nil:
				return fmt.Errorf("querying kserve serving managementState: %w", err)
			case state == check.ManagementStateManaged || state == check.ManagementStateUnmanaged:
				results.SetCompatibilityFailuref(req.Result, "KServe serverless mode is enabled (state: %s) but will be removed in RHOAI 3.x", state,
					check.WithRemediation(c.CheckRemediation))
			default:
				results.SetCompatibilitySuccessf(req.Result, "KServe serverless mode is disabled (state: %s) - ready for RHOAI 3.x upgrade", state)
			}

			return nil
		})
}
