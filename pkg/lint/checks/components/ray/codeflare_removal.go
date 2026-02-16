package ray

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

const (
	kind = "ray"

	// dscComponent is the DSC spec key for the CodeFlare component.
	// The user-facing kind is "ray" but the DSC still uses "codeflare".
	dscComponent = "codeflare"
)

// CodeFlareRemovalCheck validates that CodeFlare is disabled before upgrading to 3.x.
type CodeFlareRemovalCheck struct {
	check.BaseCheck
}

func NewCodeFlareRemovalCheck() *CodeFlareRemovalCheck {
	return &CodeFlareRemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             check.CheckTypeRemoval,
			CheckID:          "components.ray.codeflare-removal",
			CheckName:        "Components :: Ray :: CodeFlare Removal (3.x)",
			CheckDescription: "Validates that the CodeFlare security layer is disabled before upgrading from RHOAI 2.x to 3.x",
			CheckRemediation: "Disable CodeFlare by setting managementState to 'Removed' in DataScienceCluster before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x and CodeFlare is Managed.
func (c *CodeFlareRemovalCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, dscComponent, constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *CodeFlareRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		WithComponentName(dscComponent).
		Run(ctx, validate.Removal("CodeFlare is enabled (state: %s) but will be removed in RHOAI %s",
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation)))
}
