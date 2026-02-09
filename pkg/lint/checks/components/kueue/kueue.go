package kueue

import (
	"context"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	kind                    = "kueue"
	checkTypeManagedRemoval = "managed-removal"
)

// ManagedRemovalCheck validates that Kueue managed option is not used before upgrading to 3.x.
type ManagedRemovalCheck struct {
	base.BaseCheck
}

func NewManagedRemovalCheck() *ManagedRemovalCheck {
	return &ManagedRemovalCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             checkTypeManagedRemoval,
			CheckID:          "components.kueue.managed-removal",
			CheckName:        "Components :: Kueue :: Managed Removal (3.x)",
			CheckDescription: "Validates that Kueue managed option is not used before upgrading from RHOAI 2.x to 3.x (managed option will be removed)",
			CheckRemediation: "Migrate to the standalone Kueue operator (RHBOK) and set managementState to 'Removed' in DataScienceCluster before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ManagedRemovalCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *ManagedRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		InState(check.ManagementStateManaged).
		Run(ctx, validate.Removal("Kueue is managed by OpenShift AI (state: %s) but will be removed in RHOAI 3.x - migrate to RHBOK operator",
			check.WithRemediation(c.CheckRemediation)))
}
