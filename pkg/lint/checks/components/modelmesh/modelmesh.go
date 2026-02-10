package modelmesh

import (
	"context"
	"fmt"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const kind = "modelmeshserving"

// RemovalCheck validates that ModelMesh is disabled before upgrading to 3.x.
type RemovalCheck struct {
	base.BaseCheck
}

func NewRemovalCheck() *RemovalCheck {
	return &RemovalCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             check.CheckTypeRemoval,
			CheckID:          "components.modelmesh.removal",
			CheckName:        "Components :: ModelMesh :: Removal (3.x)",
			CheckDescription: "Validates that ModelMesh is disabled before upgrading from RHOAI 2.x to 3.x (component will be removed)",
			CheckRemediation: "Disable ModelMesh by setting managementState to 'Removed' in DataScienceCluster before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x and ModelMesh is Managed.
func (c *RemovalCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, kind, check.ManagementStateManaged), nil
}

func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, validate.Removal("ModelMesh is enabled (state: %s) but will be removed in RHOAI 3.x",
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation)))
}
