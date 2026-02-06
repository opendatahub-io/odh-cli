package kueue

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

// ManagedRemovalCheck validates that Kueue managed option is not used before upgrading to 3.x.
type ManagedRemovalCheck struct {
	base.BaseCheck
}

func NewManagedRemovalCheck() *ManagedRemovalCheck {
	return &ManagedRemovalCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentKueue,
			CheckType:        check.CheckTypeManagedRemoval,
			CheckID:          "components.kueue.managed-removal",
			CheckName:        "Components :: Kueue :: Managed Removal (3.x)",
			CheckDescription: "Validates that Kueue managed option is not used before upgrading from RHOAI 2.x to 3.x (managed option will be removed)",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ManagedRemovalCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *ManagedRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	// Get the DataScienceCluster singleton
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(string(c.Group()), c.Kind, c.CheckType, c.Description()), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	managementStateStr, configured, err := components.GetManagementState(dsc, "kueue")
	if err != nil {
		return nil, fmt.Errorf("querying kueue managementState: %w", err)
	}

	if !configured {
		results.SetComponentNotConfigured(dr, "Kueue")

		return dr, nil
	}

	// Add management state as annotation
	dr.Annotations[check.AnnotationComponentManagementState] = managementStateStr
	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Check if kueue is Managed (old way - needs migration)
	if managementStateStr == check.ManagementStateManaged {
		results.SetCompatibilityFailuref(dr, "Kueue is managed by OpenShift AI (state: %s) but will be removed in RHOAI 3.x - migrate to RHBOK operator", managementStateStr)

		return dr, nil
	}

	// Kueue is Unmanaged (using RHBOK operator) or Removed - check passes
	results.SetCompatibilitySuccessf(dr, "Kueue configuration (state: %s) is compatible with RHOAI 3.x", managementStateStr)

	return dr, nil
}
