package codeflare

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
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
			CheckType:        check.CheckTypeRemoval,
			CheckID:          "components.codeflare.removal",
			CheckName:        "Components :: CodeFlare :: Removal (3.x)",
			CheckDescription: "Validates that CodeFlare is disabled before upgrading from RHOAI 2.x to 3.x (component will be removed)",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *RemovalCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	// Get the DataScienceCluster singleton
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(string(c.Group()), c.Kind, c.CheckType, c.Description()), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	managementStateStr, configured, err := components.GetManagementState(dsc, "codeflare")
	if err != nil {
		return nil, fmt.Errorf("querying codeflare managementState: %w", err)
	}

	if !configured {
		results.SetComponentNotConfigured(dr, "CodeFlare")

		return dr, nil
	}

	// Add management state as annotation
	dr.Annotations[check.AnnotationComponentManagementState] = managementStateStr
	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Check if codeflare is enabled (Managed or Unmanaged)
	if managementStateStr == check.ManagementStateManaged || managementStateStr == check.ManagementStateUnmanaged {
		results.SetCompatibilityFailuref(dr, "CodeFlare is enabled (state: %s) but will be removed in RHOAI 3.x", managementStateStr)

		return dr, nil
	}

	// CodeFlare is disabled (Removed) - check passes
	results.SetCompatibilitySuccessf(dr, "CodeFlare is disabled (state: %s) - ready for RHOAI 3.x upgrade", managementStateStr)

	return dr, nil
}
