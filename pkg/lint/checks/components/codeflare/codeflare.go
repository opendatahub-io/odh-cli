package codeflare

import (
	"context"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

// RemovalCheck validates that CodeFlare is disabled before upgrading to 3.x.
type RemovalCheck struct {
	base.BaseCheck
}

func NewRemovalCheck() *RemovalCheck {
	return &RemovalCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentCodeFlare,
			Type:             check.CheckTypeRemoval,
			CheckID:          "components.codeflare.removal",
			CheckName:        "Components :: CodeFlare :: Removal (3.x)",
			CheckDescription: "Validates that CodeFlare is disabled before upgrading from RHOAI 2.x to 3.x (component will be removed)",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *RemovalCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
			switch req.ManagementState {
			case check.ManagementStateManaged:
				// CodeFlare is enabled - blocks upgrade (Unmanaged not supported for this component)
				results.SetCompatibilityFailuref(req.Result,
					"CodeFlare is enabled (state: %s) but will be removed in RHOAI 3.x",
					req.ManagementState)
			default:
				// CodeFlare is disabled (Removed, Unmanaged, or not configured) - check passes
				results.SetCompatibilitySuccessf(req.Result,
					"CodeFlare is disabled (state: %s) - ready for RHOAI 3.x upgrade",
					req.ManagementState)
			}

			return nil
		})
}
