package codeflare

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
)

const kind = "codeflare"

// RemovalCheck validates that CodeFlare is disabled before upgrading to 3.x.
type RemovalCheck struct {
	check.BaseCheck
}

func NewRemovalCheck() *RemovalCheck {
	return &RemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             check.CheckTypeRemoval,
			CheckID:          "components.codeflare.removal",
			CheckName:        "Components :: CodeFlare :: Removal (3.x)",
			CheckDescription: "Validates that CodeFlare is disabled before upgrading from RHOAI 2.x to 3.x (component will be removed)",
			CheckRemediation: "Disable CodeFlare by setting managementState to 'Removed' in DataScienceCluster before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x and CodeFlare is Managed.
func (c *RemovalCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, kind, constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, validate.Removal("CodeFlare is enabled (state: %s) but will be removed in RHOAI 3.x",
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation)))
}
